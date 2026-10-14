package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/server/internal/realtime"
)

const (
	eventHeartbeatInterval = 25 * time.Second
)

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
	return s.service.IsSessionValid(ctx, tokenHash)
}
