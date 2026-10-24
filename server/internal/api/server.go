package api

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/netip"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/valyala/fasthttp"
	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/pake"
	"github.com/tryhisar/hisar/server/internal/desktopupdate"
	"github.com/tryhisar/hisar/server/internal/ratelimit"
	"github.com/tryhisar/hisar/server/internal/realtime"
	"github.com/tryhisar/hisar/server/internal/serverupdate"
	"github.com/tryhisar/hisar/server/internal/service"
	"github.com/tryhisar/hisar/server/internal/settings"
	"github.com/tryhisar/hisar/server/internal/store"
)

// The transport refuses bodies above maxMoveBody, except for limitRequestBody's rotation-sized requests; decodeBody
// enforces maxBody below that.
const (
	maxBody         = 2 << 20
	maxRotationBody = 32 << 20
	// maxMoveBody holds a moved item of the largest size, base64-encoded, and its small tombstone.
	maxMoveBody = 4 << 20
)

// rotationPath permits authenticated bounded transition uploads as well as legacy aggregate requests.
var rotationPath = regexp.MustCompile(`^/api/v1/vaults/[^/]+/(rotate|delete|transitions(/[^/]+/(items|access))?)$`)

type Config struct {
	// Config holds what the service reads: the public URL.
	service.Config
	TrustProxy bool
	// DefaultSettings are the instance settings before the stored ones and the environment; nil means settings.Defaults.
	DefaultSettings *settings.Settings
	// Environment overrides the stored instance settings, field by field; nil reads no variables.
	Environment settings.Environment
	// EventHeartbeatInterval keeps event streams alive through proxies and rechecks their session.
	EventHeartbeatInterval time.Duration
	// WriteTimeout limits writing one response; event streams extend it with every write.
	WriteTimeout time.Duration
	// RequestTimeout ends the context, and so the database work, of one request's handler.
	RequestTimeout time.Duration
	// DesktopUpdates is nil when the server has no update source.
	DesktopUpdates *desktopupdate.Source
	// Version is hisar-server's release version, for Overview.
	Version string
	// ServerUpdates is nil when update checks are off.
	ServerUpdates *serverupdate.Source
}

// Server answers HTTP: it parses requests, limits them per IP and hands them to the service.
type Server struct {
	config  Config
	events  *realtime.Broker
	service *service.Service
	// instanceSettings is the snapshot of the effective settings; a save replaces it whole.
	instanceSettings atomic.Pointer[settings.Settings]

	limiter              *ratelimit.Limiter
	updateLimiter        *ratelimit.Limiter
	shareLinkReadLimiter *ratelimit.Limiter
	invitePageLimiter    *ratelimit.Limiter
	versionCheckLimiter  *ratelimit.Limiter
}

// NewServer reads the stored instance settings, when it has a database, and refuses settings that fail Validate.
func NewServer(database *store.Store, secrets *Secrets, config Config) (*Server, error) {
	if config.PendingLoginCapacity < 0 || config.LimiterKeyCapacity < 0 || config.LimiterTotalCapacity < 0 {
		return nil, fmt.Errorf("resource capacities must be positive")
	}
	opaqueConfiguration := pake.NewConfig()
	serverKeyMaterial, err := opaqueConfiguration.DecodeServerKeyMaterial(secrets.OpaqueKeyMaterial)
	if err != nil {
		return nil, err
	}
	defaultSettings := settings.Defaults()
	if config.DefaultSettings != nil {
		defaultSettings = *config.DefaultSettings
	}
	var storedSettings []byte
	if database != nil {
		var err error
		if err := database.InitializeOperationalSecrets(context.Background(), secrets.fingerprint(), secrets.HMACKey); err != nil {
			return nil, fmt.Errorf("initialize operational secrets: %w", err)
		}
		if storedSettings, err = database.GetInstanceSettings(context.Background()); err != nil {
			return nil, fmt.Errorf("read instance settings: %w", err)
		}
	}
	effectiveSettings, _, err := settings.Resolve(defaultSettings, storedSettings, config.Environment)
	if err != nil {
		return nil, err
	}
	if err := effectiveSettings.Validate(); err != nil {
		return nil, fmt.Errorf("instance settings: %w", err)
	}
	if config.EventHeartbeatInterval == 0 {
		config.EventHeartbeatInterval = eventHeartbeatInterval
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = 30 * time.Second
	}
	// Apps give up on a request after 30 s, so work past this answers nobody.
	if config.RequestTimeout == 0 {
		config.RequestTimeout = time.Minute
	}
	opaqueServer, err := opaqueConfiguration.Server()
	if err != nil {
		return nil, err
	}
	if err := opaqueServer.SetKeyMaterial(serverKeyMaterial); err != nil {
		return nil, err
	}
	loginConfiguration, err := pake.NewAuthConfig(config.PublicURL, pake.Login)
	if err != nil {
		return nil, fmt.Errorf("public URL for authentication: %w", err)
	}
	loginServer, err := loginConfiguration.Server()
	if err != nil {
		return nil, err
	}
	if err := loginServer.SetKeyMaterial(serverKeyMaterial); err != nil {
		return nil, err
	}
	reauthenticationConfiguration, err := pake.NewAuthConfig(config.PublicURL, pake.Reauthenticate)
	if err != nil {
		return nil, err
	}
	reauthenticationServer, err := reauthenticationConfiguration.Server()
	if err != nil {
		return nil, err
	}
	if err := reauthenticationServer.SetKeyMaterial(serverKeyMaterial); err != nil {
		return nil, err
	}
	factory := ratelimit.NewFactory(config.LimiterKeyCapacity, config.LimiterTotalCapacity)
	server := &Server{
		config:               config,
		events:               realtime.NewBroker(),
		limiter:              factory.New(rate.Every(6*time.Second), effectiveSettings.AuthBurst),
		updateLimiter:        factory.New(rate.Every(10*time.Second), 20),
		shareLinkReadLimiter: factory.New(rate.Every(shareLinkReadInterval), shareLinkReadBurst),
		invitePageLimiter:    factory.New(rate.Every(2*time.Second), 30),
		versionCheckLimiter:  factory.New(rate.Every(2*time.Second), 30),
	}
	server.instanceSettings.Store(&effectiveSettings)
	keys := service.Keys{OpaqueConfiguration: opaqueConfiguration, OpaqueServer: opaqueServer, HMACKey: secrets.HMACKey,
		LoginServer: loginServer, ReauthenticationServer: reauthenticationServer}
	server.service = service.New(database, server.events, keys, config.Config, service.InstanceSettings{
		Current: &server.instanceSettings, Defaults: defaultSettings, Environment: config.Environment,
		// A new auth burst applies to the limiter at once, in the order of the saves.
		OnSave: func(saved settings.Settings) { server.limiter.SetBurst(saved.AuthBurst) },
	}, factory)
	return server, nil
}

// Settings returns the effective instance settings of the moment.
func (s *Server) Settings() settings.Settings { return *s.instanceSettings.Load() }

// CreateInvite is the operator's invite: it returns the link instead of emailing it.
func (s *Server) CreateInvite(ctx context.Context, email, role string, createdBy *string) (*apitypes.InviteResponse, error) {
	return s.service.CreateOperatorInvite(ctx, email, role, createdBy)
}

// WaitForBackgroundWork waits for invites and share notifications that were started before it was called.
func (s *Server) WaitForBackgroundWork() { s.service.WaitForBackgroundWork() }

func (s *Server) CreateApp() *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:      "hisar-server",
		BodyLimit:    maxMoveBody,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  120 * time.Second,
		// fasthttp reuses request buffers after a handler returns.
		Immutable:                    true,
		DisablePreParseMultipartForm: true,
		ErrorHandler:                 handleError,
	})
	app.Server().HeaderReceived = s.limitRequestBody
	app.Use(setSecurityHeaders, s.limitRequestTime)
	app.Get("/healthz", func(c fiber.Ctx) error { return c.SendString("ok\n") })
	app.Get("/privacy", sendPrivacyPolicy)
	app.Get("/terms", sendTermsOfService)
	app.Get("/s/:id", sendShareLinkPage)
	app.Get("/invite/:token", s.limitInvitePageRate, s.sendInvitePage)
	newAdminPage(builtAdminPageFiles()).register(app)

	apiGroup := app.Group("/api/v1")
	apiGroup.Use(s.requireMinimumVersion)
	apiGroup.Get("/version-check", s.limitVersionCheckRate, handleCheckVersion)
	apiGroup.Get("/auth/prelogin", s.limitRate, s.handlePrelogin)
	apiGroup.Post("/auth/register/start", s.limitRate, s.handleRegisterStart)
	apiGroup.Post("/auth/register/finish", s.limitRate, s.handleRegisterFinish)
	apiGroup.Post("/auth/login/start", s.limitRate, refuseLegacyAuthentication)
	apiGroup.Post("/auth/login/finish", s.limitRate, refuseLegacyAuthentication)
	apiGroup.Post("/auth/login/v2/start", s.limitRate, s.handleLoginStart)
	apiGroup.Post("/auth/login/v2/finish", s.limitRate, s.handleLoginFinish)

	apiGroup.Post("/auth/logout", s.requireSession, s.handleLogout)
	apiGroup.Get("/events", s.requireSession, s.handleStreamEvents)
	apiGroup.Get("/account", s.requireSession, s.handleGetAccount)
	apiGroup.Post("/account/reauthenticate", s.requireSession, s.limitRate, refuseLegacyAuthentication)
	apiGroup.Post("/account/reauthenticate/v2", s.requireSession, s.limitRate, s.handleReauthenticate)
	apiGroup.Put("/account/credentials", s.requireSession, s.handleChangeCredentials)
	apiGroup.Get("/account/deletion-preview", s.requireSession, s.handleGetAccountDeletionPreview)
	apiGroup.Post("/account/delete", s.requireSession, s.handleDeleteAccount)
	apiGroup.Get("/account/preferences", s.requireSession, s.handleGetAccountPreferences)
	apiGroup.Put("/account/preferences", s.requireSession, s.handlePutAccountPreferences)
	apiGroup.Get("/members", s.requireSession, s.handleListMembers)
	apiGroup.Post("/invites", s.requireSession, s.handleCreateInvite)
	apiGroup.Get("/devices", s.requireSession, s.handleListDevices)
	apiGroup.Delete("/devices/:id", s.requireSession, s.handleRevokeDevice)
	apiGroup.Get("/push/config", s.requireSession, s.handleGetPushConfig)
	apiGroup.Put("/devices/current/push", s.requireSession, s.handleSetDevicePush)
	apiGroup.Delete("/devices/current/push", s.requireSession, s.handleDeleteDevicePush)

	apiGroup.Get("/vaults", s.requireSession, s.handleListVaults)
	apiGroup.Post("/vaults", s.requireSession, s.handleCreateVault)
	apiGroup.Post("/vaults/personal", s.requireSession, s.handleCreateVault)
	apiGroup.Get("/vaults/:vault/members", s.requireSession, s.handleListVaultMembers)
	apiGroup.Post("/vaults/:vault/share-invitations", s.requireSession, s.handleCreateShareInvitation)
	apiGroup.Post("/vaults/:vault/signed-share-invitations", s.requireSession, s.handleCreateShareInvitation)
	apiGroup.Post("/share-invitations/:invitation/complete", s.requireSession, s.handleCompleteSignedShare)
	apiGroup.Get("/share-invitations", s.requireSession, s.handleListShareInvitations)
	apiGroup.Post("/share-invitations/:invitation/accept", s.requireSession, s.handleAcceptShareInvitation)
	apiGroup.Post("/share-invitations/:invitation/decline", s.requireSession, s.handleDeclineShareInvitation)
	apiGroup.Delete("/share-invitations/:invitation", s.requireSession, s.handleCancelShareInvitation)
	apiGroup.Put("/vaults/:vault/access/:account", s.requireSession, s.handleSetAccess)
	apiGroup.Delete("/vaults/:vault/access/:account", s.requireSession, s.handleRemoveAccess)
	apiGroup.Post("/vaults/:vault/rotate", s.requireSession, s.handleRotateVault)
	apiGroup.Post("/vaults/:vault/transitions", s.requireSession, s.handleBeginVaultTransition)
	apiGroup.Get("/vaults/:vault/transitions", s.requireSession, s.handleVaultTransitionProtocol)
	apiGroup.Get("/vaults/:vault/transitions/:transition", s.requireSession, s.handleGetVaultTransition)
	apiGroup.Get("/vaults/:vault/transitions/:transition/items", s.requireSession, s.handleListVaultTransitionItems)
	apiGroup.Post("/vaults/:vault/transitions/:transition/items", s.requireSession, s.handleStageVaultItems)
	apiGroup.Post("/vaults/:vault/transitions/:transition/access", s.requireSession, s.handleStageVaultAccess)
	apiGroup.Post("/vaults/:vault/transitions/:transition/commit", s.requireSession, s.handleCommitVaultTransition)
	apiGroup.Post("/vaults/:vault/transitions/:transition/abort", s.requireSession, s.handleAbortVaultTransition)
	apiGroup.Put("/vaults/:vault/name", s.requireSession, s.handleRenameVault)
	apiGroup.Post("/vaults/:vault/delete", s.requireSession, s.handleDeleteVault)
	apiGroup.Post("/vaults/:vault/recovery/name", s.requireSession, s.handleRepairVaultName)
	apiGroup.Post("/vaults/:vault/recovery/delete", s.requireSession, s.handleDeleteUnreadableVault)
	apiGroup.Get("/vaults/:vault/changes", s.requireSession, s.handleListChanges)
	apiGroup.Put("/vaults/:vault/items/:item", s.requireSession, s.handlePutItem)
	apiGroup.Put("/vaults/:vault/items/:item/import", s.requireSession, s.handlePutImportItem)
	apiGroup.Get("/vaults/:vault/items/:item/history", s.requireSession, s.handleGetItemHistory)
	apiGroup.Post("/vaults/:vault/items/:item/move", s.requireSession, s.handleMoveItem)

	apiGroup.Get("/organizations", s.requireSession, s.handleListOrganizations)
	apiGroup.Post("/organizations", s.requireSession, s.handleCreateOrganization)
	apiGroup.Get("/organizations/:organization", s.requireSession, s.handleGetOrganization)
	apiGroup.Put("/organizations/:organization/name", s.requireSession, s.handleRenameOrganization)
	apiGroup.Delete("/organizations/:organization", s.requireSession, s.handleDeleteOrganization)
	apiGroup.Post("/organizations/:organization/invitations", s.requireSession, s.handleCreateOrganizationInvitation)
	apiGroup.Delete("/organizations/:organization/invitations/:invitation", s.requireSession, s.handleCancelOrganizationInvitation)
	apiGroup.Post("/organization-invitations/:invitation/accept", s.requireSession, s.handleAcceptOrganizationInvitation)
	apiGroup.Post("/organization-invitations/:invitation/decline", s.requireSession, s.handleDeclineOrganizationInvitation)
	apiGroup.Put("/organizations/:organization/members/:account", s.requireSession, s.handleSetOrganizationRole)
	apiGroup.Delete("/organizations/:organization/members/:account", s.requireSession, s.handleRemoveOrganizationMember)
	apiGroup.Post("/organizations/:organization/teams", s.requireSession, s.handleCreateTeam)
	apiGroup.Put("/organizations/:organization/teams/:team/name", s.requireSession, s.handleRenameTeam)
	apiGroup.Delete("/organizations/:organization/teams/:team", s.requireSession, s.handleDeleteTeam)
	apiGroup.Put("/organizations/:organization/teams/:team/members/:account", s.requireSession, s.handleAddTeamMember)
	apiGroup.Delete("/organizations/:organization/teams/:team/members/:account", s.requireSession, s.handleRemoveTeamMember)
	apiGroup.Put("/organizations/:organization/teams/:team/vaults/:vault", s.requireSession, s.handleGrantTeamVault)
	apiGroup.Delete("/organizations/:organization/teams/:team/vaults/:vault", s.requireSession, s.handleRevokeTeamVault)
	apiGroup.Get("/vault-key-requests", s.requireSession, s.handleListVaultKeyRequests)

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

	adminGroup := apiGroup.Group("/admin")
	s.registerAdminServerRoutes(adminGroup)
	s.registerAdminSettingsRoutes(adminGroup)
	s.registerAdminHealthRoutes(adminGroup)
	s.registerAdminAccountRoutes(adminGroup)
	s.registerAdminInvitesRoutes(adminGroup)
	s.registerSetupRoutes(apiGroup, adminGroup)
	s.registerAdminAuditRoutes(adminGroup)

	return app
}

// limitRequestTime gives the handlers a context that ends after RequestTimeout; fasthttp's own never ends, not even
// when the client goes away. Streamed bodies are written after the handler returns, so they bring their own.
func (s *Server) limitRequestTime(c fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.config.RequestTimeout)
	defer cancel()
	c.SetContext(ctx)
	return c.Next()
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
	service.KindInvalid:            fiber.StatusBadRequest,
	service.KindUnauthorized:       fiber.StatusUnauthorized,
	service.KindForbidden:          fiber.StatusForbidden,
	service.KindNotFound:           fiber.StatusNotFound,
	service.KindConflict:           fiber.StatusConflict,
	service.KindTooManyRequests:    fiber.StatusTooManyRequests,
	service.KindUnavailable:        fiber.StatusServiceUnavailable,
	service.KindNotImplemented:     fiber.StatusNotImplemented,
	service.KindGone:               fiber.StatusGone,
	service.KindPreconditionFailed: fiber.StatusPreconditionFailed,
}

// resourceBudgetMessages tells the user what to remove when a growth budget (store.ExceededResource) refuses a change.
var resourceBudgetMessages = map[string]string{
	"vaults":               "You have created as many vaults as one account can. Delete vaults you no longer need, then try again.",
	"devices":              "This account is already signed in on as many devices as it can be. Sign out devices you no longer use in Settings > Devices on a device that is still signed in, then try again.",
	"organizations":        "You have created as many organizations as one account can. Delete an organization you no longer need, then try again.",
	"teams":                "This organization has as many teams as it can have. Delete teams it no longer needs, then try again.",
	"organization members": "This organization has as many members as it can have. Remove people who no longer need it, then try again.",
	"team memberships":     "This organization's teams have as many members as they can have. Remove people from teams they no longer need, then try again.",
	"team grants":          "This organization's teams have access to as many vaults as they can. Remove vaults from teams that no longer need them, then try again.",
	"vault grants":         "This vault is shared with as many people as it can be. Remove people who no longer need it, then try again.",
	"history":              "This vault's item history is full. Its items stay readable and exportable. Delete items you no longer need, or wait for the server to remove old item versions, then try again.",
}

// handleError renders refusals (*service.Error, *fiber.Error) as JSON and hides all other errors behind a 500. An item
// conflict answers 409 with the current item, which the client needs to merge; items sealed with a key a rotation
// replaced answer 409 with apitypes.KeyChangedResponse, and a deletion whose plan changed with
// apitypes.DeletionPlanChangedResponse. A growth budget answers 507, which no client reads as a conflict to merge or
// retry, and which clients of every version show with its sentence.
func handleError(c fiber.Ctx, err error) error {
	if resource := store.ExceededResource(err); resource != "" {
		message, ok := resourceBudgetMessages[resource]
		if !ok {
			message = "This has reached its growth limit on the server. Delete what you no longer need, then try again."
		}
		return c.Status(fiber.StatusInsufficientStorage).JSON(apitypes.Error{Error: message})
	}
	var conflict *service.ItemConflictError
	if errors.As(err, &conflict) {
		return c.Status(fiber.StatusConflict).JSON(apitypes.ConflictResponse{Current: conflict.Current})
	}
	if errors.Is(err, service.ErrKeyChanged) {
		return c.Status(fiber.StatusConflict).JSON(apitypes.KeyChangedResponse{Error: service.ErrKeyChanged.Message, IsKeyChanged: true})
	}
	if errors.Is(err, service.ErrDeletionPlanChanged) {
		return c.Status(fiber.StatusConflict).JSON(apitypes.DeletionPlanChangedResponse{Error: service.ErrDeletionPlanChanged.Message, IsDeletionPlanChanged: true})
	}
	var serviceError *service.Error
	if errors.As(err, &serviceError) {
		err = fiber.NewError(statusByKind[serviceError.Kind], serviceError.Message)
	}
	var fiberError *fiber.Error
	if !errors.As(err, &fiberError) {
		// The route, not the path: invite tokens and share link IDs are capabilities.
		log.Printf("internal error: %s %s: %v", c.Method(), c.Route().Path, err)
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
		actor.AccountID, actor.DeviceID, actor.TokenHash, actor.Role = currentSession.AccountID, currentSession.DeviceID, currentSession.TokenHash, currentSession.Role
	}
	return actor
}

func (s *Server) requireSession(c fiber.Ctx) error {
	token, ok := strings.CutPrefix(c.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		return createAPIError(fiber.StatusUnauthorized, "unauthorized")
	}
	currentSession, err := s.service.Authenticate(c.Context(), token)
	var serviceError *service.Error
	switch {
	case errors.Is(err, store.ErrNotFound):
		return createAPIError(fiber.StatusUnauthorized, "unauthorized")
	case errors.As(err, &serviceError):
		return err
	}
	// A database that does not answer is not a signed-out session: apps keep their session and treat 503 as unreachable.
	if err != nil {
		log.Printf("session lookup: %v", err)
		return service.ErrDatabaseUnavailable
	}
	c.Locals(ctxKey{}, currentSession)
	return c.Next()
}

func requireAdmin(c fiber.Ctx) error {
	if getSession(c).Role != apitypes.RoleAdmin {
		return service.ErrAdminOnly
	}
	return c.Next()
}

func (s *Server) limitRate(c fiber.Ctx) error {
	if !s.limiter.Allow(s.getClientIP(c)) {
		return createAPIError(fiber.StatusTooManyRequests, "too many requests")
	}
	return c.Next()
}

// limitRequestBody runs once a request's headers are in, before its body is read: only a signed-in caller's rotation
// or vault deletion may send up to maxRotationBody, so nobody else makes the server buffer that much.
func (s *Server) limitRequestBody(header *fasthttp.RequestHeader) fasthttp.RequestConfig {
	path, _, _ := bytes.Cut(header.RequestURI(), []byte("?"))
	if !header.IsPost() || !rotationPath.Match(path) {
		return fasthttp.RequestConfig{}
	}
	token, ok := bytes.CutPrefix(header.Peek(fiber.HeaderAuthorization), []byte("Bearer "))
	if !ok || len(token) == 0 {
		return fasthttp.RequestConfig{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.service.Authenticate(ctx, string(token)); err != nil {
		return fasthttp.RequestConfig{}
	}
	return fasthttp.RequestConfig{MaxRequestBodySize: maxRotationBody}
}

// With TrustProxy, the last X-Forwarded-For entry is the one appended by our reverse proxy. An entry that is not an IP
// address is not the proxy's, so the peer's address counts instead.
func (s *Server) getClientIP(c fiber.Ctx) string {
	if s.config.TrustProxy {
		forwardedFor := c.Get("X-Forwarded-For")
		lastEntry := strings.TrimSpace(forwardedFor[strings.LastIndexByte(forwardedFor, ',')+1:])
		if address, err := netip.ParseAddr(lastEntry); err == nil {
			return address.Unmap().WithZone("").String()
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
