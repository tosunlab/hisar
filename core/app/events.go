package app

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
)

const (
	eventRetryMinimum = time.Second
	eventRetryMaximum = time.Minute
	// eventStreamStableAfter resets the retry backoff once a stream has stayed open this long.
	eventStreamStableAfter = time.Minute
)

// watchEvents keeps the server's event stream open while the session is unlocked and syncs on every change.
func (s *Service) watchEvents(ctx context.Context, apiClient *apiclient.Client, reconnectChannel chan struct{}) {
	pendingSyncs := make(chan struct{}, 1)
	go s.runPendingSyncs(ctx, pendingSyncs)
	retryDelay := eventRetryMinimum
	for ctx.Err() == nil {
		connectedAt := time.Now()
		streamContext, cancelStream := context.WithCancel(ctx)
		var isReconnectRequested atomic.Bool
		streamFinished := make(chan struct{})
		go func() {
			select {
			case <-reconnectChannel:
				isReconnectRequested.Store(true)
				cancelStream()
			case <-streamFinished:
			}
		}()
		err := apiClient.StreamEvents(streamContext, func(event apiclient.Event) {
			switch event.Name {
			case apitypes.EventVaultChanged, apitypes.EventVaultsChanged, apitypes.EventShareInvitationsChanged, apitypes.EventAccountPreferencesChanged,
				apitypes.EventOrganizationsChanged:
				select {
				case pendingSyncs <- struct{}{}:
				default:
				}
			case apitypes.EventApprovalRequested, apitypes.EventApprovalAnswered, apitypes.EventApprovalResolved:
				s.handleApprovalEvent(apiClient, event)
			case apitypes.EventShareLinksChanged:
				if s.config.OnShareLinksChange != nil {
					s.config.OnShareLinksChange()
				}
			}
		})
		close(streamFinished)
		cancelStream()
		if _, isUpgradeRequired := apiclient.AsUpgradeRequired(err); isUpgradeRequired || ctx.Err() != nil {
			return
		}
		if apiclient.IsStatus(err, http.StatusUnauthorized) {
			go s.endSignedOutSession(apiClient.Token)
			return
		}
		if errors.Is(err, apiclient.ErrAccountSuspended) {
			go s.lockSuspendedSession(apiClient.Token)
			return
		}
		if isReconnectRequested.Load() {
			retryDelay = eventRetryMinimum
			continue
		}
		if time.Since(connectedAt) > eventStreamStableAfter {
			retryDelay = eventRetryMinimum
		}
		select {
		case <-ctx.Done():
			return
		case <-reconnectChannel:
			retryDelay = eventRetryMinimum
			continue
		case <-time.After(retryDelay):
		}
		retryDelay = min(retryDelay*2, eventRetryMaximum)
	}
}

// ReconnectEventStream opens a new event stream now, e.g. when the iOS app returns to the foreground:
// a stream from before the app was suspended may be dead without the client knowing. Not a method, so the UI cannot bind it.
func ReconnectEventStream(s *Service) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.reconnectEvents == nil {
		return
	}
	select {
	case s.reconnectEvents <- struct{}{}:
	default:
	}
}

// runPendingSyncs coalesces bursts of events into one sync at a time.
func (s *Service) runPendingSyncs(ctx context.Context, pendingSyncs chan struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-pendingSyncs:
			if s.syncVaults(ctx) && s.config.OnRemoteChange != nil {
				s.config.OnRemoteChange()
			}
		}
	}
}

func (s *Service) syncVaults(ctx context.Context) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.unlockedSession == nil || ctx.Err() != nil {
		return false
	}
	s.completeAcceptedShares(ctx)
	s.completeOrganizationKeys(ctx)
	if !s.reloadAllVaultItem(ctx) {
		return false
	}
	s.loadFavorites(ctx)
	return true
}
