package clisocket

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/localsocket"
	"github.com/tryhisar/hisar/core/secretreference"
)

// Handler answers approved requests; returning a *cliprotocol.ResponseError sends its code to the CLI.
type Handler interface {
	GetHello() cliprotocol.HelloResult
	HandleRequest(ctx context.Context, request *cliprotocol.Request) (any, error)
}

// SecretDescriber names the secrets of a secrets.resolve request for the approval window.
type SecretDescriber interface {
	DescribeSecrets(references []string) []string
}

// ShareLinkDescriber names the item and fields of a shareLinks.create or launcher.items.share request for the approval
// window.
type ShareLinkDescriber interface {
	DescribeShareLink(request *cliprotocol.Request) (title string, fieldLabels []string)
}

type ServerConfig struct {
	Handler  Handler
	Approver Approver
	// AgentApprover asks about the requests of hisar mcp; nil denies them.
	AgentApprover AgentApprover
	// LauncherApprover asks about launchers such as Raycast; nil denies them.
	LauncherApprover LauncherApprover
	// IdentifyApp names the verified app a caller runs in; it is asked about callers without a terminal and callers inside
	// an agent harness. nil, or false, approves a caller without a terminal for its own connection only, and refuses it as
	// a launcher.
	IdentifyApp  func(pid int) (AppIdentity, bool)
	PeerResolver PeerResolver
	Approvals    *SessionApprovals
	// AgentHosts remembers the apps agent sessions ran in; nil keeps them in memory until Hisar quits.
	AgentHosts AgentHosts
	// IsEnabled follows the "Let the command-line tool use Hisar" switch.
	IsEnabled func() bool
	// CheckUpdateRequired refuses every request but hello while Hisar or this hisar is below the server's minimum version.
	CheckUpdateRequired func(request *cliprotocol.Request) error
	UID                 int
}

type Server struct {
	config           ServerConfig
	lastConnectionID atomic.Uint64
}

// connectionKind is what a connection said it is; the request types of its namespace are the only ones it may send.
type connectionKind int

const (
	connectionCommandLine connectionKind = iota
	// connectionAgent: after agent.hello, agent.* only.
	connectionAgent
	// connectionLauncher: after launcher.hello, launcher.* only.
	connectionLauncher
)

// getConnectionKind returns the only kind of connection that may send requestType.
func getConnectionKind(requestType string) connectionKind {
	switch {
	case cliprotocol.IsAgentRequest(requestType):
		return connectionAgent
	case cliprotocol.IsLauncherRequest(requestType):
		return connectionLauncher
	}
	return connectionCommandLine
}

// connectionPeer is one connection. Its scope is fixed when it connects, but for an app that becomes an agent host
// later (applyAgentHost); its kind changes once, on agent.hello or launcher.hello, and agent is what an agent connection
// said about itself. agentSession is the harness the caller runs inside when its scope is an agent session, and
// agentHost the app that harness runs in; isAgentHost is a caller outside any harness in an app that hosted one;
// isChainUnknown is a caller whose process chain could not be read, so nobody knows whether a harness runs in it.
type connectionPeer struct {
	peer           Peer
	session        TerminalSession
	connectionID   uint64
	scope          approvalScope
	kind           connectionKind
	agent          AgentIdentity
	agentSession   agentharness.Session
	agentHost      AppIdentity
	isAgentHost    bool
	isChainUnknown bool
}

func NewServer(config ServerConfig) *Server {
	if config.PeerResolver == nil {
		config.PeerResolver = SystemPeerResolver{}
	}
	if config.Approvals == nil {
		config.Approvals = NewSessionApprovals(0, nil)
	}
	if config.IsEnabled == nil {
		config.IsEnabled = func() bool { return true }
	}
	if config.UID == 0 {
		config.UID = os.Getuid()
	}
	if config.AgentHosts == nil {
		config.AgentHosts = &memoryAgentHosts{apps: map[AppIdentity]bool{}}
	}
	return &Server{config: config}
}

func (s *Server) ClearApprovals() {
	s.config.Approvals.ClearAll()
}

// Serve listens until ctx ends; closing the listener removes the socket.
func (s *Server) Serve(ctx context.Context, socketPath string) error {
	listener, err := localsocket.ListenUnixSocket(socketPath)
	if err != nil {
		return err
	}
	stopClosingListener := context.AfterFunc(ctx, func() { listener.Close() })
	defer stopClosingListener()
	var waitGroup sync.WaitGroup
	defer waitGroup.Wait()
	for {
		connection, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		waitGroup.Go(func() { s.ServeConnection(ctx, connection) })
	}
}

// identifyPeer asks IdentifyApp only about callers without a terminal and callers inside an agent harness, so a terminal
// costs what it did before apps. An app that runs an agent session becomes an agent host.
func (s *Server) identifyPeer(connection net.Conn, connectionID uint64) (connectionPeer, bool) {
	peer, err := s.config.PeerResolver.ResolvePeer(connection)
	if err != nil {
		log.Printf("commandline: refused a connection whose peer is unknown: %v", err)
		return connectionPeer{}, false
	}
	if peer.UID != s.config.UID {
		log.Printf("commandline: refused a connection from user ID %d", peer.UID)
		return connectionPeer{}, false
	}
	session, err := s.config.PeerResolver.GetTerminalSession(peer.PID)
	if err != nil {
		log.Printf("commandline: refused a connection without a known session: %v", err)
		return connectionPeer{}, false
	}
	var agent agentLookup
	agent.session, agent.isInside, err = s.config.PeerResolver.GetAgentSession(peer.PID)
	if err != nil {
		log.Printf("commandline: could not check the process chain of PID %d: %v", peer.PID, err)
		agent = agentLookup{isUnknown: true}
	}
	var app AppIdentity
	if !agent.isUnknown && (agent.isInside || !session.hasTerminal()) && s.config.IdentifyApp != nil {
		if identity, isVerified := s.config.IdentifyApp(peer.PID); isVerified {
			app = identity
		}
	}
	if agent.isInside && app.Path != "" {
		s.RecordAgentHost(app)
	}
	client := connectionPeer{peer: peer, session: session, connectionID: connectionID, agentSession: agent.session, isChainUnknown: agent.isUnknown}
	if agent.isInside {
		client.agentHost = app
	}
	if app.Path != "" && s.IsAgentHost(app) {
		client.isAgentHost, app = !agent.isInside, AppIdentity{}
	}
	client.scope = getApprovalScope(agent, session, app, connectionID)
	return client, true
}

func (s *Server) ServeConnection(ctx context.Context, connection net.Conn) {
	connectionContext, cancel := context.WithCancel(ctx)
	defer cancel()
	stopClosingConnection := context.AfterFunc(connectionContext, func() { connection.Close() })
	defer stopClosingConnection()
	defer connection.Close()
	client, isAllowed := s.identifyPeer(connection, s.lastConnectionID.Add(1))
	if !isAllowed {
		return
	}
	defer s.config.Approvals.forgetConnection(client.connectionID)
	if client.isInAgentSession() {
		stopEndingWithSession := context.AfterFunc(s.joinAgentSession(ctx, &client), cancel)
		defer stopEndingWithSession()
		connectionContext = context.WithValue(connectionContext, agentSessionContextKey{}, client.agentSession)
	}
	reader := localsocket.WatchConnectionClose(connection, cancel)
	for {
		message, err := cliprotocol.ReadMessage(reader)
		if err != nil {
			if !errors.Is(err, io.EOF) && connectionContext.Err() == nil {
				log.Printf("commandline: %v", err)
			}
			return
		}
		response := s.answerMessage(connectionContext, &client, message)
		if err := cliprotocol.WriteJSON(connection, response); err != nil {
			return
		}
	}
}

func (s *Server) answerMessage(ctx context.Context, client *connectionPeer, message []byte) cliprotocol.Response {
	var request cliprotocol.Request
	if err := json.Unmarshal(message, &request); err != nil || request.ID == "" || request.Type == "" {
		return buildErrorResponse(request.ID, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, "the request is not valid"))
	}
	result, err := s.answerRequest(ctx, client, &request)
	if err != nil {
		return buildErrorResponse(request.ID, err)
	}
	if result == nil {
		result = struct{}{}
	}
	encodedResult, err := json.Marshal(result)
	if err != nil {
		return buildErrorResponse(request.ID, err)
	}
	return cliprotocol.Response{ID: request.ID, OK: true, Result: encodedResult}
}

func buildErrorResponse(requestID string, err error) cliprotocol.Response {
	var responseError *cliprotocol.ResponseError
	if !errors.As(err, &responseError) {
		responseError = cliprotocol.NewError(cliprotocol.ErrorInternal, err.Error())
	}
	return cliprotocol.Response{ID: requestID, Error: responseError}
}

func (s *Server) answerRequest(ctx context.Context, client *connectionPeer, request *cliprotocol.Request) (any, error) {
	if !s.config.IsEnabled() {
		return nil, cliprotocol.NewError(cliprotocol.ErrorIntegrationDisabled, "the Hisar app does not let the command-line tool use it")
	}
	s.applyAgentHost(client)
	hello := s.config.Handler.GetHello()
	if request.Type == cliprotocol.TypeHello {
		hello.ProtocolVersion = cliprotocol.ProtocolVersion
		hello.Features = cliprotocol.ListFeatures()
		hello.IsApproved = s.isApproved(client, hello)
		return hello, nil
	}
	if s.config.CheckUpdateRequired != nil {
		if err := s.config.CheckUpdateRequired(request); err != nil {
			return nil, err
		}
	}
	switch request.Type {
	case cliprotocol.TypeAgentHello:
		return s.startAgentConnection(client, request)
	case cliprotocol.TypeLauncherHello:
		return s.startLauncherConnection(client, hello)
	}
	if getConnectionKind(request.Type) != client.kind {
		return nil, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, "launcher, agent and command-line requests cannot share a connection")
	}
	switch client.kind {
	case connectionAgent:
		if hello.State == cliprotocol.StateSignedOut {
			return nil, cliprotocol.NewError(cliprotocol.ErrorSignedOut, "the Hisar app is not signed in; sign in there")
		}
		return s.answerAgentRequest(ctx, client, request)
	case connectionLauncher:
		return s.answerLauncherRequest(ctx, client, hello, request)
	}
	if client.isChainUnknown {
		return nil, refuseUnknownChain()
	}
	if hello.State == cliprotocol.StateSignedOut {
		return nil, cliprotocol.NewError(cliprotocol.ErrorSignedOut, "the Hisar app is not signed in; sign in there, or run hisar --standalone")
	}
	if client.isInAgentSession() {
		return s.answerAgentSessionRequest(ctx, client, request)
	}
	if request.Type == cliprotocol.TypeResolveSecret && (len(request.References) == 0 || len(request.References) > cliprotocol.MaxReferenceCount) {
		return nil, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, fmt.Sprintf("send between 1 and %d secret references", cliprotocol.MaxReferenceCount))
	}
	if request.Type == cliprotocol.TypeCreateShareLink && request.ShareLink == nil {
		return nil, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, "the share link is missing")
	}
	if request.Type == cliprotocol.TypeExport && request.Export == nil {
		return nil, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, "the export is missing")
	}
	approvals := s.config.Approvals
	require := approvals.require
	if request.Type == cliprotocol.TypeCreateShareLink || request.Type == cliprotocol.TypeExport {
		// A share link hands secrets to people outside Hisar and an export writes every secret to a file, so both ask
		// even in an approved session.
		require = approvals.requireAlways
	}
	if err := require(&approvals.commandLine, client.scope, func() error { return s.askApproval(ctx, client, request) }); err != nil {
		return nil, convertDenial(err)
	}
	return s.config.Handler.HandleRequest(ctx, request)
}

// refuseUnknownChain answers an ordinary hisar command or a launcher whose process chain could not be read: an agent
// harness could run in it, so neither the command line's rules nor a launcher's apply. Reading it again usually works.
func refuseUnknownChain() error {
	return cliprotocol.NewError(cliprotocol.ErrorDenied, "Hisar couldn't check which program is asking. Try again.")
}

// applyAgentHost gives a connection whose app became an agent host after it connected what a new connection from that
// app gets: no app scope, so it is approved on its own, and no launcher.
func (s *Server) applyAgentHost(client *connectionPeer) {
	if client.scope.app.Path == "" || !s.IsAgentHost(client.scope.app) {
		return
	}
	client.isAgentHost, client.scope = true, approvalScope{connectionID: client.connectionID}
}

// isApproved is what hello and launcher.hello report: Hisar is unlocked, and the memory of the connection's kind holds an
// approval for it. Inside an agent harness, that is the agent session's approval.
func (s *Server) isApproved(client *connectionPeer, hello cliprotocol.HelloResult) bool {
	if hello.State != cliprotocol.StateUnlocked {
		return false
	}
	approvals := s.config.Approvals
	switch {
	case client.kind == connectionLauncher:
		return approvals.isApproved(&approvals.launchers, client.scope)
	case client.kind == connectionAgent || client.isInAgentSession():
		return approvals.isAgentApproved(client.getAgentScope())
	}
	return approvals.isApproved(&approvals.commandLine, client.scope)
}

func (s *Server) askApproval(ctx context.Context, client *connectionPeer, request *cliprotocol.Request) error {
	if s.config.Approver == nil {
		return ErrDenied
	}
	approvalRequest := ApprovalRequest{ClientPID: client.peer.PID, TerminalName: client.session.TerminalName, Coverage: client.scope.getCoverage()}
	approvalRequest.ClientPath, _ = s.config.PeerResolver.GetProcessPath(client.peer.PID)
	approvalRequest.ParentPID, _ = s.config.PeerResolver.GetParentPID(client.peer.PID)
	if request.Type == cliprotocol.TypeResolveSecret {
		approvalRequest.IsSecretRequest = true
		approvalRequest.CommandLine = request.CommandLine
		approvalRequest.SecretNames = s.describeSecrets(request.References)
	}
	if request.Type == cliprotocol.TypeCreateShareLink {
		approvalRequest.ShareLink = s.describeShareLink(request)
	}
	if request.Type == cliprotocol.TypeExport {
		approvalRequest.Export = &ExportDescription{IsPlain: request.Export.Format == cliprotocol.ExportFormatPlain, Path: request.Export.Path}
	}
	return s.config.Approver.ApproveCommandLine(ctx, approvalRequest)
}

func (s *Server) describeSecrets(references []string) []string {
	if describer, isDescriber := s.config.Handler.(SecretDescriber); isDescriber {
		return describer.DescribeSecrets(references)
	}
	names := make([]string, 0, len(references))
	for _, text := range references {
		if reference, err := secretreference.Parse(text); err == nil {
			names = append(names, reference.Describe())
		} else {
			names = append(names, text)
		}
	}
	return names
}

// describeShareLink names what the handler will share: the item when the request names one, by ID for a launcher or by
// query for hisar, even if it also carries text; the text otherwise.
func (s *Server) describeShareLink(request *cliprotocol.Request) *ShareLinkDescription {
	description := &ShareLinkDescription{Title: request.Query, FieldLabels: append([]string{}, request.ShareLink.Fields...), Detail: describeShareLinkLimit(request.ShareLink)}
	if request.Type == cliprotocol.TypeCreateShareLink && request.Query == "" {
		description.Title, description.FieldLabels = cmp.Or(request.ShareLink.Title, "Shared text"), []string{"text"}
	} else if describer, isDescriber := s.config.Handler.(ShareLinkDescriber); isDescriber {
		description.Title, description.FieldLabels = describer.DescribeShareLink(request)
	}
	if len(description.FieldLabels) == 0 {
		description.FieldLabels = []string{"username", "password"}
	}
	return description
}

func describeShareLinkLimit(change *cliprotocol.ShareLinkChange) string {
	expiry := fmt.Sprintf("%d seconds", change.ExpiresInSeconds)
	switch change.ExpiresInSeconds {
	case 3600:
		expiry = "1 hour"
	case 86400:
		expiry = "1 day"
	case 604800:
		expiry = "7 days"
	case 2592000:
		expiry = "30 days"
	}
	views := fmt.Sprintf("%d views", change.MaxViews)
	switch change.MaxViews {
	case 0:
		views = "unlimited views"
	case 1:
		views = "1 view"
	}
	parts := []string{"Expires in " + expiry, views}
	if change.Passphrase != "" {
		parts = append(parts, "passphrase")
	}
	return strings.Join(parts, " · ")
}
