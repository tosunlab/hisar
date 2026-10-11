package api

import (
	"encoding/hex"
	"sort"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/proto"
)

const (
	approvalMaximumLifetime    = 3 * time.Minute
	approvalClockSkew          = 5 * time.Second
	approvalRetention          = 60 * time.Second
	approvalMaximumSealedSize  = 16 << 10
	approvalMaximumBody        = 64 << 10
	maxOpenApprovalPerAccount  = 16
	approvalRequestIDByteCount = 16
	approvalRateEvery          = 500 * time.Millisecond
	approvalRateBurst          = 60
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

// approvalRelay keeps sealed phone approvals in memory; it never sees what they say.
type approvalRelay struct {
	mutex                  sync.Mutex
	now                    func() time.Time
	allApprovalByAccount   map[string]map[string]*storedApproval
	allLimiterByAccount    map[string]*rate.Limiter
	allLimiterLastUsedTime map[string]time.Time
}

func createApprovalRelay() *approvalRelay {
	return &approvalRelay{
		now:                    time.Now,
		allApprovalByAccount:   map[string]map[string]*storedApproval{},
		allLimiterByAccount:    map[string]*rate.Limiter{},
		allLimiterLastUsedTime: map[string]time.Time{},
	}
}

// removeAllExpired drops approvals past their retention; the caller holds the mutex.
func (relay *approvalRelay) removeAllExpired(now time.Time) {
	for accountID, allApproval := range relay.allApprovalByAccount {
		for requestID, approval := range allApproval {
			if now.After(approval.expiresAt.Add(approvalRetention)) {
				delete(allApproval, requestID)
			}
		}
		if len(allApproval) == 0 {
			delete(relay.allApprovalByAccount, accountID)
		}
	}
	for accountID, lastUsedTime := range relay.allLimiterLastUsedTime {
		if now.Sub(lastUsedTime) > 10*time.Minute {
			delete(relay.allLimiterByAccount, accountID)
			delete(relay.allLimiterLastUsedTime, accountID)
		}
	}
}

func (relay *approvalRelay) allow(accountID string) bool {
	relay.mutex.Lock()
	defer relay.mutex.Unlock()
	limiter, isFound := relay.allLimiterByAccount[accountID]
	if !isFound {
		limiter = rate.NewLimiter(rate.Every(approvalRateEvery), approvalRateBurst)
		relay.allLimiterByAccount[accountID] = limiter
	}
	relay.allLimiterLastUsedTime[accountID] = relay.now()
	return limiter.Allow()
}

// find returns an approval of the account that is still within its retention; the caller holds the mutex.
func (relay *approvalRelay) find(accountID, requestID string, now time.Time) *storedApproval {
	approval, isFound := relay.allApprovalByAccount[accountID][requestID]
	if !isFound || now.After(approval.expiresAt.Add(approvalRetention)) {
		return nil
	}
	return approval
}

func isValidApprovalRequestID(requestID string) bool {
	decoded, err := hex.DecodeString(requestID)
	return err == nil && len(decoded) == approvalRequestIDByteCount && hex.EncodeToString(decoded) == requestID
}

func (s *Server) limitApprovalRate(c fiber.Ctx) error {
	if !s.approvals.allow(getSession(c).AccountID) {
		return createAPIError(fiber.StatusTooManyRequests, "too many approval requests")
	}
	return c.Next()
}

func (s *Server) handleCreateApproval(c fiber.Ctx) error {
	var request proto.CreateApprovalRequest
	if err := decodeBodyWithLimit(c, &request, approvalMaximumBody); err != nil {
		return err
	}
	if !isValidApprovalRequestID(request.RequestID) {
		return createAPIError(fiber.StatusBadRequest, "invalid request ID")
	}
	if len(request.SealedRequest) == 0 || len(request.SealedRequest) > approvalMaximumSealedSize {
		return createAPIError(fiber.StatusBadRequest, "invalid sealed request")
	}
	currentSession := getSession(c)
	relay := s.approvals
	relay.mutex.Lock()
	now := relay.now()
	if !request.ExpiresAt.After(now) || request.ExpiresAt.After(now.Add(approvalMaximumLifetime+approvalClockSkew)) {
		relay.mutex.Unlock()
		return createAPIError(fiber.StatusBadRequest, "the approval must expire within 3 minutes")
	}
	relay.removeAllExpired(now)
	allApproval := relay.allApprovalByAccount[currentSession.AccountID]
	if allApproval == nil {
		allApproval = map[string]*storedApproval{}
		relay.allApprovalByAccount[currentSession.AccountID] = allApproval
	}
	if _, isFound := allApproval[request.RequestID]; isFound {
		relay.mutex.Unlock()
		return createAPIError(fiber.StatusConflict, "an approval with this ID exists")
	}
	openCount := 0
	for _, approval := range allApproval {
		if approval.isOpen(now) {
			openCount++
		}
	}
	if openCount >= maxOpenApprovalPerAccount {
		relay.mutex.Unlock()
		return createAPIError(fiber.StatusTooManyRequests, "too many open approvals")
	}
	expiresAt := request.ExpiresAt
	if latestExpiry := now.Add(approvalMaximumLifetime); expiresAt.After(latestExpiry) {
		expiresAt = latestExpiry
	}
	allApproval[request.RequestID] = &storedApproval{
		requestID:         request.RequestID,
		accountID:         currentSession.AccountID,
		requesterDeviceID: currentSession.DeviceID,
		sealedRequest:     request.SealedRequest,
		createdAt:         now,
		expiresAt:         expiresAt,
	}
	relay.mutex.Unlock()
	s.publishApprovalEvent(currentSession.AccountID, proto.EventApprovalRequested, request.RequestID)
	return c.SendStatus(fiber.StatusCreated)
}

func (s *Server) handleListAllPendingApproval(c fiber.Ctx) error {
	accountID := getSession(c).AccountID
	relay := s.approvals
	relay.mutex.Lock()
	now := relay.now()
	allPending := []proto.PendingApproval{}
	for _, approval := range relay.allApprovalByAccount[accountID] {
		if approval.isOpen(now) {
			allPending = append(allPending, proto.PendingApproval{
				RequestID:         approval.requestID,
				SealedRequest:     approval.sealedRequest,
				RequesterDeviceID: approval.requesterDeviceID,
				CreatedAt:         approval.createdAt,
				ExpiresAt:         approval.expiresAt,
			})
		}
	}
	relay.mutex.Unlock()
	sort.Slice(allPending, func(first, second int) bool { return allPending[first].CreatedAt.Before(allPending[second].CreatedAt) })
	return c.JSON(allPending)
}

func (s *Server) handleGetApproval(c fiber.Ctx) error {
	relay := s.approvals
	relay.mutex.Lock()
	defer relay.mutex.Unlock()
	approval := relay.find(getSession(c).AccountID, c.Params("id"), relay.now())
	if approval == nil {
		return errNotFound
	}
	return c.JSON(proto.Approval{
		RequestID:     approval.requestID,
		SealedRequest: approval.sealedRequest,
		SealedAnswer:  approval.sealedAnswer,
		IsResolved:    approval.isResolved,
		ExpiresAt:     approval.expiresAt,
	})
}

func (s *Server) handleAnswerApproval(c fiber.Ctx) error {
	var request proto.AnswerApprovalRequest
	if err := decodeBodyWithLimit(c, &request, approvalMaximumBody); err != nil {
		return err
	}
	if len(request.SealedAnswer) == 0 || len(request.SealedAnswer) > approvalMaximumSealedSize {
		return createAPIError(fiber.StatusBadRequest, "invalid sealed answer")
	}
	currentSession := getSession(c)
	relay := s.approvals
	relay.mutex.Lock()
	now := relay.now()
	approval := relay.find(currentSession.AccountID, c.Params("id"), now)
	var apiError error
	switch {
	case approval == nil:
		apiError = errNotFound
	case approval.requesterDeviceID == currentSession.DeviceID:
		apiError = createAPIError(fiber.StatusForbidden, "a device cannot answer its own approval")
	case approval.sealedAnswer != nil:
		apiError = createAPIError(fiber.StatusConflict, "the approval was already answered")
	case approval.isResolved || !now.Before(approval.expiresAt):
		apiError = createAPIError(fiber.StatusGone, "the approval is no longer open")
	default:
		approval.sealedAnswer = request.SealedAnswer
	}
	relay.mutex.Unlock()
	if apiError != nil {
		return apiError
	}
	s.publishApprovalEvent(currentSession.AccountID, proto.EventApprovalAnswered, approval.requestID)
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) handleResolveApproval(c fiber.Ctx) error {
	currentSession := getSession(c)
	relay := s.approvals
	relay.mutex.Lock()
	approval := relay.find(currentSession.AccountID, c.Params("id"), relay.now())
	if approval == nil {
		relay.mutex.Unlock()
		return errNotFound
	}
	if approval.requesterDeviceID != currentSession.DeviceID {
		relay.mutex.Unlock()
		return createAPIError(fiber.StatusForbidden, "only the requesting device can resolve an approval")
	}
	approval.isResolved = true
	relay.mutex.Unlock()
	s.publishApprovalEvent(currentSession.AccountID, proto.EventApprovalResolved, approval.requestID)
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) publishApprovalEvent(accountID, eventName, requestID string) {
	s.events.publish([]string{accountID}, serverEvent{name: eventName, data: proto.ApprovalEvent{RequestID: requestID}})
}
