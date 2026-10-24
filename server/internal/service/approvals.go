package service

import (
	"context"
	"encoding/hex"
	"sort"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/ratelimit"
	"github.com/tryhisar/hisar/server/internal/realtime"
)

const (
	ApprovalMaximumLifetime    = 3 * time.Minute
	approvalClockSkew          = 5 * time.Second
	ApprovalRetention          = 60 * time.Second
	ApprovalMaximumSealedSize  = 16 << 10
	ApprovalMaximumBody        = 64 << 10
	MaxOpenApprovalPerAccount  = 16
	approvalRequestIDByteCount = 16
	approvalRateEvery          = 500 * time.Millisecond
	ApprovalRateBurst          = 60
)

type storedApproval struct {
	requestID         string
	accountID         string
	requesterDeviceID string
	sealedRequest     []byte
	sealedAnswer      []byte
	createdAt         time.Time
	expiresAt         time.Time
	isResolved        bool
}

func (approval *storedApproval) isOpen(now time.Time) bool {
	return approval.sealedAnswer == nil && !approval.isResolved && now.Before(approval.expiresAt)
}

// ApprovalRelay keeps sealed phone approvals in memory; it never sees what they say.
type ApprovalRelay struct {
	mutex              sync.Mutex
	now                func() time.Time
	approvalsByAccount map[string]map[string]*storedApproval
	limiter            *ratelimit.Limiter
}

func newApprovalRelay(limiter *ratelimit.Limiter) *ApprovalRelay {
	return &ApprovalRelay{
		now:                time.Now,
		approvalsByAccount: map[string]map[string]*storedApproval{},
		limiter:            limiter,
	}
}

// SetClock replaces the relay's clock, for tests.
func (relay *ApprovalRelay) SetClock(now func() time.Time) {
	relay.mutex.Lock()
	defer relay.mutex.Unlock()
	relay.now = now
}

// ResetRateLimits forgets every account's rate, for tests.
func (relay *ApprovalRelay) ResetRateLimits() {
	relay.mutex.Lock()
	defer relay.mutex.Unlock()
	relay.limiter = ratelimit.New(rate.Every(approvalRateEvery), ApprovalRateBurst)
}

// IsKept reports whether the relay still holds an approval, for tests.
func (relay *ApprovalRelay) IsKept(accountID, requestID string) bool {
	relay.mutex.Lock()
	defer relay.mutex.Unlock()
	_, isKept := relay.approvalsByAccount[accountID][requestID]
	return isKept
}

// removeExpired drops approvals past their retention; the caller holds the mutex.
func (relay *ApprovalRelay) removeExpired(now time.Time) {
	for accountID, approvals := range relay.approvalsByAccount {
		for requestID, approval := range approvals {
			if now.After(approval.expiresAt.Add(ApprovalRetention)) {
				delete(approvals, requestID)
			}
		}
		if len(approvals) == 0 {
			delete(relay.approvalsByAccount, accountID)
		}
	}
}

// Run drops approvals past their retention every interval until ctx ends, so a sealed request does not stay in memory
// for want of a new approval to sweep it.
func (relay *ApprovalRelay) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			relay.mutex.Lock()
			relay.removeExpired(relay.now())
			relay.mutex.Unlock()
			relay.limiter.Maintain()
		}
	}
}

// Allow limits how fast one account may use approvals.
func (relay *ApprovalRelay) Allow(accountID string) bool {
	relay.mutex.Lock()
	defer relay.mutex.Unlock()
	return relay.limiter.Allow(accountID)
}

func (relay *ApprovalRelay) removeForAccount(accountID string) {
	relay.mutex.Lock()
	defer relay.mutex.Unlock()
	delete(relay.approvalsByAccount, accountID)
}

// find returns an approval of the account that is still within its retention; the caller holds the mutex.
func (relay *ApprovalRelay) find(accountID, requestID string, now time.Time) *storedApproval {
	approval, isFound := relay.approvalsByAccount[accountID][requestID]
	if !isFound || now.After(approval.expiresAt.Add(ApprovalRetention)) {
		return nil
	}
	return approval
}

func isValidApprovalRequestID(requestID string) bool {
	decoded, err := hex.DecodeString(requestID)
	return err == nil && len(decoded) == approvalRequestIDByteCount && hex.EncodeToString(decoded) == requestID
}

func (s *Service) publishApprovalEvent(accountID, eventName, requestID string) {
	s.events.Publish([]string{accountID}, realtime.Event{Name: eventName, Data: apitypes.ApprovalEvent{RequestID: requestID}})
}

// CreateApproval stores a sealed request of the caller's device for the account's other devices.
func (s *Service) CreateApproval(actor Actor, request *apitypes.CreateApprovalRequest) error {
	if !isValidApprovalRequestID(request.RequestID) {
		return newError(KindInvalid, "invalid request ID")
	}
	if len(request.SealedRequest) == 0 || len(request.SealedRequest) > ApprovalMaximumSealedSize {
		return newError(KindInvalid, "invalid sealed request")
	}
	relay := s.Approvals
	relay.mutex.Lock()
	now := relay.now()
	if !request.ExpiresAt.After(now) || request.ExpiresAt.After(now.Add(ApprovalMaximumLifetime+approvalClockSkew)) {
		relay.mutex.Unlock()
		return newError(KindInvalid, "the approval must expire within 3 minutes")
	}
	relay.removeExpired(now)
	approvals := relay.approvalsByAccount[actor.AccountID]
	if approvals == nil {
		approvals = map[string]*storedApproval{}
		relay.approvalsByAccount[actor.AccountID] = approvals
	}
	if _, isFound := approvals[request.RequestID]; isFound {
		relay.mutex.Unlock()
		return newError(KindConflict, "an approval with this ID exists")
	}
	openCount := 0
	for _, approval := range approvals {
		if approval.isOpen(now) {
			openCount++
		}
	}
	if openCount >= MaxOpenApprovalPerAccount {
		relay.mutex.Unlock()
		return newError(KindTooManyRequests, "too many open approvals")
	}
	expiresAt := request.ExpiresAt
	if latestExpiry := now.Add(ApprovalMaximumLifetime); expiresAt.After(latestExpiry) {
		expiresAt = latestExpiry
	}
	approvals[request.RequestID] = &storedApproval{
		requestID:         request.RequestID,
		accountID:         actor.AccountID,
		requesterDeviceID: actor.DeviceID,
		sealedRequest:     request.SealedRequest,
		createdAt:         now,
		expiresAt:         expiresAt,
	}
	relay.mutex.Unlock()
	s.publishApprovalEvent(actor.AccountID, apitypes.EventApprovalRequested, request.RequestID)
	return nil
}

// ListPendingApprovals returns the account's open approvals, oldest first.
func (s *Service) ListPendingApprovals(actor Actor) []apitypes.PendingApproval {
	relay := s.Approvals
	relay.mutex.Lock()
	now := relay.now()
	pendingRequests := []apitypes.PendingApproval{}
	for _, approval := range relay.approvalsByAccount[actor.AccountID] {
		if approval.isOpen(now) {
			pendingRequests = append(pendingRequests, apitypes.PendingApproval{
				RequestID:         approval.requestID,
				SealedRequest:     approval.sealedRequest,
				RequesterDeviceID: approval.requesterDeviceID,
				CreatedAt:         approval.createdAt,
				ExpiresAt:         approval.expiresAt,
			})
		}
	}
	relay.mutex.Unlock()
	sort.Slice(pendingRequests, func(first, second int) bool {
		return pendingRequests[first].CreatedAt.Before(pendingRequests[second].CreatedAt)
	})
	return pendingRequests
}

func (s *Service) GetApproval(actor Actor, requestID string) (apitypes.Approval, error) {
	relay := s.Approvals
	relay.mutex.Lock()
	defer relay.mutex.Unlock()
	approval := relay.find(actor.AccountID, requestID, relay.now())
	if approval == nil {
		return apitypes.Approval{}, ErrNotFound
	}
	return apitypes.Approval{
		RequestID:     approval.requestID,
		SealedRequest: approval.sealedRequest,
		SealedAnswer:  approval.sealedAnswer,
		IsResolved:    approval.isResolved,
		ExpiresAt:     approval.expiresAt,
	}, nil
}

// AnswerApproval stores another device's sealed answer, once, while the approval is open.
func (s *Service) AnswerApproval(actor Actor, requestID string, request *apitypes.AnswerApprovalRequest) error {
	if len(request.SealedAnswer) == 0 || len(request.SealedAnswer) > ApprovalMaximumSealedSize {
		return newError(KindInvalid, "invalid sealed answer")
	}
	relay := s.Approvals
	relay.mutex.Lock()
	now := relay.now()
	approval := relay.find(actor.AccountID, requestID, now)
	var refusal error
	switch {
	case approval == nil:
		refusal = ErrNotFound
	case approval.requesterDeviceID == actor.DeviceID:
		refusal = newError(KindForbidden, "a device cannot answer its own approval")
	case approval.sealedAnswer != nil:
		refusal = newError(KindConflict, "the approval was already answered")
	case approval.isResolved || !now.Before(approval.expiresAt):
		refusal = newError(KindGone, "the approval is no longer open")
	default:
		approval.sealedAnswer = request.SealedAnswer
	}
	relay.mutex.Unlock()
	if refusal != nil {
		return refusal
	}
	s.publishApprovalEvent(actor.AccountID, apitypes.EventApprovalAnswered, approval.requestID)
	return nil
}

// ResolveApproval closes an approval; only the device that asked may.
func (s *Service) ResolveApproval(actor Actor, requestID string) error {
	relay := s.Approvals
	relay.mutex.Lock()
	approval := relay.find(actor.AccountID, requestID, relay.now())
	if approval == nil {
		relay.mutex.Unlock()
		return ErrNotFound
	}
	if approval.requesterDeviceID != actor.DeviceID {
		relay.mutex.Unlock()
		return newError(KindForbidden, "only the requesting device can resolve an approval")
	}
	approval.isResolved = true
	relay.mutex.Unlock()
	s.publishApprovalEvent(actor.AccountID, apitypes.EventApprovalResolved, approval.requestID)
	return nil
}
