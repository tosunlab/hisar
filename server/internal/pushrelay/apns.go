package pushrelay

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
)

const (
	apnsProductionURL = "https://api.push.apple.com"
	apnsSandboxURL    = "https://api.sandbox.push.apple.com"
	// Apple refuses provider tokens older than an hour and too frequent new ones, so one is reused for 50 minutes.
	apnsTokenLifetime = 50 * time.Minute
)

// APNsConfig holds the relay's APNs Auth Key and the app it sends to.
type APNsConfig struct {
	KeyID   string
	TeamID  string
	Topic   string
	Key     *ecdsa.PrivateKey
	BaseURL string
	// SandboxBaseURL is the development environment's address; empty means Apple's.
	SandboxBaseURL string
	Client         *http.Client
	Now            func() time.Time
}

type APNsSender struct {
	config APNsConfig

	mutex     sync.Mutex
	token     string
	createdAt time.Time
}

// ParseAPNsKey reads the .p8 file Apple gives for an APNs Auth Key.
func ParseAPNsKey(pemBytes []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("pushrelay: the APNs key is not PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("pushrelay: the APNs key: %w", err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("pushrelay: the APNs key is not an EC key")
	}
	return key, nil
}

func NewAPNsSender(config APNsConfig) *APNsSender {
	if config.BaseURL == "" {
		config.BaseURL = apnsProductionURL
	}
	if config.SandboxBaseURL == "" {
		config.SandboxBaseURL = apnsSandboxURL
	}
	if config.Client == nil {
		config.Client = &http.Client{Timeout: 15 * time.Second}
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &APNsSender{config: config}
}

func (sender *APNsSender) providerToken() (string, error) {
	sender.mutex.Lock()
	defer sender.mutex.Unlock()
	now := sender.config.Now()
	if sender.token != "" && now.Sub(sender.createdAt) < apnsTokenLifetime {
		return sender.token, nil
	}
	header, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": sender.config.KeyID})
	claims, _ := json.Marshal(map[string]any{"iss": sender.config.TeamID, "iat": now.Unix()})
	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, sender.config.Key, digest[:])
	if err != nil {
		return "", err
	}
	signature := append(padTo32(r), padTo32(s)...)
	sender.token = signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
	sender.createdAt = now
	return sender.token, nil
}

func padTo32(value *big.Int) []byte {
	out := make([]byte, 32)
	value.FillBytes(out)
	return out
}

// apnsPayload is the alert: its text is the app's own localized sentence for the kind, so the push carries no words.
// With a payload, the details encrypted to the phone, the app's notification service extension may replace the text
// (mutable-content); APNs and the relay cannot read it. An approval request ends within minutes, so it is
// time-sensitive: it breaks through a Focus that allows time-sensitive notifications.
func apnsPayload(kind apitypes.PushKind, payload string) []byte {
	aps := map[string]any{
		"alert":     map[string]string{"loc-key": "push." + string(kind)},
		"sound":     "default",
		"thread-id": string(kind),
	}
	if kind == apitypes.PushKindApproval {
		aps["interruption-level"] = "time-sensitive"
	}
	message := map[string]any{"aps": aps, "kind": string(kind)}
	if payload != "" {
		aps["mutable-content"] = 1
		message["payload"] = payload
	}
	encoded, _ := json.Marshal(message)
	return encoded
}

func (sender *APNsSender) Send(ctx context.Context, platform apitypes.PushPlatform, token string, kind apitypes.PushKind, payload string) error {
	baseURL := sender.config.BaseURL
	if platform == apitypes.PushPlatformAPNsSandbox {
		baseURL = sender.config.SandboxBaseURL
	}
	providerToken, err := sender.providerToken()
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/3/device/"+token, bytes.NewReader(apnsPayload(kind, payload)))
	if err != nil {
		return err
	}
	request.Header.Set("authorization", "bearer "+providerToken)
	request.Header.Set("apns-topic", sender.config.Topic)
	request.Header.Set("apns-push-type", "alert")
	request.Header.Set("apns-priority", "10")
	request.Header.Set("apns-collapse-id", string(kind))
	request.Header.Set("apns-expiration", strconv.FormatInt(sender.config.Now().Add(kind.GetTimeToLive()).Unix(), 10))
	response, err := sender.config.Client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK {
		return nil
	}
	var failure struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&failure)
	if response.StatusCode == http.StatusGone || failure.Reason == "BadDeviceToken" || failure.Reason == "Unregistered" || failure.Reason == "DeviceTokenNotForTopic" {
		return ErrUnregistered
	}
	return fmt.Errorf("pushrelay: APNs answered %d %s", response.StatusCode, failure.Reason)
}
