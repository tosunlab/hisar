// Package approval seals the Mac's approval requests for the account's phones and checks their signed answers.
package approval

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/crypt"
)

const (
	KindSSH         = "ssh"
	KindCommandLine = "cli"
	// KindLauncher is an app such as Raycast asking to search Hisar until it locks. A phone older than this kind cannot
	// open the request and skips it.
	KindLauncher = "launcher"

	DecisionApprove = "approve"
	DecisionDeny    = "deny"

	MaximumLifetime   = 3 * time.Minute
	MaximumSealedSize = 16 << 10

	requestIDByteCount = 16
	timeLayout         = "2006-01-02T15:04:05.000Z07:00"
)

var (
	ErrInvalidRequest   = errors.New("approval: the request is not valid")
	ErrInvalidAnswer    = errors.New("approval: the answer is not valid")
	ErrExpired          = errors.New("approval: the request expired")
	ErrUnknownRequest   = errors.New("approval: no such request is waiting")
	ErrAlreadyAnswered  = errors.New("approval: the request was already answered")
	ErrSelfAnswer       = errors.New("approval: a device cannot answer its own request")
	ErrOwnRequest       = errors.New("approval: the request came from this device")
	ErrRequestMismatch  = errors.New("approval: the answer is for another request")
	ErrUnknownDecision  = errors.New("approval: unknown decision")
	ErrMissingDeviceID  = errors.New("approval: the device ID is unknown")
	ErrLifetimeTooLong  = errors.New("approval: the request lasts longer than allowed")
	ErrSealedTooLarge   = errors.New("approval: the sealed message is too large")
	ErrMissingRequestID = errors.New("approval: the request ID is missing or malformed")
)

// Description is what the Mac's approval dialog shows; empty fields are lines the dialog does not have.
type Description struct {
	Kind           string `json:"kind"`
	AppName        string `json:"appName"`
	Purpose        string `json:"purpose"`
	Host           string `json:"host"`
	CommandLine    string `json:"commandLine"`
	RepositoryPath string `json:"repositoryPath"`
	// ProjectPath is the folder of the coding agent's harness, such as omp's project, for every agent request.
	ProjectPath       string `json:"projectPath,omitempty"`
	KeyName           string `json:"keyName"`
	IsForwarded       bool   `json:"isForwarded"`
	ForwardedFromHost string `json:"forwardedFromHost"`
	TerminalName      string `json:"terminalName"`
	// Secrets are the secrets a command-line request reads, as the dialog lists them.
	Secrets []string `json:"secrets,omitempty"`
	// AppIcon is the app's small PNG icon; AppWarning is the dialog's warning about the app's code signature.
	AppIcon    []byte `json:"appIcon,omitempty"`
	AppWarning string `json:"appWarning,omitempty"`
	// RememberLabel is set when approving can also be remembered: the sentence for the phone's switch.
	RememberLabel         string `json:"rememberLabel,omitempty"`
	IsRememberedByDefault bool   `json:"isRememberedByDefault,omitempty"`
}

type Request struct {
	RequestID           string    `json:"requestId"`
	RequesterDeviceID   string    `json:"requesterDeviceId"`
	RequesterDeviceName string    `json:"requesterDeviceName"`
	CreatedAt           time.Time `json:"createdAt"`
	ExpiresAt           time.Time `json:"expiresAt"`
	Description
}

type Answer struct {
	RequestID           string `json:"requestId"`
	Decision            string `json:"decision"`
	AnsweringDeviceID   string `json:"answeringDeviceId"`
	AnsweringDeviceName string `json:"answeringDeviceName"`
	AnsweredAt          string `json:"answeredAt"`
	RequestHash         []byte `json:"requestHash"`
	Signature           []byte `json:"signature"`
	// IsRemembered is the phone's remember switch; nil for an answer to a request without one, or from an older phone.
	IsRemembered *bool `json:"isRemembered,omitempty"`
}

// AcceptedAnswer is a verified answer; IsRemembered is true only when the phone chose to remember and signed that choice.
type AcceptedAnswer struct {
	Decision     string
	IsRemembered bool
}

func getRememberChoice(isRemembered *bool) string {
	switch {
	case isRemembered == nil:
		return ""
	case *isRemembered:
		return crypt.ApprovalRememberChoice
	}
	return crypt.ApprovalOnceChoice
}

func GenerateRequestID() string {
	randomBytes := make([]byte, requestIDByteCount)
	rand.Read(randomBytes)
	return hex.EncodeToString(randomBytes)
}

func IsValidRequestID(requestID string) bool {
	decoded, err := hex.DecodeString(requestID)
	return err == nil && len(decoded) == requestIDByteCount && hex.EncodeToString(decoded) == requestID
}

func FormatTime(moment time.Time) string {
	return moment.UTC().Format(timeLayout)
}

// NewRequest builds a request that lasts lifetime, at most MaximumLifetime, from now.
func NewRequest(requesterDeviceID, requesterDeviceName string, description Description, now time.Time, lifetime time.Duration) Request {
	createdAt := now.UTC().Truncate(time.Millisecond)
	return Request{
		RequestID:           GenerateRequestID(),
		RequesterDeviceID:   requesterDeviceID,
		RequesterDeviceName: requesterDeviceName,
		CreatedAt:           createdAt,
		ExpiresAt:           createdAt.Add(min(lifetime, MaximumLifetime)),
		Description:         description,
	}
}

func checkRequest(request Request) error {
	if !IsValidRequestID(request.RequestID) {
		return ErrMissingRequestID
	}
	if request.RequesterDeviceID == "" {
		return ErrMissingDeviceID
	}
	if !request.ExpiresAt.After(request.CreatedAt) || request.ExpiresAt.Sub(request.CreatedAt) > MaximumLifetime {
		return ErrLifetimeTooLong
	}
	if request.Kind != KindSSH && request.Kind != KindCommandLine && request.Kind != KindLauncher {
		return ErrInvalidRequest
	}
	return nil
}

// SealRequest encrypts the request to the account's own KEM key, so only the account's unlocked devices can read it.
func SealRequest(keyset *crypt.Keyset, accountID string, request Request) ([]byte, error) {
	if err := checkRequest(request); err != nil {
		return nil, err
	}
	plaintext, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	sealedRequest, err := crypt.SealApprovalRequest(keyset.GetKEMPublicKey(), accountID, request.RequestID, plaintext)
	if err != nil {
		return nil, err
	}
	if len(sealedRequest) > MaximumSealedSize {
		if len(request.AppIcon) > 0 {
			request.AppIcon = nil
			return SealRequest(keyset, accountID, request)
		}
		return nil, ErrSealedTooLarge
	}
	return sealedRequest, nil
}

// decodeMessage ignores unknown fields, so a newer device's optional fields do not break an older one; the answer's
// signature binds the hash of the exact sealed request, and only the fields decoded here are acted on.
func decodeMessage(plaintext []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(plaintext))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.More() {
		return errors.New("approval: data after the message")
	}
	return nil
}

// OpenRequest opens a request the server listed under requestID and refuses expired requests and this device's own.
func OpenRequest(keyset *crypt.Keyset, accountID, requestID string, sealedRequest []byte, ownDeviceID string, now time.Time) (Request, error) {
	if !IsValidRequestID(requestID) {
		return Request{}, ErrMissingRequestID
	}
	if len(sealedRequest) > MaximumSealedSize {
		return Request{}, ErrSealedTooLarge
	}
	plaintext, err := keyset.OpenApprovalRequest(accountID, requestID, sealedRequest)
	if err != nil {
		return Request{}, err
	}
	var request Request
	if err := decodeMessage(plaintext, &request); err != nil {
		return Request{}, ErrInvalidRequest
	}
	if request.RequestID != requestID {
		return Request{}, ErrRequestMismatch
	}
	if err := checkRequest(request); err != nil {
		return Request{}, err
	}
	if !now.Before(request.ExpiresAt) {
		return Request{}, ErrExpired
	}
	if ownDeviceID == "" {
		return Request{}, ErrMissingDeviceID
	}
	if request.RequesterDeviceID == ownDeviceID {
		return Request{}, ErrOwnRequest
	}
	return request, nil
}

// CreateAnswer signs the decision over the exact sealed request and seals the signed answer to the account.
func CreateAnswer(keyset *crypt.Keyset, accountID string, sealedRequest []byte, request Request, decision, answeringDeviceID, answeringDeviceName string, now time.Time) ([]byte, error) {
	return CreateRememberingAnswer(keyset, accountID, sealedRequest, request, decision, false, answeringDeviceID, answeringDeviceName, now)
}

// CreateRememberingAnswer also signs the phone's remember switch, but only for a request that offers one,
// so a Mac that never offers it verifies the answer as before.
func CreateRememberingAnswer(keyset *crypt.Keyset, accountID string, sealedRequest []byte, request Request, decision string, isRemembered bool, answeringDeviceID, answeringDeviceName string, now time.Time) ([]byte, error) {
	if decision != DecisionApprove && decision != DecisionDeny {
		return nil, ErrUnknownDecision
	}
	if answeringDeviceID == "" {
		return nil, ErrMissingDeviceID
	}
	if answeringDeviceID == request.RequesterDeviceID {
		return nil, ErrSelfAnswer
	}
	if !now.Before(request.ExpiresAt) {
		return nil, ErrExpired
	}
	var rememberedAnswer *bool
	if request.RememberLabel != "" {
		isApprovedAndRemembered := isRemembered && decision == DecisionApprove
		rememberedAnswer = &isApprovedAndRemembered
	}
	fields := crypt.ApprovalAnswerFields{
		AccountID:           accountID,
		RequestID:           request.RequestID,
		RequestHash:         crypt.HashApprovalRequest(sealedRequest),
		Decision:            decision,
		AnsweringDeviceID:   answeringDeviceID,
		AnsweringDeviceName: answeringDeviceName,
		AnsweredAt:          FormatTime(now),
		RememberChoice:      getRememberChoice(rememberedAnswer),
	}
	plaintext, err := json.Marshal(Answer{
		RequestID:           fields.RequestID,
		Decision:            fields.Decision,
		AnsweringDeviceID:   fields.AnsweringDeviceID,
		AnsweringDeviceName: fields.AnsweringDeviceName,
		AnsweredAt:          fields.AnsweredAt,
		RequestHash:         fields.RequestHash,
		Signature:           keyset.SignApprovalAnswer(fields),
		IsRemembered:        rememberedAnswer,
	})
	if err != nil {
		return nil, err
	}
	return crypt.SealApprovalAnswer(keyset.GetKEMPublicKey(), accountID, request.RequestID, plaintext)
}

type trackedRequest struct {
	requestHash []byte
	expiresAt   time.Time
	isAnswered  bool
}

// Tracker holds the requests this device posted and accepts at most one valid answer for each.
type Tracker struct {
	mutex           sync.Mutex
	pendingRequests map[string]*trackedRequest
}

func NewTracker() *Tracker {
	return &Tracker{pendingRequests: map[string]*trackedRequest{}}
}

func (tracker *Tracker) Add(requestID string, sealedRequest []byte, expiresAt time.Time) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	tracker.pendingRequests[requestID] = &trackedRequest{requestHash: crypt.HashApprovalRequest(sealedRequest), expiresAt: expiresAt}
}

func (tracker *Tracker) Remove(requestID string) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	delete(tracker.pendingRequests, requestID)
}

func (tracker *Tracker) RemoveAll() {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	clear(tracker.pendingRequests)
}

// Accept returns the decision of a valid answer, see AcceptAnswer.
func (tracker *Tracker) Accept(keyset *crypt.Keyset, accountID, requestID string, sealedAnswer []byte, ownDeviceID string, now time.Time) (string, error) {
	accepted, err := tracker.AcceptAnswer(keyset, accountID, requestID, sealedAnswer, ownDeviceID, now)
	return accepted.Decision, err
}

// AcceptAnswer returns a valid answer: the account's own signature over this exact request, and over the remember
// choice when the answer has one, before it expires, from another device, and only once.
func (tracker *Tracker) AcceptAnswer(keyset *crypt.Keyset, accountID, requestID string, sealedAnswer []byte, ownDeviceID string, now time.Time) (AcceptedAnswer, error) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	pending, isFound := tracker.pendingRequests[requestID]
	if !isFound {
		return AcceptedAnswer{}, ErrUnknownRequest
	}
	if pending.isAnswered {
		return AcceptedAnswer{}, ErrAlreadyAnswered
	}
	if !now.Before(pending.expiresAt) {
		return AcceptedAnswer{}, ErrExpired
	}
	if len(sealedAnswer) > MaximumSealedSize {
		return AcceptedAnswer{}, ErrSealedTooLarge
	}
	plaintext, err := keyset.OpenApprovalAnswer(accountID, requestID, sealedAnswer)
	if err != nil {
		return AcceptedAnswer{}, err
	}
	var answer Answer
	if err := decodeMessage(plaintext, &answer); err != nil {
		return AcceptedAnswer{}, ErrInvalidAnswer
	}
	if answer.RequestID != requestID || subtle.ConstantTimeCompare(answer.RequestHash, pending.requestHash) != 1 {
		return AcceptedAnswer{}, ErrRequestMismatch
	}
	if answer.Decision != DecisionApprove && answer.Decision != DecisionDeny {
		return AcceptedAnswer{}, ErrUnknownDecision
	}
	if ownDeviceID == "" {
		return AcceptedAnswer{}, ErrMissingDeviceID
	}
	if answer.AnsweringDeviceID == "" || answer.AnsweringDeviceID == ownDeviceID {
		return AcceptedAnswer{}, ErrSelfAnswer
	}
	fields := crypt.ApprovalAnswerFields{
		AccountID:           accountID,
		RequestID:           answer.RequestID,
		RequestHash:         answer.RequestHash,
		Decision:            answer.Decision,
		AnsweringDeviceID:   answer.AnsweringDeviceID,
		AnsweringDeviceName: answer.AnsweringDeviceName,
		AnsweredAt:          answer.AnsweredAt,
		RememberChoice:      getRememberChoice(answer.IsRemembered),
	}
	if err := crypt.VerifyApprovalAnswer(keyset.GetSigningPublicKey(), fields, answer.Signature); err != nil {
		return AcceptedAnswer{}, err
	}
	pending.isAnswered = true
	isRemembered := answer.Decision == DecisionApprove && answer.IsRemembered != nil && *answer.IsRemembered
	return AcceptedAnswer{Decision: answer.Decision, IsRemembered: isRemembered}, nil
}
