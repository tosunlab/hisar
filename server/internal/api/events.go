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

	"github.com/tryhisar/hisar/core/proto"
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
	mutex                  sync.Mutex
	allSubscriberByAccount map[string]map[*eventSubscriber]struct{}
}

func createEventBroker() *eventBroker {
	return &eventBroker{allSubscriberByAccount: map[string]map[*eventSubscriber]struct{}{}}
}

func (broker *eventBroker) subscribe(accountID string, tokenHash []byte) (*eventSubscriber, bool) {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	allSubscriber := broker.allSubscriberByAccount[accountID]
	if len(allSubscriber) >= maxEventStreamPerAccount {
		return nil, false
	}
	if allSubscriber == nil {
		allSubscriber = map[*eventSubscriber]struct{}{}
		broker.allSubscriberByAccount[accountID] = allSubscriber
	}
	subscriber := &eventSubscriber{tokenHash: tokenHash, channel: make(chan serverEvent, eventSubscriberBufferCount)}
	allSubscriber[subscriber] = struct{}{}
	return subscriber, true
}

func (broker *eventBroker) unsubscribe(accountID string, subscriber *eventSubscriber) {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	delete(broker.allSubscriberByAccount[accountID], subscriber)
	if len(broker.allSubscriberByAccount[accountID]) == 0 {
		delete(broker.allSubscriberByAccount, accountID)
	}
}

func (broker *eventBroker) publish(allAccountID []string, event serverEvent) {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	for _, accountID := range allAccountID {
		for subscriber := range broker.allSubscriberByAccount[accountID] {
			select {
			case subscriber.channel <- event:
			default:
				subscriber.isOverflowed = true
			}
		}
	}
}

// endAllStream closes the account's streams except the one of keptTokenHash, without waiting for the next heartbeat.
func (broker *eventBroker) endAllStream(accountID string, keptTokenHash []byte) {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	for subscriber := range broker.allSubscriberByAccount[accountID] {
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
	return len(broker.allSubscriberByAccount[accountID])
}

// publishVaultChanged tells every member of a vault that it reached seq.
func (s *Server) publishVaultChanged(ctx context.Context, vaultID string, seq int64) {
	allMember, err := s.store.ListAllMember(ctx, vaultID)
	if err != nil {
		log.Printf("events: members of %s: %v", vaultID, err)
		return
	}
	allAccountID := make([]string, 0, len(allMember))
	for _, member := range allMember {
		allAccountID = append(allAccountID, member.AccountID)
	}
	s.events.publish(allAccountID, serverEvent{name: proto.EventVaultChanged, data: proto.VaultChangedEvent{VaultID: vaultID, Seq: seq}})
}

// publishVaultsChanged tells accounts that their list of vaults changed, e.g. after sharing or removal.
func (s *Server) publishVaultsChanged(allAccountID ...string) {
	s.events.publish(allAccountID, serverEvent{name: proto.EventVaultsChanged, data: struct{}{}})
}

func (s *Server) publishShareInvitationsChanged(allAccountID ...string) {
	s.events.publish(allAccountID, serverEvent{name: proto.EventShareInvitationsChanged, data: struct{}{}})
}

// publishShareInvitationsChangedToOwner tells the vault's owners, who complete accepted shares, and allOtherAccountID.
func (s *Server) publishShareInvitationsChangedToOwner(ctx context.Context, vaultID string, allOtherAccountID ...string) {
	allMember, err := s.store.ListAllMember(ctx, vaultID)
	if err != nil {
		log.Printf("events: members of %s: %v", vaultID, err)
	}
	allAccountID := allOtherAccountID
	for _, member := range allMember {
		if member.Role == proto.VaultOwner {
			allAccountID = append(allAccountID, member.AccountID)
		}
	}
	s.publishShareInvitationsChanged(allAccountID...)
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
