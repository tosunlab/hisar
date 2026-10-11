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
	Kind              string `json:"kind"`
	AppName           string `json:"appName"`
	Purpose           string `json:"purpose"`
	Host              string `json:"host"`
	CommandLine       string `json:"commandLine"`
	RepositoryPath    string `json:"repositoryPath"`
	KeyName           string `json:"keyName"`
	IsForwarded       bool   `json:"isForwarded"`
	ForwardedFromHost string `json:"forwardedFromHost"`
	TerminalName      string `json:"terminalName"`
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

// CreateRequest builds a request that lasts lifetime, at most MaximumLifetime, from now.
func CreateRequest(requesterDeviceID, requesterDeviceName string, description Description, now time.Time, lifetime time.Duration) Request {
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
	if request.Kind != KindSSH && request.Kind != KindCommandLine {
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
		return nil, ErrSealedTooLarge
	}
	return sealedRequest, nil
}

func decodeStrictly(plaintext []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(plaintext))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
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
	if err := decodeStrictly(plaintext, &request); err != nil {
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
	fields := crypt.ApprovalAnswerFields{
		AccountID:           accountID,
		RequestID:           request.RequestID,
		RequestHash:         crypt.HashApprovalRequest(sealedRequest),
		Decision:            decision,
		AnsweringDeviceID:   answeringDeviceID,
		AnsweringDeviceName: answeringDeviceName,
		AnsweredAt:          FormatTime(now),
	}
	plaintext, err := json.Marshal(Answer{
		RequestID:           fields.RequestID,
		Decision:            fields.Decision,
		AnsweringDeviceID:   fields.AnsweringDeviceID,
		AnsweringDeviceName: fields.AnsweringDeviceName,
		AnsweredAt:          fields.AnsweredAt,
		RequestHash:         fields.RequestHash,
		Signature:           keyset.SignApprovalAnswer(fields),
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
	mutex      sync.Mutex
	allPending map[string]*trackedRequest
}

func CreateTracker() *Tracker {
	return &Tracker{allPending: map[string]*trackedRequest{}}
}

func (tracker *Tracker) Add(requestID string, sealedRequest []byte, expiresAt time.Time) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	tracker.allPending[requestID] = &trackedRequest{requestHash: crypt.HashApprovalRequest(sealedRequest), expiresAt: expiresAt}
}

func (tracker *Tracker) Remove(requestID string) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	delete(tracker.allPending, requestID)
}

func (tracker *Tracker) RemoveAll() {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	clear(tracker.allPending)
}

// Accept returns the decision of a valid answer: the account's own signature over this exact request,
// before it expires, from another device, and only once.
func (tracker *Tracker) Accept(keyset *crypt.Keyset, accountID, requestID string, sealedAnswer []byte, ownDeviceID string, now time.Time) (string, error) {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	pending, isFound := tracker.allPending[requestID]
	if !isFound {
		return "", ErrUnknownRequest
	}
	if pending.isAnswered {
		return "", ErrAlreadyAnswered
	}
	if !now.Before(pending.expiresAt) {
		return "", ErrExpired
	}
	if len(sealedAnswer) > MaximumSealedSize {
		return "", ErrSealedTooLarge
	}
	plaintext, err := keyset.OpenApprovalAnswer(accountID, requestID, sealedAnswer)
	if err != nil {
		return "", err
	}
	var answer Answer
	if err := decodeStrictly(plaintext, &answer); err != nil {
		return "", ErrInvalidAnswer
	}
	if answer.RequestID != requestID || subtle.ConstantTimeCompare(answer.RequestHash, pending.requestHash) != 1 {
		return "", ErrRequestMismatch
	}
	if answer.Decision != DecisionApprove && answer.Decision != DecisionDeny {
		return "", ErrUnknownDecision
	}
	if ownDeviceID == "" {
		return "", ErrMissingDeviceID
	}
	if answer.AnsweringDeviceID == "" || answer.AnsweringDeviceID == ownDeviceID {
		return "", ErrSelfAnswer
	}
	fields := crypt.ApprovalAnswerFields{
		AccountID:           accountID,
		RequestID:           answer.RequestID,
		RequestHash:         answer.RequestHash,
		Decision:            answer.Decision,
		AnsweringDeviceID:   answer.AnsweringDeviceID,
		AnsweringDeviceName: answer.AnsweringDeviceName,
		AnsweredAt:          answer.AnsweredAt,
	}
	if err := crypt.VerifyApprovalAnswer(keyset.GetSigningPublicKey(), fields, answer.Signature); err != nil {
		return "", err
	}
	pending.isAnswered = true
	return answer.Decision, nil
}
