package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/realtime"
)

const (
	eventHeartbeatInterval = 25 * time.Second
)

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
	s.events.Publish(accountIDs, realtime.Event{Name: apitypes.EventVaultChanged, Data: apitypes.VaultChangedEvent{VaultID: vaultID, Seq: seq}})
}

// publishVaultsChanged tells accounts that their list of vaults changed, e.g. after sharing or removal.
func (s *Server) publishVaultsChanged(accountIDs ...string) {
	s.events.Publish(accountIDs, realtime.Event{Name: apitypes.EventVaultsChanged, Data: struct{}{}})
}

func (s *Server) publishShareInvitationsChanged(accountIDs ...string) {
	s.events.Publish(accountIDs, realtime.Event{Name: apitypes.EventShareInvitationsChanged, Data: struct{}{}})
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

func writeEvent(writer *bufio.Writer, event realtime.Event) error {
	data, err := json.Marshal(event.Data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", event.Name, data); err != nil {
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
	subscriber, isSubscribed := s.events.Subscribe(currentSession.AccountID, currentSession.TokenHash)
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
		defer s.events.Unsubscribe(accountID, subscriber)
		extendWriteDeadline()
		if writeComment(writer, "connected") != nil {
			return
		}
		ticker := time.NewTicker(s.config.EventHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case event := <-subscriber.Events:
				if s.events.IsEnded(subscriber) {
					return
				}
				extendWriteDeadline()
				if writeEvent(writer, event) != nil {
					return
				}
			case <-ticker.C:
				if s.events.IsOverflowed(subscriber) || s.events.IsEnded(subscriber) || !s.isSessionValid(subscriber.TokenHash) {
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
