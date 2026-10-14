// Package service is the server's business layer: what each API request may do and what it changes. It checks the
// caller's rights and the request, calls the store, records the audit trail, publishes events and sends email;
// internal/api only speaks HTTP.
package service

import (
	"context"

	"github.com/tryhisar/hisar/server/internal/realtime"
	"github.com/tryhisar/hisar/server/internal/store"
)

// Actor is who makes a request: a signed-in account and device, and the IP address it came from.
type Actor struct {
	AccountID string
	DeviceID  string
	IP        string
}

type Service struct {
	store  *store.Store
	events *realtime.Broker
}

func New(database *store.Store, events *realtime.Broker) *Service {
	return &Service{store: database, events: events}
}

// recordAudit keeps the trail of who changed what; a failure to record does not fail the request.
func (s *Service) recordAudit(ctx context.Context, actor Actor, eventType string, detail any) {
	var accountID, deviceID *string
	if actor.AccountID != "" {
		accountID = &actor.AccountID
	}
	if actor.DeviceID != "" {
		deviceID = &actor.DeviceID
	}
	s.store.RecordAudit(ctx, accountID, deviceID, eventType, detail, actor.IP)
}
