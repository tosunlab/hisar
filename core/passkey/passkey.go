// Package passkey is Hisar's WebAuthn authenticator: attestation, assertion, CBOR encoding and the PRF extension.
package passkey

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"

	"golang.org/x/net/publicsuffix"
)

const (
	AAGUID               = "fb041679-042a-49ae-b616-8779e2ac14ce"
	AlgorithmES256       = -7
	CredentialIDLength   = 32
	TypeCreate           = "webauthn.create"
	TypeGet              = "webauthn.get"
	MaxUserHandleLength  = 64
	ClientDataHashLength = sha256.Size
)

const (
	FlagUserPresent           byte = 0x01
	FlagUserVerified          byte = 0x04
	FlagBackupEligible        byte = 0x08
	FlagBackedUp              byte = 0x10
	FlagAttestedCredential    byte = 0x40
	flagAssertion                  = FlagUserPresent | FlagBackupEligible | FlagBackedUp
	flagCreation                   = flagAssertion | FlagAttestedCredential
	maxRelyingPartyIDLength        = 253
	maxChallengeLength             = 1024
	maxCredentialIDLength          = 1023
	maxClientDataStringLength      = 2048
	coseKeyTypeEC2                 = 2
	coseCurveP256                  = 1
	coseLabelKeyType               = 1
	coseLabelAlgorithm             = 3
	coseLabelCurve                 = -1
	coseLabelX                     = -2
	coseLabelY                     = -3
	cborMajorUnsigned              = 0
	cborMajorNegative              = 1
	cborMajorBytes                 = 2
	cborMajorText                  = 3
	cborMajorMap                   = 5
)

var aaguidBytes = [16]byte{0xfb, 0x04, 0x16, 0x79, 0x04, 0x2a, 0x49, 0xae, 0xb6, 0x16, 0x87, 0x79, 0xe2, 0xac, 0x14, 0xce}

var (
	ErrInvalidOrigin         = errors.New("passkey: the origin is not https, localhost or an Android app")
	ErrInvalidRelyingPartyID = errors.New("passkey: the relying party ID does not belong to the origin")
	ErrInvalidRequest        = errors.New("passkey: the request is not valid")
)

func GetAAGUID() []byte {
	return bytes.Clone(aaguidBytes[:])
}

func ParseOrigin(origin string) (*url.URL, error) {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, ErrInvalidOrigin
	}
	host := strings.ToLower(parsed.Hostname())
	switch parsed.Scheme {
	case "https":
	case "http":
		if host != "localhost" && !strings.HasSuffix(host, ".localhost") {
			return nil, ErrInvalidOrigin
		}
	default:
		return nil, ErrInvalidOrigin
	}
	return parsed, nil
}

// androidAppOriginPrefix starts a native Android app's origin; the SHA-256 of its signing certificate follows.
const androidAppOriginPrefix = "android:apk-key-hash:"

// BuildAndroidAppOrigin returns the origin Android passkey providers write in a native app's client data.
func BuildAndroidAppOrigin(signingCertificateSHA256 []byte) (string, error) {
	if len(signingCertificateSHA256) != sha256.Size {
		return "", ErrInvalidOrigin
	}
	return androidAppOriginPrefix + base64.RawURLEncoding.EncodeToString(signingCertificateSHA256), nil
}

func isAndroidAppOrigin(origin string) bool {
	encodedDigest, hasPrefix := strings.CutPrefix(origin, androidAppOriginPrefix)
	digest, err := base64.RawURLEncoding.Strict().DecodeString(encodedDigest)
	return hasPrefix && err == nil && len(digest) == sha256.Size
}

// ResolveRelyingPartyID: the host, or a suffix of it that is not a public suffix.
func ResolveRelyingPartyID(origin, requestedRelyingPartyID string) (string, error) {
	parsed, err := ParseOrigin(origin)
	if err != nil {
		return "", err
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	relyingPartyID := strings.ToLower(strings.TrimSpace(requestedRelyingPartyID))
	if relyingPartyID == "" {
		return host, nil
	}
	if len(relyingPartyID) > maxRelyingPartyIDLength || strings.HasPrefix(relyingPartyID, ".") || strings.HasSuffix(relyingPartyID, ".") {
		return "", ErrInvalidRelyingPartyID
	}
	if relyingPartyID == host {
		return relyingPartyID, nil
	}
	if net.ParseIP(host) != nil || net.ParseIP(relyingPartyID) != nil || !strings.HasSuffix(host, "."+relyingPartyID) {
		return "", ErrInvalidRelyingPartyID
	}
	if isPublicSuffix(relyingPartyID) {
		return "", ErrInvalidRelyingPartyID
	}
	return relyingPartyID, nil
}

// ValidateRelyingPartyID is for RP IDs the platform already bound to the site (iOS), so there is no origin to compare.
func ValidateRelyingPartyID(requestedRelyingPartyID string) (string, error) {
	relyingPartyID := strings.ToLower(strings.TrimSpace(requestedRelyingPartyID))
	if relyingPartyID == "" || len(relyingPartyID) > maxRelyingPartyIDLength || strings.HasPrefix(relyingPartyID, ".") || strings.HasSuffix(relyingPartyID, ".") || net.ParseIP(relyingPartyID) != nil {
		return "", ErrInvalidRelyingPartyID
	}
	parsed, err := url.Parse("https://" + relyingPartyID)
	if err != nil || parsed.Host != relyingPartyID || parsed.Port() != "" || parsed.Hostname() != relyingPartyID || strings.ContainsAny(relyingPartyID, "*_ ") {
		return "", ErrInvalidRelyingPartyID
	}
	if isPublicSuffix(relyingPartyID) {
		return "", ErrInvalidRelyingPartyID
	}
	return relyingPartyID, nil
}

type ClientData struct {
	Type          string
	Challenge     []byte
	Origin        string
	TopOrigin     string
	IsCrossOrigin bool
	// AndroidPackageName goes with an Android app's origin, and only with it.
	AndroidPackageName string
}

type clientDataJSON struct {
	Type               string `json:"type"`
	Challenge          string `json:"challenge"`
	Origin             string `json:"origin"`
	CrossOrigin        bool   `json:"crossOrigin"`
	TopOrigin          string `json:"topOrigin,omitempty"`
	AndroidPackageName string `json:"androidPackageName,omitempty"`
}

// BuildClientDataJSON takes a web origin, or an Android app's from BuildAndroidAppOrigin with its package name.
func BuildClientDataJSON(clientData ClientData) ([]byte, error) {
	if clientData.Type != TypeCreate && clientData.Type != TypeGet {
		return nil, ErrInvalidRequest
	}
	if len(clientData.Challenge) == 0 || len(clientData.Challenge) > maxChallengeLength || len(clientData.Origin) > maxClientDataStringLength || len(clientData.TopOrigin) > maxClientDataStringLength || len(clientData.AndroidPackageName) > maxClientDataStringLength {
		return nil, ErrInvalidRequest
	}
	if isAndroidAppOrigin(clientData.Origin) {
		if clientData.AndroidPackageName == "" || clientData.IsCrossOrigin {
			return nil, ErrInvalidRequest
		}
	} else if _, err := ParseOrigin(clientData.Origin); err != nil {
		return nil, err
	} else if clientData.AndroidPackageName != "" {
		return nil, ErrInvalidRequest
	}
	encoded := clientDataJSON{
		Type:               clientData.Type,
		Challenge:          base64.RawURLEncoding.EncodeToString(clientData.Challenge),
		Origin:             clientData.Origin,
		CrossOrigin:        clientData.IsCrossOrigin,
		AndroidPackageName: clientData.AndroidPackageName,
	}
	if clientData.IsCrossOrigin {
		encoded.TopOrigin = clientData.TopOrigin
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(encoded); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}

// The sign count is always 0.
func BuildAuthenticatorData(relyingPartyID string, flags byte, attestedCredentialData []byte) []byte {
	relyingPartyIDHash := sha256.Sum256([]byte(relyingPartyID))
	authenticatorData := make([]byte, 0, 37+len(attestedCredentialData))
	authenticatorData = append(authenticatorData, relyingPartyIDHash[:]...)
	authenticatorData = append(authenticatorData, flags)
	authenticatorData = binary.BigEndian.AppendUint32(authenticatorData, 0)
	return append(authenticatorData, attestedCredentialData...)
}

func BuildAttestedCredentialData(credentialID []byte, publicKey *ecdsa.PublicKey) ([]byte, error) {
	if len(credentialID) == 0 || len(credentialID) > maxCredentialIDLength {
		return nil, ErrInvalidRequest
	}
	coseKey, err := EncodeCOSEPublicKey(publicKey)
	if err != nil {
		return nil, err
	}
	attestedCredentialData := append(GetAAGUID(), byte(len(credentialID)>>8), byte(len(credentialID)))
	attestedCredentialData = append(attestedCredentialData, credentialID...)
	return append(attestedCredentialData, coseKey...), nil
}

// Map keys in CTAP2 canonical order.
func EncodeCOSEPublicKey(publicKey *ecdsa.PublicKey) ([]byte, error) {
	ecdhKey, err := publicKey.ECDH()
	if err != nil || ecdhKey.Curve() != ecdh.P256() {
		return nil, ErrInvalidRequest
	}
	uncompressed := ecdhKey.Bytes()
	encoded := appendCBORHeader(nil, cborMajorMap, 5)
	encoded = appendCBORInteger(encoded, coseLabelKeyType)
	encoded = appendCBORInteger(encoded, coseKeyTypeEC2)
	encoded = appendCBORInteger(encoded, coseLabelAlgorithm)
	encoded = appendCBORInteger(encoded, AlgorithmES256)
	encoded = appendCBORInteger(encoded, coseLabelCurve)
	encoded = appendCBORInteger(encoded, coseCurveP256)
	encoded = appendCBORInteger(encoded, coseLabelX)
	encoded = appendCBORBytes(encoded, uncompressed[1:33])
	encoded = appendCBORInteger(encoded, coseLabelY)
	return appendCBORBytes(encoded, uncompressed[33:65]), nil
}

// Map keys in CTAP2 canonical order.
func BuildAttestationObject(authenticatorData []byte) []byte {
	encoded := appendCBORHeader(nil, cborMajorMap, 3)
	encoded = appendCBORText(encoded, "fmt")
	encoded = appendCBORText(encoded, "none")
	encoded = appendCBORText(encoded, "attStmt")
	encoded = appendCBORHeader(encoded, cborMajorMap, 0)
	encoded = appendCBORText(encoded, "authData")
	return appendCBORBytes(encoded, authenticatorData)
}

type CreationRequest struct {
	Origin         string
	TopOrigin      string
	IsCrossOrigin  bool
	RelyingPartyID string
	Challenge      []byte
	UserVerified   bool
}

type Creation struct {
	CredentialID      []byte
	PrivateKey        []byte
	ClientDataJSON    []byte
	AuthenticatorData []byte
	AttestationObject []byte
	PublicKey         []byte
}

func CreateCredential(request CreationRequest) (Creation, error) {
	relyingPartyID, err := ResolveRelyingPartyID(request.Origin, request.RelyingPartyID)
	if err != nil {
		return Creation{}, err
	}
	clientData, err := BuildClientDataJSON(ClientData{Type: TypeCreate, Challenge: request.Challenge, Origin: request.Origin, TopOrigin: request.TopOrigin, IsCrossOrigin: request.IsCrossOrigin})
	if err != nil {
		return Creation{}, err
	}
	creation, err := createCredentialForRelyingParty(relyingPartyID, request.UserVerified)
	if err != nil {
		return Creation{}, err
	}
	creation.ClientDataJSON = clientData
	return creation, nil
}

// CreateCredentialWithClientDataHash serves platforms that hash clientDataJSON themselves (iOS); "none" attestation does not sign the hash.
func CreateCredentialWithClientDataHash(requestedRelyingPartyID string, clientDataHash []byte, userVerified bool) (Creation, error) {
	relyingPartyID, err := ValidateRelyingPartyID(requestedRelyingPartyID)
	if err != nil {
		return Creation{}, err
	}
	if len(clientDataHash) != ClientDataHashLength {
		return Creation{}, ErrInvalidRequest
	}
	return createCredentialForRelyingParty(relyingPartyID, userVerified)
}

func createCredentialForRelyingParty(relyingPartyID string, userVerified bool) (Creation, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return Creation{}, err
	}
	credentialID := make([]byte, CredentialIDLength)
	if _, err := rand.Read(credentialID); err != nil {
		return Creation{}, err
	}
	attestedCredentialData, err := BuildAttestedCredentialData(credentialID, &privateKey.PublicKey)
	if err != nil {
		return Creation{}, err
	}
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return Creation{}, err
	}
	publicKeyDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return Creation{}, err
	}
	authenticatorData := BuildAuthenticatorData(relyingPartyID, ceremonyFlags(flagCreation, userVerified), attestedCredentialData)
	return Creation{
		CredentialID:      credentialID,
		PrivateKey:        privateKeyDER,
		AuthenticatorData: authenticatorData,
		AttestationObject: BuildAttestationObject(authenticatorData),
		PublicKey:         publicKeyDER,
	}, nil
}

type AssertionRequest struct {
	Origin         string
	TopOrigin      string
	IsCrossOrigin  bool
	RelyingPartyID string
	Challenge      []byte
	UserVerified   bool
}

type Assertion struct {
	ClientDataJSON    []byte
	AuthenticatorData []byte
	Signature         []byte
}

func SignAssertion(request AssertionRequest, credentialRelyingPartyID string, privateKeyDER []byte) (Assertion, error) {
	relyingPartyID, err := ResolveRelyingPartyID(request.Origin, request.RelyingPartyID)
	if err != nil {
		return Assertion{}, err
	}
	if relyingPartyID != credentialRelyingPartyID {
		return Assertion{}, ErrInvalidRelyingPartyID
	}
	clientData, err := BuildClientDataJSON(ClientData{Type: TypeGet, Challenge: request.Challenge, Origin: request.Origin, TopOrigin: request.TopOrigin, IsCrossOrigin: request.IsCrossOrigin})
	if err != nil {
		return Assertion{}, err
	}
	clientDataHash := sha256.Sum256(clientData)
	assertion, err := signAuthenticatorData(relyingPartyID, clientDataHash[:], privateKeyDER, request.UserVerified)
	if err != nil {
		return Assertion{}, err
	}
	assertion.ClientDataJSON = clientData
	return assertion, nil
}

// SignAssertionWithClientDataHash serves platforms that hash clientDataJSON themselves (iOS); the result has no ClientDataJSON.
func SignAssertionWithClientDataHash(requestedRelyingPartyID, credentialRelyingPartyID string, clientDataHash, privateKeyDER []byte, userVerified bool) (Assertion, error) {
	relyingPartyID, err := ValidateRelyingPartyID(requestedRelyingPartyID)
	if err != nil {
		return Assertion{}, err
	}
	if relyingPartyID != credentialRelyingPartyID {
		return Assertion{}, ErrInvalidRelyingPartyID
	}
	if len(clientDataHash) != ClientDataHashLength {
		return Assertion{}, ErrInvalidRequest
	}
	return signAuthenticatorData(relyingPartyID, clientDataHash, privateKeyDER, userVerified)
}

func signAuthenticatorData(relyingPartyID string, clientDataHash, privateKeyDER []byte, userVerified bool) (Assertion, error) {
	privateKey, err := parseES256Key(privateKeyDER)
	if err != nil {
		return Assertion{}, err
	}
	authenticatorData := BuildAuthenticatorData(relyingPartyID, ceremonyFlags(flagAssertion, userVerified), nil)
	signedData := sha256.Sum256(append(bytes.Clone(authenticatorData), clientDataHash...))
	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, signedData[:])
	if err != nil {
		return Assertion{}, err
	}
	return Assertion{AuthenticatorData: authenticatorData, Signature: signature}, nil
}

func ceremonyFlags(flags byte, userVerified bool) byte {
	if userVerified {
		flags |= FlagUserVerified
	}
	return flags
}

func isPublicSuffix(domain string) bool {
	suffix, _ := publicsuffix.PublicSuffix(domain)
	return suffix == domain
}

func appendCBORHeader(encoded []byte, majorType byte, value uint64) []byte {
	prefix := majorType << 5
	switch {
	case value < 24:
		return append(encoded, prefix|byte(value))
	case value <= 0xff:
		return append(encoded, prefix|24, byte(value))
	case value <= 0xffff:
		return binary.BigEndian.AppendUint16(append(encoded, prefix|25), uint16(value))
	case value <= 0xffffffff:
		return binary.BigEndian.AppendUint32(append(encoded, prefix|26), uint32(value))
	}
	return binary.BigEndian.AppendUint64(append(encoded, prefix|27), value)
}

func appendCBORInteger(encoded []byte, value int64) []byte {
	if value >= 0 {
		return appendCBORHeader(encoded, cborMajorUnsigned, uint64(value))
	}
	return appendCBORHeader(encoded, cborMajorNegative, uint64(-1-value))
}

func appendCBORBytes(encoded, value []byte) []byte {
	return append(appendCBORHeader(encoded, cborMajorBytes, uint64(len(value))), value...)
}

func appendCBORText(encoded []byte, value string) []byte {
	return append(appendCBORHeader(encoded, cborMajorText, uint64(len(value))), value...)
}
