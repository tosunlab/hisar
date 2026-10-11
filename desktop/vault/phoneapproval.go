package vault

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/approval"
	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/proto"
)

const (
	phoneApprovalDeviceRefreshInterval = 5 * time.Minute
	phoneApprovalRequestTimeout        = 15 * time.Second
	phoneApprovalDeviceCheckTimeout    = 3 * time.Second
	phoneApprovalPostAttemptCount      = 3
	phoneApprovalPostRetryDelay        = 2 * time.Second
	iosPlatformPrefix                  = "ios/"
)

var errPhoneApprovalGone = errors.New("this request is no longer waiting for an answer")

// phoneApprovalRelay is the backend's part of phone approvals: the requests this device posted and what it knows about its devices.
type phoneApprovalRelay struct {
	mutex                sync.Mutex
	isRequester          bool
	ownDeviceID          string
	hasIOSDevice         bool
	deviceRefreshedAt    time.Time
	isRefreshing         bool
	tracker              *approval.Tracker
	allDecideByRequestID map[string]func(isApproved bool)
}

func createPhoneApprovalRelay() *phoneApprovalRelay {
	return &phoneApprovalRelay{tracker: approval.CreateTracker(), allDecideByRequestID: map[string]func(isApproved bool){}}
}

// PhoneApprovalRequest is a Mac's request as the phone shows it; times are RFC 3339.
type PhoneApprovalRequest struct {
	RequestID           string `json:"requestId"`
	RequesterDeviceName string `json:"requesterDeviceName"`
	CreatedAt           string `json:"createdAt"`
	ExpiresAt           string `json:"expiresAt"`
	approval.Description
}

// PhoneApprover lets the approval window send its requests to the account's iPhones; it is not bound to the UI.
type PhoneApprover struct {
	service *Service
}

func CreatePhoneApprover(service *Service) *PhoneApprover {
	relay := service.phoneApprovals
	relay.mutex.Lock()
	relay.isRequester = true
	relay.mutex.Unlock()
	return &PhoneApprover{service: service}
}

// forgetPhoneApprovalSession drops what belongs to the session that ends and withdraws its open requests.
func (s *Service) forgetPhoneApprovalSession() {
	relay := s.phoneApprovals
	relay.mutex.Lock()
	allRequestID := make([]string, 0, len(relay.allDecideByRequestID))
	for requestID := range relay.allDecideByRequestID {
		allRequestID = append(allRequestID, requestID)
	}
	clear(relay.allDecideByRequestID)
	relay.tracker.RemoveAll()
	relay.ownDeviceID, relay.hasIOSDevice, relay.deviceRefreshedAt = "", false, time.Time{}
	relay.mutex.Unlock()
	if len(allRequestID) == 0 || s.unlockedSession == nil {
		return
	}
	apiClient := s.unlockedSession.Client
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), phoneApprovalRequestTimeout)
		defer cancel()
		for _, requestID := range allRequestID {
			apiClient.ResolveApproval(ctx, requestID)
		}
	}()
}

func (s *Service) startPhoneApprovalDeviceRefresh(apiClient *client.Client) {
	relay := s.phoneApprovals
	relay.mutex.Lock()
	if relay.isRefreshing {
		relay.mutex.Unlock()
		return
	}
	relay.isRefreshing = true
	relay.mutex.Unlock()
	go s.refreshPhoneApprovalDevice(apiClient)
}

// refreshPhoneApprovalDevice learns this device's ID and whether the account has an iPhone.
func (s *Service) refreshPhoneApprovalDevice(apiClient *client.Client) (string, error) {
	return s.refreshPhoneApprovalDeviceWithin(apiClient, phoneApprovalRequestTimeout)
}

func (s *Service) refreshPhoneApprovalDeviceWithin(apiClient *client.Client, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	allDevice, err := apiClient.ListAllDevice(ctx)
	s.mutex.Lock()
	defer s.mutex.Unlock()
	relay := s.phoneApprovals
	relay.mutex.Lock()
	defer relay.mutex.Unlock()
	relay.isRefreshing = false
	if err != nil {
		return "", convertError(err)
	}
	if s.unlockedSession == nil || s.unlockedSession.Client != apiClient {
		return "", ErrLocked
	}
	relay.ownDeviceID, relay.hasIOSDevice = "", false
	for _, device := range allDevice {
		switch {
		case device.Current:
			relay.ownDeviceID = device.ID
		case strings.HasPrefix(device.Platform, iosPlatformPrefix):
			relay.hasIOSDevice = true
		}
	}
	relay.deviceRefreshedAt = s.config.Now()
	if relay.ownDeviceID == "" {
		return "", errors.New("this device is missing from the account's devices")
	}
	return relay.ownDeviceID, nil
}

func (s *Service) getOwnDeviceID(apiClient *client.Client) (string, error) {
	relay := s.phoneApprovals
	relay.mutex.Lock()
	ownDeviceID := relay.ownDeviceID
	relay.mutex.Unlock()
	if ownDeviceID != "" {
		return ownDeviceID, nil
	}
	return s.refreshPhoneApprovalDevice(apiClient)
}

func (s *Service) getPhoneApprovalDeviceName() string {
	return strings.TrimSuffix(s.config.DeviceName, ".local")
}

// IsPhoneApprovalAvailable is true while Hisar is unlocked, the switch is on and the account has an iPhone; a stale
// "no iPhone" is checked again before answering, for at most phoneApprovalDeviceCheckTimeout.
func (phoneApprover *PhoneApprover) IsPhoneApprovalAvailable() bool {
	s := phoneApprover.service
	s.mutex.Lock()
	if s.unlockedSession == nil || s.state.IsPhoneApprovalDisabled {
		s.mutex.Unlock()
		return false
	}
	apiClient := s.unlockedSession.Client
	s.mutex.Unlock()
	relay := s.phoneApprovals
	isAvailable, isStale := relay.getDeviceAvailability(s.config.Now())
	switch {
	case isAvailable && isStale:
		s.startPhoneApprovalDeviceRefresh(apiClient)
		return true
	case isAvailable || !isStale:
		return isAvailable
	}
	s.refreshPhoneApprovalDeviceWithin(apiClient, phoneApprovalDeviceCheckTimeout)
	isAvailable, _ = relay.getDeviceAvailability(s.config.Now())
	return isAvailable
}

func (relay *phoneApprovalRelay) getDeviceAvailability(now time.Time) (isAvailable, isStale bool) {
	relay.mutex.Lock()
	defer relay.mutex.Unlock()
	return relay.hasIOSDevice && relay.ownDeviceID != "", now.Sub(relay.deviceRefreshedAt) > phoneApprovalDeviceRefreshInterval
}

// StartPhoneApproval seals the dialog's request to the account and posts it; it stays answerable until the dialog's
// deadline (at most approval.MaximumLifetime), and decide runs at most once, with a verified answer.
func (phoneApprover *PhoneApprover) StartPhoneApproval(description approval.Description, expiresAt time.Time, decide func(isApproved bool)) func() {
	s := phoneApprover.service
	s.mutex.Lock()
	relay := s.phoneApprovals
	relay.mutex.Lock()
	ownDeviceID := relay.ownDeviceID
	relay.mutex.Unlock()
	if s.unlockedSession == nil || s.state.IsPhoneApprovalDisabled || ownDeviceID == "" {
		s.mutex.Unlock()
		return func() {}
	}
	now := s.config.Now()
	lifetime := expiresAt.Sub(now)
	if lifetime <= 0 || lifetime > approval.MaximumLifetime {
		lifetime = approval.MaximumLifetime
	}
	request := approval.CreateRequest(ownDeviceID, s.getPhoneApprovalDeviceName(), description, now, lifetime)
	sealedRequest, err := approval.SealRequest(s.unlockedSession.Keyset, s.unlockedSession.AccountID, request)
	apiClient := s.unlockedSession.Client
	if err != nil {
		s.mutex.Unlock()
		log.Printf("phone approval: %v", err)
		return func() {}
	}
	relay.mutex.Lock()
	relay.tracker.Add(request.RequestID, sealedRequest, request.ExpiresAt)
	relay.allDecideByRequestID[request.RequestID] = decide
	relay.mutex.Unlock()
	s.mutex.Unlock()

	postedChannel := make(chan bool, 1)
	finishedChannel := make(chan struct{})
	go func() {
		isPosted := postPhoneApproval(apiClient, &proto.CreateApprovalRequest{RequestID: request.RequestID, SealedRequest: sealedRequest, ExpiresAt: request.ExpiresAt}, finishedChannel)
		if !isPosted {
			s.forgetPhoneApproval(request.RequestID)
		}
		postedChannel <- isPosted
	}()
	var finishOnce sync.Once
	return func() {
		finishOnce.Do(func() {
			close(finishedChannel)
			s.forgetPhoneApproval(request.RequestID)
			go func() {
				if !<-postedChannel {
					return
				}
				ctx, cancel := context.WithTimeout(context.Background(), phoneApprovalRequestTimeout)
				defer cancel()
				apiClient.ResolveApproval(ctx, request.RequestID)
			}()
		})
	}
}

// postPhoneApproval tries again after a failure, e.g. right after the Mac wakes; a duplicate ID means an earlier try arrived.
func postPhoneApproval(apiClient *client.Client, createRequest *proto.CreateApprovalRequest, finishedChannel chan struct{}) bool {
	for attempt := range phoneApprovalPostAttemptCount {
		if attempt > 0 {
			select {
			case <-finishedChannel:
				return false
			case <-time.After(phoneApprovalPostRetryDelay):
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), phoneApprovalRequestTimeout)
		err := apiClient.CreateApproval(ctx, createRequest)
		cancel()
		if err == nil || client.IsStatus(err, http.StatusConflict) {
			return true
		}
		log.Printf("phone approval: post attempt %d: %v", attempt+1, err)
		if client.IsStatus(err, http.StatusBadRequest) {
			return false
		}
	}
	return false
}

func (s *Service) forgetPhoneApproval(requestID string) {
	relay := s.phoneApprovals
	relay.mutex.Lock()
	defer relay.mutex.Unlock()
	relay.tracker.Remove(requestID)
	delete(relay.allDecideByRequestID, requestID)
}

// handleApprovalEvent hands answers to the waiting dialogs and tells the UI, which lists requests on a phone.
func (s *Service) handleApprovalEvent(apiClient *client.Client, event client.Event) {
	var approvalEvent proto.ApprovalEvent
	if err := json.Unmarshal(event.Data, &approvalEvent); err != nil || !approval.IsValidRequestID(approvalEvent.RequestID) {
		return
	}
	if event.Name == proto.EventApprovalAnswered {
		go s.acceptPhoneApprovalAnswer(apiClient, approvalEvent.RequestID)
	}
	if s.config.OnApprovalEvent != nil {
		go s.config.OnApprovalEvent(event.Name, approvalEvent.RequestID)
	}
}

// acceptPhoneApprovalAnswer fetches the answer and checks it with the account's own keys; an invalid one is ignored.
func (s *Service) acceptPhoneApprovalAnswer(apiClient *client.Client, requestID string) {
	relay := s.phoneApprovals
	relay.mutex.Lock()
	_, isWaiting := relay.allDecideByRequestID[requestID]
	relay.mutex.Unlock()
	if !isWaiting {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), phoneApprovalRequestTimeout)
	defer cancel()
	stored, err := apiClient.GetApproval(ctx, requestID)
	if err != nil || stored.SealedAnswer == nil {
		return
	}
	s.mutex.Lock()
	if s.unlockedSession == nil || s.unlockedSession.Client != apiClient {
		s.mutex.Unlock()
		return
	}
	relay.mutex.Lock()
	decision, err := relay.tracker.Accept(s.unlockedSession.Keyset, s.unlockedSession.AccountID, requestID, stored.SealedAnswer, relay.ownDeviceID, s.config.Now())
	decide := relay.allDecideByRequestID[requestID]
	if err == nil {
		delete(relay.allDecideByRequestID, requestID)
	}
	relay.mutex.Unlock()
	s.mutex.Unlock()
	if err != nil {
		log.Printf("phone approval %s: refused an answer: %v", requestID, err)
		return
	}
	if decide != nil {
		decide(decision == approval.DecisionApprove)
	}
}

func (s *Service) getUnlockedClient() (*client.Client, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.unlockedSession == nil {
		if s.state.IsLoggedIn() {
			return nil, ErrLocked
		}
		return nil, ErrSignedOut
	}
	return s.unlockedSession.Client, nil
}

// ListAllPendingPhoneApproval opens the other devices' open requests; expired, answered and withdrawn ones are left out.
func ListAllPendingPhoneApproval(s *Service) ([]PhoneApprovalRequest, error) {
	apiClient, err := s.getUnlockedClient()
	if err != nil {
		return nil, err
	}
	ownDeviceID, err := s.getOwnDeviceID(apiClient)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), phoneApprovalRequestTimeout)
	defer cancel()
	allPending, err := apiClient.ListAllPendingApproval(ctx)
	if err != nil {
		return nil, convertError(err)
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.unlockedSession == nil || s.unlockedSession.Client != apiClient {
		return nil, ErrLocked
	}
	allRequest := []PhoneApprovalRequest{}
	for _, pending := range allPending {
		request, err := approval.OpenRequest(s.unlockedSession.Keyset, s.unlockedSession.AccountID, pending.RequestID, pending.SealedRequest, ownDeviceID, s.config.Now())
		if err != nil {
			continue
		}
		allRequest = append(allRequest, PhoneApprovalRequest{
			RequestID:           request.RequestID,
			RequesterDeviceName: request.RequesterDeviceName,
			CreatedAt:           request.CreatedAt.UTC().Format(time.RFC3339Nano),
			ExpiresAt:           request.ExpiresAt.UTC().Format(time.RFC3339Nano),
			Description:         request.Description,
		})
	}
	sort.SliceStable(allRequest, func(first, second int) bool { return allRequest[first].CreatedAt < allRequest[second].CreatedAt })
	return allRequest, nil
}

// AnswerPhoneApproval signs this device's decision; the caller checked Face ID or the master password for an approval. Not a method, so the desktop UI cannot call it.
func AnswerPhoneApproval(s *Service, requestID string, isApproved bool) error {
	if !approval.IsValidRequestID(requestID) {
		return errPhoneApprovalGone
	}
	apiClient, err := s.getUnlockedClient()
	if err != nil {
		return err
	}
	ownDeviceID, err := s.getOwnDeviceID(apiClient)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), phoneApprovalRequestTimeout)
	defer cancel()
	stored, err := apiClient.GetApproval(ctx, requestID)
	if client.IsStatus(err, http.StatusNotFound) {
		return errPhoneApprovalGone
	}
	if err != nil {
		return convertError(err)
	}
	if stored.SealedAnswer != nil || stored.IsResolved {
		return errPhoneApprovalGone
	}
	decision := approval.DecisionDeny
	if isApproved {
		decision = approval.DecisionApprove
	}
	s.mutex.Lock()
	if s.unlockedSession == nil || s.unlockedSession.Client != apiClient {
		s.mutex.Unlock()
		return ErrLocked
	}
	s.resetLockTimer()
	now := s.config.Now()
	var sealedAnswer []byte
	request, err := approval.OpenRequest(s.unlockedSession.Keyset, s.unlockedSession.AccountID, requestID, stored.SealedRequest, ownDeviceID, now)
	if err == nil {
		sealedAnswer, err = approval.CreateAnswer(s.unlockedSession.Keyset, s.unlockedSession.AccountID, stored.SealedRequest, request, decision, ownDeviceID, s.config.DeviceName, now)
	}
	s.mutex.Unlock()
	if errors.Is(err, approval.ErrExpired) {
		return errPhoneApprovalGone
	}
	if err != nil {
		return err
	}
	err = apiClient.AnswerApproval(ctx, requestID, sealedAnswer)
	if client.IsStatus(err, http.StatusNotFound) || client.IsStatus(err, http.StatusConflict) || client.IsStatus(err, http.StatusGone) {
		return errPhoneApprovalGone
	}
	return convertError(err)
}
