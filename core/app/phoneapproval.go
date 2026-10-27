package app

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/approval"
)

const (
	phoneApprovalDeviceRefreshInterval = 5 * time.Minute
	phoneApprovalRequestTimeout        = 15 * time.Second
	phoneApprovalDeviceCheckTimeout    = 3 * time.Second
	phoneApprovalPostAttemptCount      = 3
	phoneApprovalPostRetryDelay        = 2 * time.Second
)

var (
	errPhoneApprovalGone        = errors.New("this request is no longer waiting for an answer")
	errPhoneApprovalUnavailable = errors.New("phone approval is not available")
)

// phoneApprovalRelay is the backend's part of phone approvals: the requests this device posted and what it knows about its devices.
type phoneApprovalRelay struct {
	mutex              sync.Mutex
	isRequester        bool
	ownDeviceID        string
	hasPhoneDevice     bool
	deviceRefreshedAt  time.Time
	isRefreshing       bool
	tracker            *approval.Tracker
	decidesByRequestID map[string]func(answer approval.AcceptedAnswer)
}

func createPhoneApprovalRelay() *phoneApprovalRelay {
	return &phoneApprovalRelay{tracker: approval.NewTracker(), decidesByRequestID: map[string]func(answer approval.AcceptedAnswer){}}
}

// PhoneApprover lets the approval window send its requests to the account's phones; it is not bound to the UI.
type PhoneApprover struct {
	service *Service
}

func NewPhoneApprover(service *Service) *PhoneApprover {
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
	requestIDs := make([]string, 0, len(relay.decidesByRequestID))
	for requestID := range relay.decidesByRequestID {
		requestIDs = append(requestIDs, requestID)
	}
	clear(relay.decidesByRequestID)
	relay.tracker.RemoveAll()
	relay.ownDeviceID, relay.hasPhoneDevice, relay.deviceRefreshedAt = "", false, time.Time{}
	relay.mutex.Unlock()
	if len(requestIDs) == 0 || s.unlockedSession == nil {
		return
	}
	apiClient := s.unlockedSession.Client
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), phoneApprovalRequestTimeout)
		defer cancel()
		for _, requestID := range requestIDs {
			apiClient.ResolveApproval(ctx, requestID)
		}
	}()
}

func (s *Service) startPhoneApprovalDeviceRefresh(apiClient *apiclient.Client) {
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

// refreshPhoneApprovalDevice learns this device's ID and whether the account has a phone.
func (s *Service) refreshPhoneApprovalDevice(apiClient *apiclient.Client) (string, error) {
	return s.refreshPhoneApprovalDeviceWithin(apiClient, phoneApprovalRequestTimeout)
}

func (s *Service) refreshPhoneApprovalDeviceWithin(apiClient *apiclient.Client, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	devices, err := apiClient.ListDevices(ctx)
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
	relay.ownDeviceID, relay.hasPhoneDevice = "", false
	for _, device := range devices {
		switch {
		case device.Current:
			relay.ownDeviceID = device.ID
		case apitypes.IsPhonePlatform(device.Platform):
			relay.hasPhoneDevice = true
		}
	}
	relay.deviceRefreshedAt = s.config.Now()
	if relay.ownDeviceID == "" {
		return "", errors.New("this device is missing from the account's devices")
	}
	return relay.ownDeviceID, nil
}

func (s *Service) getOwnDeviceID(apiClient *apiclient.Client) (string, error) {
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

// IsPhoneApprovalAvailable is true while Hisar is unlocked, the switch is on and the account has a phone; a stale
// "no phone" is checked again before answering, for at most phoneApprovalDeviceCheckTimeout.
func (phoneApprover *PhoneApprover) IsPhoneApprovalAvailable() bool {
	s := phoneApprover.service
	s.mutex.Lock()
	if !s.isUnlocked() || s.state.IsPhoneApprovalDisabled {
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
	return relay.hasPhoneDevice && relay.ownDeviceID != "", now.Sub(relay.deviceRefreshedAt) > phoneApprovalDeviceRefreshInterval
}

// StartPhoneApproval seals the dialog's request to the account and posts it; it stays answerable until the dialog's
// deadline (at most approval.MaximumLifetime), and decide runs at most once, with a verified answer. It sends nothing
// and fails when phone approval is not available, and with approval.ErrSealedTooLarge when the request does not fit.
func (phoneApprover *PhoneApprover) StartPhoneApproval(description approval.Description, expiresAt time.Time, posted func(isSent bool), decide func(answer approval.AcceptedAnswer)) (func(), error) {
	s := phoneApprover.service
	s.mutex.Lock()
	relay := s.phoneApprovals
	relay.mutex.Lock()
	ownDeviceID := relay.ownDeviceID
	relay.mutex.Unlock()
	if !s.isUnlocked() || s.state.IsPhoneApprovalDisabled || ownDeviceID == "" {
		s.mutex.Unlock()
		return nil, errPhoneApprovalUnavailable
	}
	now := s.config.Now()
	lifetime := expiresAt.Sub(now)
	if lifetime <= 0 || lifetime > approval.MaximumLifetime {
		lifetime = approval.MaximumLifetime
	}
	request := approval.NewRequest(ownDeviceID, s.getPhoneApprovalDeviceName(), description, now, lifetime)
	sealedRequest, err := approval.SealRequest(s.unlockedSession.Keyset, s.unlockedSession.AccountID, request)
	apiClient := s.unlockedSession.Client
	if err != nil {
		s.mutex.Unlock()
		return nil, err
	}
	relay.mutex.Lock()
	relay.tracker.Add(request.RequestID, sealedRequest, request.ExpiresAt)
	relay.decidesByRequestID[request.RequestID] = decide
	relay.mutex.Unlock()
	s.mutex.Unlock()

	postedChannel := make(chan bool, 1)
	finishedChannel := make(chan struct{})
	go func() {
		isPosted := postPhoneApproval(apiClient, &apitypes.CreateApprovalRequest{RequestID: request.RequestID, SealedRequest: sealedRequest, ExpiresAt: request.ExpiresAt}, finishedChannel)
		if !isPosted {
			s.forgetPhoneApproval(request.RequestID)
		}
		postedChannel <- isPosted
		if posted != nil {
			posted(isPosted)
		}
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
	}, nil
}

// postPhoneApproval tries again after a failure, e.g. right after the Mac wakes; a duplicate ID means an earlier try arrived.
func postPhoneApproval(apiClient *apiclient.Client, createRequest *apitypes.CreateApprovalRequest, finishedChannel chan struct{}) bool {
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
		if err == nil || apiclient.IsStatus(err, http.StatusConflict) {
			return true
		}
		log.Printf("phone approval: post attempt %d: %v", attempt+1, err)
		if apiclient.IsStatus(err, http.StatusBadRequest) {
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
	delete(relay.decidesByRequestID, requestID)
}

// handleApprovalEvent hands answers to the waiting dialogs and tells the UI, which lists requests on a phone.
func (s *Service) handleApprovalEvent(apiClient *apiclient.Client, event apiclient.Event) {
	var approvalEvent apitypes.ApprovalEvent
	if err := json.Unmarshal(event.Data, &approvalEvent); err != nil || !approval.IsValidRequestID(approvalEvent.RequestID) {
		return
	}
	if event.Name == apitypes.EventApprovalAnswered {
		go s.acceptPhoneApprovalAnswer(apiClient, approvalEvent.RequestID)
	}
	if s.config.OnApprovalEvent != nil {
		go s.config.OnApprovalEvent(event.Name, approvalEvent.RequestID)
	}
}

// acceptPhoneApprovalAnswer fetches the answer and checks it with the account's own keys; an invalid one is ignored.
func (s *Service) acceptPhoneApprovalAnswer(apiClient *apiclient.Client, requestID string) {
	relay := s.phoneApprovals
	relay.mutex.Lock()
	_, isWaiting := relay.decidesByRequestID[requestID]
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
	if !s.isUnlocked() || s.unlockedSession.Client != apiClient {
		s.mutex.Unlock()
		return
	}
	relay.mutex.Lock()
	accepted, err := relay.tracker.AcceptAnswer(s.unlockedSession.Keyset, s.unlockedSession.AccountID, requestID, stored.SealedAnswer, relay.ownDeviceID, s.config.Now())
	decide := relay.decidesByRequestID[requestID]
	if err == nil {
		delete(relay.decidesByRequestID, requestID)
	}
	relay.mutex.Unlock()
	s.mutex.Unlock()
	if err != nil {
		log.Printf("phone approval %s: refused an answer: %v", requestID, err)
		return
	}
	if decide != nil {
		decide(accepted)
	}
}

func (s *Service) getUnlockedClient() (*apiclient.Client, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlockedWithoutActivity(); err != nil {
		return nil, err
	}
	return s.unlockedSession.Client, nil
}
