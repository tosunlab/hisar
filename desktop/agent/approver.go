// Package agent runs the Hisar SSH agent inside the desktop app: approvals, the socket and ~/.ssh/config.
package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/sshagent"
)

const (
	ApprovalEventName       = "ssh-approval"
	ApprovalClosedEventName = "ssh-approval-closed"
	defaultApprovalTimeout  = 60 * time.Second
)

type Biometric interface {
	IsAvailable() bool
	Authenticate(reason string) error
}

// ApprovalEvent is what the in-app approval prompt shows.
type ApprovalEvent struct {
	RequestID              string `json:"requestID"`
	ClientName             string `json:"clientName"`
	ClientPath             string `json:"clientPath"`
	KeyTitle               string `json:"keyTitle"`
	Fingerprint            string `json:"fingerprint"`
	DestinationHost        string `json:"destinationHost"`
	DestinationFingerprint string `json:"destinationFingerprint"`
	IsForwarded            bool   `json:"isForwarded"`
}

type ApproverConfig struct {
	Biometric Biometric
	// Emit sends a UI event: ApprovalEventName with an ApprovalEvent, ApprovalClosedEventName with the request ID.
	Emit       func(eventName string, data any)
	ShowWindow func()
	Timeout    time.Duration
}

// Approver asks the user before the agent signs: with Touch ID when available, otherwise in the app window.
type Approver struct {
	config         ApproverConfig
	biometricMutex sync.Mutex
	mutex          sync.Mutex
	allPendingByID map[string]chan bool
}

func CreateApprover(config ApproverConfig) *Approver {
	if config.Timeout == 0 {
		config.Timeout = defaultApprovalTimeout
	}
	return &Approver{config: config, allPendingByID: map[string]chan bool{}}
}

func (approver *Approver) IsTouchIDApproval() bool {
	return approver.config.Biometric != nil && approver.config.Biometric.IsAvailable()
}

func (approver *Approver) Approve(ctx context.Context, request sshagent.ApprovalRequest) (bool, error) {
	if approver.IsTouchIDApproval() {
		return approver.approveWithBiometric(request), nil
	}
	return approver.approveInApp(ctx, request)
}

func buildApprovalReason(request sshagent.ApprovalRequest) string {
	target := request.DestinationHost
	if target == "" {
		target = request.DestinationFingerprint
	}
	if target == "" {
		target = request.Fingerprint
	}
	reason := fmt.Sprintf("allow %q to use the SSH key %q for %s", request.ClientName, request.KeyTitle, target)
	if request.IsForwarded {
		reason += " through a forwarded agent"
	}
	return reason
}

// approveWithBiometric shows one Touch ID prompt at a time; any failure or cancel denies.
func (approver *Approver) approveWithBiometric(request sshagent.ApprovalRequest) bool {
	approver.biometricMutex.Lock()
	defer approver.biometricMutex.Unlock()
	return approver.config.Biometric.Authenticate(buildApprovalReason(request)) == nil
}

func generateRequestID() string {
	randomBytes := make([]byte, 16)
	rand.Read(randomBytes)
	return hex.EncodeToString(randomBytes)
}

func (approver *Approver) approveInApp(ctx context.Context, request sshagent.ApprovalRequest) (bool, error) {
	if approver.config.Emit == nil {
		return false, nil
	}
	requestID := generateRequestID()
	responseChannel := make(chan bool, 1)
	approver.mutex.Lock()
	approver.allPendingByID[requestID] = responseChannel
	approver.mutex.Unlock()
	defer func() {
		approver.mutex.Lock()
		delete(approver.allPendingByID, requestID)
		approver.mutex.Unlock()
	}()

	approver.config.Emit(ApprovalEventName, ApprovalEvent{
		RequestID:              requestID,
		ClientName:             request.ClientName,
		ClientPath:             request.ClientPath,
		KeyTitle:               request.KeyTitle,
		Fingerprint:            request.Fingerprint,
		DestinationHost:        request.DestinationHost,
		DestinationFingerprint: request.DestinationFingerprint,
		IsForwarded:            request.IsForwarded,
	})
	if approver.config.ShowWindow != nil {
		approver.config.ShowWindow()
	}
	timer := time.NewTimer(approver.config.Timeout)
	defer timer.Stop()
	select {
	case isAllowed := <-responseChannel:
		return isAllowed, nil
	case <-timer.C:
	case <-ctx.Done():
	}
	approver.config.Emit(ApprovalClosedEventName, requestID)
	return false, nil
}

// Respond answers a pending in-app approval; it returns false for an unknown or finished request.
func (approver *Approver) Respond(requestID string, isAllowed bool) bool {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	responseChannel, found := approver.allPendingByID[requestID]
	if !found {
		return false
	}
	delete(approver.allPendingByID, requestID)
	responseChannel <- isAllowed
	return true
}
