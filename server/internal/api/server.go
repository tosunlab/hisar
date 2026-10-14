package api

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/bytemare/opaque"
	"github.com/gofiber/fiber/v3"
	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/pake"
	"github.com/tryhisar/hisar/server/internal/desktopupdate"
	"github.com/tryhisar/hisar/server/internal/mail"
	"github.com/tryhisar/hisar/server/internal/store"
)

// The transport accepts rotation-sized bodies; decodeBody enforces maxBody everywhere else.
const (
	maxBody         = 2 << 20
	maxRotationBody = 32 << 20
)

type Config struct {
	PublicURL  string
	SessionTTL time.Duration
	InviteTTL  time.Duration
	TrustProxy bool
	// AuthBurst is how many auth requests one IP may make before being limited to one per 6 seconds.
	AuthBurst int
	// EventHeartbeatInterval keeps event streams alive through proxies and rechecks their session.
	EventHeartbeatInterval time.Duration
	// WriteTimeout limits writing one response; event streams extend it with every write.
	WriteTimeout time.Duration
	// DesktopUpdates is nil when the server has no update source.
	DesktopUpdates *desktopupdate.Source
	// Mailer is nil when email is not configured.
	Mailer mail.Mailer
	// ShareAcceptedEmailDelay is how long an accepted share may wait for the owner's app before the inviter gets an email.
	ShareAcceptedEmailDelay time.Duration
	// MinimumVersions refuses older apps with 426; independent of the latest version offered for updates.
	MinimumVersions MinimumVersions
}

type Server struct {
	config              Config
	store               *store.Store
	secrets             *Secrets
	opaqueConfiguration *opaque.Configuration
	opaque              *opaque.Server
	loginCache          *loginCache
	limiter             *ipLimiter
	updateLimiter       *ipLimiter
	events              *eventBroker
	approvals           *approvalRelay

	inviteAccountLimiter          *dailyLimiter
	inviteIPLimiter               *dailyLimiter
	shareInvitationAccountLimiter *dailyLimiter
	shareInvitationIPLimiter      *dailyLimiter
	shareAcceptedEmailLimiter     *dailyLimiter
	shareLinkAccountLimiter       *dailyLimiter
	shareLinkReadLimiter          *ipLimiter
	reauthenticationLimiter       *ipLimiter
	invitePageLimiter             *ipLimiter
	versionCheckLimiter           *ipLimiter
	backgroundWork                sync.WaitGroup
}

func CreateServer(database *store.Store, secrets *Secrets, config Config) (*Server, error) {
	if config.SessionTTL == 0 {
		config.SessionTTL = 30 * 24 * time.Hour
	}
	if config.InviteTTL == 0 {
		config.InviteTTL = 7 * 24 * time.Hour
	}
	if config.AuthBurst == 0 {
		config.AuthBurst = 10
	}
	if config.EventHeartbeatInterval == 0 {
		config.EventHeartbeatInterval = eventHeartbeatInterval
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = 30 * time.Second
	}
	if config.ShareAcceptedEmailDelay == 0 {
		config.ShareAcceptedEmailDelay = defaultShareAcceptedEmailDelay
	}
	opaqueConfiguration := pake.CreateConfig()
	serverKeyMaterial, err := opaqueConfiguration.DecodeServerKeyMaterial(secrets.OpaqueKeyMaterial)
	if err != nil {
		return nil, err
	}
	opaqueServer, err := opaqueConfiguration.Server()
	if err != nil {
		return nil, err
	}
	if err := opaqueServer.SetKeyMaterial(serverKeyMaterial); err != nil {
		return nil, err
	}
	return &Server{
		config:              config,
		store:               database,
		secrets:             secrets,
		opaqueConfiguration: opaqueConfiguration,
		opaque:              opaqueServer,
		loginCache:          createLoginCache(time.Minute),
		limiter:             createIPLimiter(rate.Every(6*time.Second), config.AuthBurst),
		updateLimiter:       createIPLimiter(rate.Every(10*time.Second), 20),
		events:              createEventBroker(),
		approvals:           createApprovalRelay(),

		inviteAccountLimiter:          createDailyLimiter(invitePerAccountPerDay),
		inviteIPLimiter:               createDailyLimiter(invitePerIPPerDay),
		shareInvitationAccountLimiter: createDailyLimiter(shareInvitationPerAccountPerDay),
		shareInvitationIPLimiter:      createDailyLimiter(shareInvitationPerIPPerDay),
		shareAcceptedEmailLimiter:     createDailyLimiter(shareAcceptedEmailPerInviterPerDay),
		shareLinkAccountLimiter:       createDailyLimiter(shareLinkPerAccountPerDay),
		shareLinkReadLimiter:          createShareLinkReadLimiter(),
		reauthenticationLimiter:       createReauthenticationLimiter(),
		invitePageLimiter:             createInvitePageLimiter(),
		versionCheckLimiter:           createVersionCheckLimiter(),
	}, nil
}

func (s *Server) CreateApp() *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:      "hisar-server",
		BodyLimit:    maxRotationBody,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  120 * time.Second,
		// fasthttp reuses request buffers after a handler returns.
		Immutable:    true,
		ErrorHandler: handleError,
	})
	app.Use(setAllSecurityHeader)
	app.Get("/healthz", func(c fiber.Ctx) error { return c.SendString("ok\n") })
	app.Get("/privacy", sendPrivacyPolicy)
	app.Get("/terms", sendTermsOfService)
	app.Get("/s/:id", sendShareLinkPage)
	app.Get("/invite/:token", s.limitInvitePageRate, s.sendInvitePage)

	apiGroup := app.Group("/api/v1")
	apiGroup.Use(s.requireMinimumVersion)
	apiGroup.Get("/version-check", s.limitVersionCheckRate, handleCheckVersion)
	apiGroup.Get("/auth/prelogin", s.limitRate, s.handlePrelogin)
	apiGroup.Post("/auth/register/start", s.limitRate, s.handleRegisterStart)
	apiGroup.Post("/auth/register/finish", s.limitRate, s.handleRegisterFinish)
	apiGroup.Post("/auth/login/start", s.limitRate, s.handleLoginStart)
	apiGroup.Post("/auth/login/finish", s.limitRate, s.handleLoginFinish)

	apiGroup.Post("/auth/logout", s.requireSession, s.handleLogout)
	apiGroup.Get("/events", s.requireSession, s.handleStreamEvents)
	apiGroup.Get("/account", s.requireSession, s.handleGetAccount)
	apiGroup.Post("/account/reauthenticate", s.requireSession, s.limitRate, s.handleReauthenticate)
	apiGroup.Put("/account/credentials", s.requireSession, s.handleChangeCredentials)
	apiGroup.Get("/account/deletion-preview", s.requireSession, s.handleGetAccountDeletionPreview)
	apiGroup.Post("/account/delete", s.requireSession, s.handleDeleteAccount)
	apiGroup.Get("/members", s.requireSession, s.handleListAllMember)
	apiGroup.Post("/invites", s.requireSession, s.handleCreateInvite)
	apiGroup.Get("/devices", s.requireSession, s.handleListAllDevice)
	apiGroup.Delete("/devices/:id", s.requireSession, s.handleRevokeDevice)

	apiGroup.Get("/vaults", s.requireSession, s.handleListAllVault)
	apiGroup.Post("/vaults", s.requireSession, s.handleCreateVault)
	apiGroup.Get("/vaults/:vault/members", s.requireSession, s.handleListAllVaultMember)
	apiGroup.Post("/vaults/:vault/share-invitations", s.requireSession, s.handleCreateShareInvitation)
	apiGroup.Get("/share-invitations", s.requireSession, s.handleListAllShareInvitation)
	apiGroup.Post("/share-invitations/:invitation/accept", s.requireSession, s.handleAcceptShareInvitation)
	apiGroup.Post("/share-invitations/:invitation/decline", s.requireSession, s.handleDeclineShareInvitation)
	apiGroup.Delete("/share-invitations/:invitation", s.requireSession, s.handleCancelShareInvitation)
	apiGroup.Put("/vaults/:vault/access/:account", s.requireSession, s.handleSetAccess)
	apiGroup.Delete("/vaults/:vault/access/:account", s.requireSession, s.handleRemoveAccess)
	apiGroup.Post("/vaults/:vault/rotate", s.requireSession, s.handleRotateVault)
	apiGroup.Put("/vaults/:vault/name", s.requireSession, s.handleRenameVault)
	apiGroup.Post("/vaults/:vault/delete", s.requireSession, s.handleDeleteVault)
	apiGroup.Get("/vaults/:vault/changes", s.requireSession, s.handleListAllChange)
	apiGroup.Put("/vaults/:vault/items/:item", s.requireSession, s.handlePutItem)
	apiGroup.Get("/vaults/:vault/items/:item/history", s.requireSession, s.handleGetItemHistory)

	apiGroup.Post("/share-links", s.requireSession, s.handleCreateShareLink)
	apiGroup.Get("/share-links", s.requireSession, s.handleListAllShareLink)
	apiGroup.Delete("/share-links/:id", s.requireSession, s.handleRevokeShareLink)
	apiGroup.Get("/public/share-links/:id", s.handleViewShareLink)

	apiGroup.Post("/approvals", s.requireSession, s.limitApprovalRate, s.handleCreateApproval)
	apiGroup.Get("/approvals", s.requireSession, s.limitApprovalRate, s.handleListAllPendingApproval)
	apiGroup.Get("/approvals/:id", s.requireSession, s.limitApprovalRate, s.handleGetApproval)
	apiGroup.Post("/approvals/:id/answer", s.requireSession, s.limitApprovalRate, s.handleAnswerApproval)
	apiGroup.Delete("/approvals/:id", s.requireSession, s.limitApprovalRate, s.handleResolveApproval)

	apiGroup.Get("/desktop/updates/appcast.xml", s.limitUpdateRate, s.requireSession, s.handleGetDesktopAppcast)
	apiGroup.Get("/desktop/updates/:version/:file", s.limitUpdateRate, s.requireSession, s.handleDownloadDesktopUpdate)

	return app
}

// privacyPolicy is linked from the Chrome Web Store and App Store listings.
//
//go:embed privacy.html
var privacyPolicy []byte

//go:embed terms.html
var termsOfService []byte

func sendPrivacyPolicy(c fiber.Ctx) error { return sendLegalPage(c, privacyPolicy) }

func sendTermsOfService(c fiber.Ctx) error { return sendLegalPage(c, termsOfService) }

func sendLegalPage(c fiber.Ctx, page []byte) error {
	c.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'")
	c.Set("Cache-Control", "public, max-age=3600")
	c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
	return c.Send(page)
}

func setAllSecurityHeader(c fiber.Ctx) error {
	c.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("X-Frame-Options", "DENY")
	c.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	c.Set("Cache-Control", "no-store")
	return c.Next()
}

// handleError renders *fiber.Error as JSON and hides all other errors behind a 500.
func handleError(c fiber.Ctx, err error) error {
	var fiberError *fiber.Error
	if !errors.As(err, &fiberError) {
		log.Printf("internal error: %s %s: %v", c.Method(), c.Path(), err)
		fiberError = fiber.NewError(fiber.StatusInternalServerError, "internal error")
	}
	message := fiberError.Message
	if fiberError.Code == fiber.StatusRequestEntityTooLarge {
		message = "request too large"
	}
	return c.Status(fiberError.Code).JSON(apitypes.Error{Error: message})
}

func createAPIError(status int, message string) error { return fiber.NewError(status, message) }

var errNotFound = createAPIError(fiber.StatusNotFound, "not found")

type ctxKey struct{}

func getSession(c fiber.Ctx) *store.Session {
	return c.Locals(ctxKey{}).(*store.Session)
}

func (s *Server) requireSession(c fiber.Ctx) error {
	token, ok := strings.CutPrefix(c.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		return createAPIError(fiber.StatusUnauthorized, "unauthorized")
	}
	currentSession, err := s.store.GetSessionByHash(c.Context(), hashToken(token), s.config.SessionTTL)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			log.Printf("session lookup: %v", err)
		}
		return createAPIError(fiber.StatusUnauthorized, "unauthorized")
	}
	c.Locals(ctxKey{}, currentSession)
	return c.Next()
}

func (s *Server) limitRate(c fiber.Ctx) error {
	if !s.limiter.allow(s.getClientIP(c)) {
		return createAPIError(fiber.StatusTooManyRequests, "too many requests")
	}
	return c.Next()
}

// With TrustProxy, the last X-Forwarded-For entry is the one appended by our reverse proxy.
func (s *Server) getClientIP(c fiber.Ctx) string {
	if s.config.TrustProxy {
		if forwardedFor := c.Get("X-Forwarded-For"); forwardedFor != "" {
			allForwardedIP := strings.Split(forwardedFor, ",")
			return strings.TrimSpace(allForwardedIP[len(allForwardedIP)-1])
		}
	}
	return c.RequestCtx().RemoteIP().String()
}

func (s *Server) recordAudit(c fiber.Ctx, accountID, deviceID string, eventType string, detail any) {
	var optionalAccountID, optionalDeviceID *string
	if accountID != "" {
		optionalAccountID = &accountID
	}
	if deviceID != "" {
		optionalDeviceID = &deviceID
	}
	s.store.RecordAudit(c.Context(), optionalAccountID, optionalDeviceID, eventType, detail, s.getClientIP(c))
}

func hashToken(token string) []byte {
	hash := sha256.Sum256([]byte(token))
	return hash[:]
}

func decodeBody(c fiber.Ctx, target any) error {
	return decodeBodyWithLimit(c, target, maxBody)
}

func decodeBodyWithLimit(c fiber.Ctx, target any, limit int) error {
	if len(c.Body()) > limit {
		return createAPIError(fiber.StatusRequestEntityTooLarge, "request too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(c.Body()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return createAPIError(fiber.StatusBadRequest, "invalid request body")
	}
	return nil
}

type ipLimiter struct {
	mutex     sync.Mutex
	every     rate.Limit
	burst     int
	allClient map[string]*ipEntry
}

type ipEntry struct {
	limiter *rate.Limiter
	seen    time.Time
}

func createIPLimiter(every rate.Limit, burst int) *ipLimiter {
	return &ipLimiter{every: every, burst: burst, allClient: map[string]*ipEntry{}}
}

func (l *ipLimiter) allow(ip string) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	now := time.Now()
	if len(l.allClient) > 10000 {
		for clientIP, entry := range l.allClient {
			if now.Sub(entry.seen) > 10*time.Minute {
				delete(l.allClient, clientIP)
			}
		}
	}
	entry, ok := l.allClient[ip]
	if !ok {
		entry = &ipEntry{limiter: rate.NewLimiter(l.every, l.burst)}
		l.allClient[ip] = entry
	}
	entry.seen = now
	return entry.limiter.Allow()
}
