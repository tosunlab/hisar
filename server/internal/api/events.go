package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/apitypes"
)

const (
	eventHeartbeatInterval     = 25 * time.Second
	maxEventStreamPerAccount   = 8
	eventSubscriberBufferCount = 32
)

type serverEvent struct {
	name string
	data any
}

type eventSubscriber struct {
	tokenHash []byte
	channel   chan serverEvent
	// isOverflowed is set when an event could not be queued; the stream then ends so the client resyncs.
	isOverflowed bool
	isEnded      bool
}

// eventBroker fans vault changes out to the event streams of one server process.
type eventBroker struct {
	mutex                sync.Mutex
	subscribersByAccount map[string]map[*eventSubscriber]struct{}
}

func createEventBroker() *eventBroker {
	return &eventBroker{subscribersByAccount: map[string]map[*eventSubscriber]struct{}{}}
}

func (broker *eventBroker) subscribe(accountID string, tokenHash []byte) (*eventSubscriber, bool) {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	subscribers := broker.subscribersByAccount[accountID]
	if len(subscribers) >= maxEventStreamPerAccount {
		return nil, false
	}
	if subscribers == nil {
		subscribers = map[*eventSubscriber]struct{}{}
		broker.subscribersByAccount[accountID] = subscribers
	}
	subscriber := &eventSubscriber{tokenHash: tokenHash, channel: make(chan serverEvent, eventSubscriberBufferCount)}
	subscribers[subscriber] = struct{}{}
	return subscriber, true
}

func (broker *eventBroker) unsubscribe(accountID string, subscriber *eventSubscriber) {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	delete(broker.subscribersByAccount[accountID], subscriber)
	if len(broker.subscribersByAccount[accountID]) == 0 {
		delete(broker.subscribersByAccount, accountID)
	}
}

func (broker *eventBroker) publish(accountIDs []string, event serverEvent) {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	for _, accountID := range accountIDs {
		for subscriber := range broker.subscribersByAccount[accountID] {
			select {
			case subscriber.channel <- event:
			default:
				subscriber.isOverflowed = true
			}
		}
	}
}

// endStreams closes the account's streams except the one of keptTokenHash, without waiting for the next heartbeat.
func (broker *eventBroker) endStreams(accountID string, keptTokenHash []byte) {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	for subscriber := range broker.subscribersByAccount[accountID] {
		if keptTokenHash != nil && bytes.Equal(subscriber.tokenHash, keptTokenHash) {
			continue
		}
		subscriber.isEnded = true
		select {
		case subscriber.channel <- serverEvent{}:
		default:
		}
	}
}

func (broker *eventBroker) isEnded(subscriber *eventSubscriber) bool {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	return subscriber.isEnded
}

func (broker *eventBroker) isOverflowed(subscriber *eventSubscriber) bool {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	return subscriber.isOverflowed
}

func (broker *eventBroker) countSubscriber(accountID string) int {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	return len(broker.subscribersByAccount[accountID])
}

// publishVaultChanged tells every member of a vault that it reached seq.
func (s *Server) publishVaultChanged(ctx context.Context, vaultID string, seq int64) {
	members, err := s.store.ListMembers(ctx, vaultID)
	if err != nil {
		log.Printf("events: members of %s: %v", vaultID, err)
		return
	}
	accountIDs := make([]string, 0, len(members))
	for _, member := range members {
		accountIDs = append(accountIDs, member.AccountID)
	}
	s.events.publish(accountIDs, serverEvent{name: apitypes.EventVaultChanged, data: apitypes.VaultChangedEvent{VaultID: vaultID, Seq: seq}})
}

// publishVaultsChanged tells accounts that their list of vaults changed, e.g. after sharing or removal.
func (s *Server) publishVaultsChanged(accountIDs ...string) {
	s.events.publish(accountIDs, serverEvent{name: apitypes.EventVaultsChanged, data: struct{}{}})
}

func (s *Server) publishShareInvitationsChanged(accountIDs ...string) {
	s.events.publish(accountIDs, serverEvent{name: apitypes.EventShareInvitationsChanged, data: struct{}{}})
}

// publishShareInvitationsChangedToOwner tells the vault's owners, who complete accepted shares, and otherAccountIDs.
func (s *Server) publishShareInvitationsChangedToOwner(ctx context.Context, vaultID string, otherAccountIDs ...string) {
	members, err := s.store.ListMembers(ctx, vaultID)
	if err != nil {
		log.Printf("events: members of %s: %v", vaultID, err)
	}
	accountIDs := otherAccountIDs
	for _, member := range members {
		if member.Role == apitypes.VaultOwner {
			accountIDs = append(accountIDs, member.AccountID)
		}
	}
	s.publishShareInvitationsChanged(accountIDs...)
}

func writeEvent(writer *bufio.Writer, event serverEvent) error {
	data, err := json.Marshal(event.data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", event.name, data); err != nil {
		return err
	}
	return writer.Flush()
}

func writeComment(writer *bufio.Writer, comment string) error {
	if _, err := fmt.Fprintf(writer, ": %s\n\n", comment); err != nil {
		return err
	}
	return writer.Flush()
}

// handleStreamEvents keeps a server-sent event stream open; it ends when the session is revoked or expires.
func (s *Server) handleStreamEvents(c fiber.Ctx) error {
	currentSession := getSession(c)
	subscriber, isSubscribed := s.events.subscribe(currentSession.AccountID, currentSession.TokenHash)
	if !isSubscribed {
		return createAPIError(fiber.StatusTooManyRequests, "too many event streams")
	}
	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set("X-Accel-Buffering", "no")
	accountID := currentSession.AccountID
	connection := c.RequestCtx().Conn()
	// The server's WriteTimeout covers the whole response; each write moves the deadline so the stream can stay open.
	extendWriteDeadline := func() {
		connection.SetWriteDeadline(time.Now().Add(s.config.EventHeartbeatInterval + 30*time.Second))
	}
	return c.SendStreamWriter(func(writer *bufio.Writer) {
		defer s.events.unsubscribe(accountID, subscriber)
		extendWriteDeadline()
		if writeComment(writer, "connected") != nil {
			return
		}
		ticker := time.NewTicker(s.config.EventHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case event := <-subscriber.channel:
				if s.events.isEnded(subscriber) {
					return
				}
				extendWriteDeadline()
				if writeEvent(writer, event) != nil {
					return
				}
			case <-ticker.C:
				if s.events.isOverflowed(subscriber) || s.events.isEnded(subscriber) || !s.isSessionValid(subscriber.tokenHash) {
					return
				}
				extendWriteDeadline()
				if writeComment(writer, "heartbeat") != nil {
					return
				}
			}
		}
	})
}

func (s *Server) isSessionValid(tokenHash []byte) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := s.store.GetSessionByHash(ctx, tokenHash, s.config.SessionTTL)
	return err == nil
}
