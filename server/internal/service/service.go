// Package service is the server's business layer: what each API request may do and what it changes. It checks the
// caller's rights and the request, calls the store, records the audit trail, publishes events and sends email;
// internal/api only speaks HTTP.
package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytemare/opaque"
	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/server/internal/mail"
	"github.com/tryhisar/hisar/server/internal/ratelimit"
	"github.com/tryhisar/hisar/server/internal/realtime"
	"github.com/tryhisar/hisar/server/internal/settings"
	"github.com/tryhisar/hisar/server/internal/store"
)

// Actor is who makes a request: a signed-in account and device, and the IP address it came from.
type Actor struct {
	AccountID string
	DeviceID  string
	Role      string
	IP        string
	// TokenHash is the hash of the session token; credential changes keep this session signed in.
	TokenHash []byte
}

// Config is what the service reads on every request and never changes while it serves.
type Config struct {
	// PublicURL is the operator-approved authentication origin and the base URL of email links.
	PublicURL            string
	PendingLoginCapacity int
	LimiterKeyCapacity   int
	LimiterTotalCapacity int
}

// InstanceSettings is where the service reads the settings an admin edits: Current is the snapshot a save replaces
// whole; Defaults and Environment let a save resolve the next one, and OnSave applies it outside the service.
type InstanceSettings struct {
	Current     *atomic.Pointer[settings.Settings]
	Defaults    settings.Settings
	Environment settings.Environment
	OnSave      func(settings.Settings)
}

// Limiters are the per-account and per-IP limits of what accounts may do; tests replace them.
type Limiters struct {
	InviteAccount          *ratelimit.Limiter
	InviteIP               *ratelimit.Limiter
	ShareInvitationAccount *ratelimit.Limiter
	ShareInvitationIP      *ratelimit.Limiter
	ShareAcceptedEmail     *ratelimit.Limiter
	ShareLinkAccount       *ratelimit.Limiter
	Reauthentication       *ratelimit.Limiter
	TestEmail              *ratelimit.Limiter
}

const (
	invitePerAccountPerDay             = 10
	invitePerIPPerDay                  = 20
	shareInvitationPerAccountPerDay    = 50
	shareInvitationPerIPPerDay         = 100
	shareAcceptedEmailPerInviterPerDay = 50
	shareLinkPerAccountPerDay          = 100
	testEmailPerAccountBurst           = 5
	defaultInviteTTL                   = 7 * 24 * time.Hour
	defaultShareAcceptedEmailDelay     = 2 * time.Minute
)

func newLimiters(factory *ratelimit.Factory) Limiters {
	return Limiters{
		InviteAccount:          factory.Daily(invitePerAccountPerDay),
		InviteIP:               factory.Daily(invitePerIPPerDay),
		ShareInvitationAccount: factory.Daily(shareInvitationPerAccountPerDay),
		ShareInvitationIP:      factory.Daily(shareInvitationPerIPPerDay),
		ShareAcceptedEmail:     factory.Daily(shareAcceptedEmailPerInviterPerDay),
		ShareLinkAccount:       factory.Daily(shareLinkPerAccountPerDay),
		Reauthentication:       factory.New(rate.Every(30*time.Second), 5),
		// An admin's test emails: a few at once, then five a minute, since each one connects to a host the admin chose.
		TestEmail: factory.New(rate.Every(time.Minute/testEmailPerAccountBurst), testEmailPerAccountBurst),
	}
}

// NewReauthenticationLimiter allows a few master password checks per account, then one every 30 seconds.
func NewReauthenticationLimiter() *ratelimit.Limiter {
	return ratelimit.New(rate.Every(30*time.Second), 5)
}

// Keys are the server's long-term secrets: the OPAQUE server key material and the key of the MACs behind fake salts.
type Keys struct {
	OpaqueConfiguration    *opaque.Configuration
	OpaqueServer           *opaque.Server
	HMACKey                []byte
	LoginServer            *opaque.Server
	ReauthenticationServer *opaque.Server
}

type Service struct {
	config           Config
	instanceSettings InstanceSettings
	// settingsMutex orders saves, so the snapshot always follows the last stored document.
	settingsMutex sync.Mutex
	store         *store.Store
	events        *realtime.Broker
	keys          Keys
	loginCache    *loginCache
	// Limiters, InviteTTL, ShareAcceptedEmailDelay and NewMailer may be replaced before the server serves.
	Limiters  Limiters
	InviteTTL time.Duration
	// ShareAcceptedEmailDelay is how long an accepted share may wait for the owner's app before the inviter gets an email.
	ShareAcceptedEmailDelay time.Duration
	// NewMailer sends with the given SMTP settings; it returns nil when they turn email off.
	NewMailer func(settings.SMTP) mail.Mailer
	// Approvals relays phone approvals between the devices of an account.
	Approvals *ApprovalRelay
	// PushClient sends to the push relay.
	PushClient     *http.Client
	backgroundWork sync.WaitGroup
}

func New(database *store.Store, events *realtime.Broker, keys Keys, config Config, instanceSettings InstanceSettings, factory *ratelimit.Factory) *Service {
	return &Service{
		config:                  config,
		instanceSettings:        instanceSettings,
		store:                   database,
		events:                  events,
		keys:                    keys,
		loginCache:              newLoginCache(time.Minute, config.PendingLoginCapacity),
		Limiters:                newLimiters(factory),
		InviteTTL:               defaultInviteTTL,
		ShareAcceptedEmailDelay: defaultShareAcceptedEmailDelay,
		NewMailer:               newSMTPMailer,
		Approvals:               newApprovalRelay(factory.New(rate.Every(approvalRateEvery), ApprovalRateBurst)),
		PushClient:              &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *Service) currentSettings() *settings.Settings { return s.instanceSettings.Current.Load() }

func (s *Service) computeMAC(parts ...string) []byte {
	hmacHash := hmac.New(sha256.New, s.keys.HMACKey)
	for _, part := range parts {
		hmacHash.Write([]byte(part))
		hmacHash.Write([]byte{0})
	}
	return hmacHash.Sum(nil)
}

// MaintainAuthenticationState expires pending exchanges and fully repaid idle limiter entries.
func (s *Service) MaintainAuthenticationState() {
	s.loginCache.maintain()
	for _, limiter := range []*ratelimit.Limiter{
		s.Limiters.InviteAccount, s.Limiters.InviteIP, s.Limiters.ShareInvitationAccount,
		s.Limiters.ShareInvitationIP, s.Limiters.ShareAcceptedEmail, s.Limiters.ShareLinkAccount,
		s.Limiters.Reauthentication, s.Limiters.TestEmail,
	} {
		limiter.Maintain()
	}
	s.Approvals.mutex.Lock()
	s.Approvals.limiter.Maintain()
	s.Approvals.mutex.Unlock()
}

// RunAuthenticationMaintenance retires bounded batches until shutdown, even without request traffic.
func (s *Service) RunAuthenticationMaintenance(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.MaintainAuthenticationState()
		}
	}
}
