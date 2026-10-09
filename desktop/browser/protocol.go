// Package browser connects the browser extension to the desktop app over native messaging.
package browser

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	ProtocolVersion = 1
	HostName        = "com.tryhisar.hisar"
	ExtensionID     = "mmipiffoogblblocokimpemdopmdjghf"
	AllowedOrigin   = "chrome-extension://" + ExtensionID + "/"
	MaxMessageSize  = 1 << 20
)

const (
	ErrorInvalidRequest     = "invalid-request"
	ErrorNotPaired          = "not-paired"
	ErrorPairingFailed      = "pairing-failed"
	ErrorSignedOut          = "signed-out"
	ErrorLocked             = "locked"
	ErrorOffline            = "offline"
	ErrorNotFound           = "not-found"
	ErrorNoMatch            = "no-match"
	ErrorDesktopUnavailable = "desktop-unavailable"
	ErrorInternal           = "internal"
)

var ErrMessageTooLarge = errors.New("browser: message larger than 1 MiB")

type Request struct {
	ID               string `json:"id"`
	Type             string `json:"type"`
	Token            string `json:"token,omitempty"`
	InstanceID       string `json:"instanceID,omitempty"`
	Browser          string `json:"browser,omitempty"`
	ExtensionVersion string `json:"extensionVersion,omitempty"`
	Code             string `json:"code,omitempty"`
	URL              string `json:"url,omitempty"`
	VaultID          string `json:"vaultID,omitempty"`
	ItemID           string `json:"itemID,omitempty"`
	Username         string `json:"username,omitempty"`
	Password         string `json:"password,omitempty"`
	Length           int    `json:"length,omitempty"`
	Digits           *bool  `json:"digits,omitempty"`
	Symbols          *bool  `json:"symbols,omitempty"`
}

type Response struct {
	ID     string         `json:"id"`
	OK     bool           `json:"ok"`
	Result any            `json:"result,omitempty"`
	Error  *ResponseError `json:"error,omitempty"`
}

type ResponseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ResponseError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

func CreateError(code, message string) *ResponseError {
	return &ResponseError{Code: code, Message: message}
}

type connectMessage struct {
	Type            string `json:"type"`
	ExtensionOrigin string `json:"extensionOrigin"`
}

type HelloResult struct {
	ProtocolVersion int    `json:"protocolVersion"`
	IsPaired        bool   `json:"isPaired"`
	State           string `json:"state"`
}

type TokenResult struct {
	Token string `json:"token"`
}

type StatusResult struct {
	State     string `json:"state"`
	Email     string `json:"email"`
	IsOffline bool   `json:"isOffline"`
}

type Login struct {
	VaultID   string `json:"vaultID"`
	ItemID    string `json:"itemID"`
	Title     string `json:"title"`
	Username  string `json:"username"`
	VaultName string `json:"vaultName"`
}

type MatchResult struct {
	Logins []Login `json:"logins"`
}

type FillResult struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type CheckResult struct {
	Status  string `json:"status"`
	VaultID string `json:"vaultID,omitempty"`
	ItemID  string `json:"itemID,omitempty"`
	Title   string `json:"title,omitempty"`
}

type SaveResult struct {
	VaultID string `json:"vaultID"`
	ItemID  string `json:"itemID"`
}

type PasswordResult struct {
	Password string `json:"password"`
}

// ReadMessage reads one native messaging frame: a 32-bit length in native byte order, then the JSON.
func ReadMessage(reader io.Reader) ([]byte, error) {
	var length uint32
	if err := binary.Read(reader, binary.NativeEndian, &length); err != nil {
		return nil, err
	}
	if length > MaxMessageSize {
		return nil, ErrMessageTooLarge
	}
	message := make([]byte, length)
	if _, err := io.ReadFull(reader, message); err != nil {
		return nil, err
	}
	return message, nil
}

func WriteMessage(writer io.Writer, message []byte) error {
	if len(message) > MaxMessageSize {
		return ErrMessageTooLarge
	}
	frame := make([]byte, 4+len(message))
	binary.NativeEndian.PutUint32(frame, uint32(len(message)))
	copy(frame[4:], message)
	_, err := writer.Write(frame)
	return err
}
