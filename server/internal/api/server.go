package api

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/pake"
	"github.com/tryhisar/hisar/server/internal/desktopupdate"
	"github.com/tryhisar/hisar/server/internal/ratelimit"
	"github.com/tryhisar/hisar/server/internal/realtime"
	"github.com/tryhisar/hisar/server/internal/service"
	"github.com/tryhisar/hisar/server/internal/store"
)

// The transport accepts rotation-sized bodies; decodeBody enforces maxBody everywhere else.
const (
	maxBody         = 2 << 20
	maxRotationBody = 32 << 20
)

type Config struct {
	// Config holds what the service reads: the public URL, session and invite lifetimes, the mailer.
	service.Config
	TrustProxy bool
	// AuthBurst is how many auth requests one IP may make before being limited to one per 6 seconds.
	AuthBurst int
	// EventHeartbeatInterval keeps event streams alive through proxies and rechecks their session.
	EventHeartbeatInterval time.Duration
	// WriteTimeout limits writing one response; event streams extend it with every write.
	WriteTimeout time.Duration
	// DesktopUpdates is nil when the server has no update source.
	DesktopUpdates *desktopupdate.Source
	// MinimumVersions refuses older apps with 426; independent of the latest version offered for updates.
	MinimumVersions MinimumVersions
}

// Server answers HTTP: it parses requests, limits them per IP and hands them to the service.
type Server struct {
	config  Config
	events  *realtime.Broker
	service *service.Service

	limiter              *ratelimit.Limiter
	updateLimiter        *ratelimit.Limiter
	shareLinkReadLimiter *ratelimit.Limiter
	invitePageLimiter    *ratelimit.Limiter
	versionCheckLimiter  *ratelimit.Limiter
}

func NewServer(database *store.Store, secrets *Secrets, config Config) (*Server, error) {
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
		config.ShareAcceptedEmailDelay = service.DefaultShareAcceptedEmailDelay
	}
	opaqueConfiguration := pake.NewConfig()
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
	server := &Server{
		config:               config,
		events:               realtime.NewBroker(),
		limiter:              ratelimit.New(rate.Every(6*time.Second), config.AuthBurst),
		updateLimiter:        ratelimit.New(rate.Every(10*time.Second), 20),
		shareLinkReadLimiter: newShareLinkReadLimiter(),
		invitePageLimiter:    newInvitePageLimiter(),
		versionCheckLimiter:  newVersionCheckLimiter(),
	}
	keys := service.Keys{OpaqueConfiguration: opaqueConfiguration, OpaqueServer: opaqueServer, HMACKey: secrets.HMACKey}
	// The service reads the server's config, so a change before serving reaches it.
	server.service = service.New(database, server.events, keys, &server.config.Config)
	return server, nil
}

// CreateInvite is the operator's invite: it returns the link instead of emailing it.
func (s *Server) CreateInvite(ctx context.Context, email, role string, createdBy *string) (*apitypes.InviteResponse, error) {
	return s.service.CreateOperatorInvite(ctx, email, role, createdBy)
}

// WaitForBackgroundWork waits for invites and share notifications that were started before it was called.
func (s *Server) WaitForBackgroundWork() { s.service.WaitForBackgroundWork() }

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
	app.Use(setSecurityHeaders)
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
	apiGroup.Get("/members", s.requireSession, s.handleListMembers)
	apiGroup.Post("/invites", s.requireSession, s.handleCreateInvite)
	apiGroup.Get("/devices", s.requireSession, s.handleListDevices)
	apiGroup.Delete("/devices/:id", s.requireSession, s.handleRevokeDevice)

	apiGroup.Get("/vaults", s.requireSession, s.handleListVaults)
	apiGroup.Post("/vaults", s.requireSession, s.handleCreateVault)
	apiGroup.Get("/vaults/:vault/members", s.requireSession, s.handleListVaultMembers)
	apiGroup.Post("/vaults/:vault/share-invitations", s.requireSession, s.handleCreateShareInvitation)
	apiGroup.Get("/share-invitations", s.requireSession, s.handleListShareInvitations)
	apiGroup.Post("/share-invitations/:invitation/accept", s.requireSession, s.handleAcceptShareInvitation)
	apiGroup.Post("/share-invitations/:invitation/decline", s.requireSession, s.handleDeclineShareInvitation)
	apiGroup.Delete("/share-invitations/:invitation", s.requireSession, s.handleCancelShareInvitation)
	apiGroup.Put("/vaults/:vault/access/:account", s.requireSession, s.handleSetAccess)
	apiGroup.Delete("/vaults/:vault/access/:account", s.requireSession, s.handleRemoveAccess)
	apiGroup.Post("/vaults/:vault/rotate", s.requireSession, s.handleRotateVault)
	apiGroup.Put("/vaults/:vault/name", s.requireSession, s.handleRenameVault)
	apiGroup.Post("/vaults/:vault/delete", s.requireSession, s.handleDeleteVault)
	apiGroup.Get("/vaults/:vault/changes", s.requireSession, s.handleListChanges)
	apiGroup.Put("/vaults/:vault/items/:item", s.requireSession, s.handlePutItem)
	apiGroup.Get("/vaults/:vault/items/:item/history", s.requireSession, s.handleGetItemHistory)

	apiGroup.Post("/share-links", s.requireSession, s.handleCreateShareLink)
	apiGroup.Get("/share-links", s.requireSession, s.handleListShareLinks)
	apiGroup.Delete("/share-links/:id", s.requireSession, s.handleRevokeShareLink)
	apiGroup.Get("/public/share-links/:id", s.handleViewShareLink)

	apiGroup.Post("/approvals", s.requireSession, s.limitApprovalRate, s.handleCreateApproval)
	apiGroup.Get("/approvals", s.requireSession, s.limitApprovalRate, s.handleListPendingApprovals)
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

func setSecurityHeaders(c fiber.Ctx) error {
	c.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("X-Frame-Options", "DENY")
	c.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	c.Set("Cache-Control", "no-store")
	return c.Next()
}

// statusByKind is the HTTP status of each kind of refusal from the service.
var statusByKind = map[service.Kind]int{
	service.KindInvalid:         fiber.StatusBadRequest,
	service.KindUnauthorized:    fiber.StatusUnauthorized,
	service.KindForbidden:       fiber.StatusForbidden,
	service.KindNotFound:        fiber.StatusNotFound,
	service.KindConflict:        fiber.StatusConflict,
	service.KindTooManyRequests: fiber.StatusTooManyRequests,
	service.KindUnavailable:     fiber.StatusServiceUnavailable,
	service.KindNotImplemented:  fiber.StatusNotImplemented,
	service.KindGone:            fiber.StatusGone,
}

// handleError renders refusals (*service.Error, *fiber.Error) as JSON and hides all other errors behind a 500.
func handleError(c fiber.Ctx, err error) error {
	var serviceError *service.Error
	if errors.As(err, &serviceError) {
		err = fiber.NewError(statusByKind[serviceError.Kind], serviceError.Message)
	}
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

func getSession(c fiber.Ctx) *service.Session {
	return c.Locals(ctxKey{}).(*service.Session)
}

// getActor is who makes the request, for the service.
func (s *Server) getActor(c fiber.Ctx) service.Actor {
	actor := service.Actor{IP: s.getClientIP(c)}
	if currentSession, isSignedIn := c.Locals(ctxKey{}).(*service.Session); isSignedIn {
		actor.AccountID, actor.DeviceID, actor.TokenHash = currentSession.AccountID, currentSession.DeviceID, currentSession.TokenHash
	}
	return actor
}

func (s *Server) requireSession(c fiber.Ctx) error {
	token, ok := strings.CutPrefix(c.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		return createAPIError(fiber.StatusUnauthorized, "unauthorized")
	}
	currentSession, err := s.service.Authenticate(c.Context(), token)
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
	if !s.limiter.Allow(s.getClientIP(c)) {
		return createAPIError(fiber.StatusTooManyRequests, "too many requests")
	}
	return c.Next()
}

// With TrustProxy, the last X-Forwarded-For entry is the one appended by our reverse proxy.
func (s *Server) getClientIP(c fiber.Ctx) string {
	if s.config.TrustProxy {
		if forwardedFor := c.Get("X-Forwarded-For"); forwardedFor != "" {
			forwardedIPs := strings.Split(forwardedFor, ",")
			return strings.TrimSpace(forwardedIPs[len(forwardedIPs)-1])
		}
	}
	return c.RequestCtx().RemoteIP().String()
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
