package vault

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/proto"
)

const (
	eventRetryMinimum = time.Second
	eventRetryMaximum = time.Minute
	// eventStreamStableAfter resets the retry backoff once a stream has stayed open this long.
	eventStreamStableAfter = time.Minute
)

// watchEvents keeps the server's event stream open while the session is unlocked and syncs on every change.
func (s *Service) watchEvents(ctx context.Context, apiClient *client.Client, reconnectChannel chan struct{}) {
	allPendingSync := make(chan struct{}, 1)
	go s.runAllPendingSync(ctx, allPendingSync)
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
		err := apiClient.StreamEvents(streamContext, func(event client.Event) {
			switch event.Name {
			case proto.EventVaultChanged, proto.EventVaultsChanged, proto.EventShareInvitationsChanged:
				select {
				case allPendingSync <- struct{}{}:
				default:
				}
			case proto.EventApprovalRequested, proto.EventApprovalAnswered, proto.EventApprovalResolved:
				s.handleApprovalEvent(apiClient, event)
			case proto.EventShareLinksChanged:
				if s.config.OnShareLinksChange != nil {
					s.config.OnShareLinksChange()
				}
			}
		})
		close(streamFinished)
		cancelStream()
		if ctx.Err() != nil || client.IsStatus(err, http.StatusUnauthorized) {
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

// runAllPendingSync coalesces bursts of events into one sync at a time.
func (s *Service) runAllPendingSync(ctx context.Context, allPendingSync chan struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-allPendingSync:
			if s.syncAllVault(ctx) && s.config.OnRemoteChange != nil {
				s.config.OnRemoteChange()
			}
		}
	}
}

func (s *Service) syncAllVault(ctx context.Context) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.unlockedSession == nil || ctx.Err() != nil {
		return false
	}
	s.completeAllAcceptedShare(ctx)
	if err := s.loadAllVault(ctx); err != nil {
		return false
	}
	allPreviousItem := s.allItemByVaultID
	s.allItemByVaultID = map[string][]item.Item{}
	if err := s.loadAllItemOfAllVault(ctx); err != nil {
		s.allItemByVaultID = allPreviousItem
		return false
	}
	return true
}
