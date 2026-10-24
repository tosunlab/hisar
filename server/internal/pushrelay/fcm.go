package pushrelay

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
)

const (
	fcmBaseURL   = "https://fcm.googleapis.com"
	fcmScope     = "https://www.googleapis.com/auth/firebase.messaging"
	googleGrant  = "urn:ietf:params:oauth:grant-type:jwt-bearer"
	accessMargin = 5 * time.Minute
)

// FCMServiceAccount is the part of a Google service account's JSON key the relay reads.
type FCMServiceAccount struct {
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

type FCMConfig struct {
	Account FCMServiceAccount
	BaseURL string
	Client  *http.Client
	Now     func() time.Time
}

type FCMSender struct {
	config FCMConfig
	key    *rsa.PrivateKey

	mutex       sync.Mutex
	accessToken string
	expiresAt   time.Time
}

func NewFCMSender(config FCMConfig) (*FCMSender, error) {
	block, _ := pem.Decode([]byte(config.Account.PrivateKey))
	if block == nil {
		return nil, errors.New("pushrelay: the FCM service account has no PEM private key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("pushrelay: the FCM service account key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("pushrelay: the FCM service account key is not RSA")
	}
	if config.Account.ProjectID == "" || config.Account.ClientEmail == "" || config.Account.TokenURI == "" {
		return nil, errors.New("pushrelay: the FCM service account lacks project_id, client_email or token_uri")
	}
	if config.BaseURL == "" {
		config.BaseURL = fcmBaseURL
	}
	if config.Client == nil {
		config.Client = &http.Client{Timeout: 15 * time.Second}
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &FCMSender{config: config, key: key}, nil
}

// getAccessToken trades a signed assertion for an OAuth access token, as Google's service accounts do, and keeps it
// until shortly before it expires.
func (sender *FCMSender) getAccessToken(ctx context.Context) (string, error) {
	sender.mutex.Lock()
	defer sender.mutex.Unlock()
	now := sender.config.Now()
	if sender.accessToken != "" && now.Before(sender.expiresAt.Add(-accessMargin)) {
		return sender.accessToken, nil
	}
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{
		"iss": sender.config.Account.ClientEmail, "scope": fcmScope, "aud": sender.config.Account.TokenURI,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	})
	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, sender.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	form := url.Values{"grant_type": {googleGrant}, "assertion": {signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, sender.config.Account.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := sender.config.Client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var answer struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<16)).Decode(&answer); err != nil || response.StatusCode != http.StatusOK || answer.AccessToken == "" {
		return "", fmt.Errorf("pushrelay: Google's token endpoint answered %d", response.StatusCode)
	}
	sender.accessToken = answer.AccessToken
	sender.expiresAt = now.Add(time.Duration(answer.ExpiresIn) * time.Second)
	return sender.accessToken, nil
}

// fcmMessage is a data message: the app posts the notification itself, in its own words for the kind.
func fcmMessage(token string, kind apitypes.PushKind) []byte {
	message, _ := json.Marshal(map[string]any{
		"message": map[string]any{
			"token": token,
			"data":  map[string]string{"kind": string(kind)},
			"android": map[string]any{
				"priority":     "HIGH",
				"collapse_key": string(kind),
				"ttl":          strconv.Itoa(int(getTimeToLive(kind).Seconds())) + "s",
			},
		},
	})
	return message
}

func (sender *FCMSender) Send(ctx context.Context, _ apitypes.PushPlatform, token string, kind apitypes.PushKind) error {
	accessToken, err := sender.getAccessToken(ctx)
	if err != nil {
		return err
	}
	endpoint := sender.config.BaseURL + "/v1/projects/" + url.PathEscape(sender.config.Account.ProjectID) + "/messages:send"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(fcmMessage(token, kind)))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := sender.config.Client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK {
		return nil
	}
	var failure struct {
		Error struct {
			Status  string `json:"status"`
			Details []struct {
				ErrorCode string `json:"errorCode"`
			} `json:"details"`
		} `json:"error"`
	}
	json.NewDecoder(io.LimitReader(response.Body, 1<<16)).Decode(&failure)
	for _, detail := range failure.Error.Details {
		if detail.ErrorCode == "UNREGISTERED" || detail.ErrorCode == "INVALID_ARGUMENT" {
			return ErrUnregistered
		}
	}
	if response.StatusCode == http.StatusNotFound {
		return ErrUnregistered
	}
	return fmt.Errorf("pushrelay: FCM answered %d %s", response.StatusCode, failure.Error.Status)
}
