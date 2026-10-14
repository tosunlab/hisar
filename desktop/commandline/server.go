package commandline

import (
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

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/secretreference"
	"github.com/tryhisar/hisar/core/sshagent"
)

// Handler answers approved requests; returning a *cliprotocol.ResponseError sends its code to the CLI.
type Handler interface {
	GetHello() cliprotocol.HelloResult
	HandleRequest(ctx context.Context, request *cliprotocol.Request) (any, error)
}

// SecretDescriber names the secrets of a secrets.resolve request for the approval window.
type SecretDescriber interface {
	DescribeAllSecret(allReference []string) []string
}

// ShareLinkDescriber names the item and fields of a shareLinks.create request for the approval window.
type ShareLinkDescriber interface {
	DescribeShareLink(request *cliprotocol.Request) (title string, allFieldLabel []string)
}

type ServerConfig struct {
	Handler  Handler
	Approver Approver
	// AgentApprover asks about the requests of hisar mcp; nil denies them.
	AgentApprover AgentApprover
	PeerResolver  PeerResolver
	Approvals     *SessionApprovals
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

// connectionPeer is one connection; agent is set once it sent agent.hello.
type connectionPeer struct {
	peer         Peer
	session      TerminalSession
	connectionID uint64
	agent        *AgentIdentity
}

func CreateServer(config ServerConfig) *Server {
	if config.PeerResolver == nil {
		config.PeerResolver = SystemPeerResolver{}
	}
	if config.Approvals == nil {
		config.Approvals = CreateSessionApprovals(0, nil)
	}
	if config.IsEnabled == nil {
		config.IsEnabled = func() bool { return true }
	}
	if config.UID == 0 {
		config.UID = os.Getuid()
	}
	return &Server{config: config}
}

func (s *Server) ClearAllApproval() {
	s.config.Approvals.ClearAll()
}

// Serve listens until ctx ends; closing the listener removes the socket.
func (s *Server) Serve(ctx context.Context, socketPath string) error {
	listener, err := sshagent.ListenUnixSocket(socketPath)
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

func (s *Server) identifyPeer(connection net.Conn) (connectionPeer, bool) {
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
	return connectionPeer{peer: peer, session: session}, true
}

func (s *Server) ServeConnection(ctx context.Context, connection net.Conn) {
	connectionContext, cancel := context.WithCancel(ctx)
	defer cancel()
	stopClosingConnection := context.AfterFunc(connectionContext, func() { connection.Close() })
	defer stopClosingConnection()
	defer connection.Close()
	client, isAllowed := s.identifyPeer(connection)
	if !isAllowed {
		return
	}
	client.connectionID = s.lastConnectionID.Add(1)
	defer s.config.Approvals.ForgetAgentConnection(client.connectionID)
	reader := sshagent.WatchConnectionClose(connection, cancel)
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
		return buildErrorResponse(request.ID, cliprotocol.CreateError(cliprotocol.ErrorInvalidRequest, "the request is not valid"))
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
		responseError = cliprotocol.CreateError(cliprotocol.ErrorInternal, err.Error())
	}
	return cliprotocol.Response{ID: requestID, Error: responseError}
}

func (s *Server) answerRequest(ctx context.Context, client *connectionPeer, request *cliprotocol.Request) (any, error) {
	if !s.config.IsEnabled() {
		return nil, cliprotocol.CreateError(cliprotocol.ErrorIntegrationDisabled, "the Hisar app does not let the command-line tool use it")
	}
	hello := s.config.Handler.GetHello()
	if request.Type == cliprotocol.TypeHello {
		hello.ProtocolVersion = cliprotocol.ProtocolVersion
		if client.agent != nil {
			hello.IsApproved = hello.State == cliprotocol.StateUnlocked && s.config.Approvals.IsAgentConnectionApproved(client.connectionID)
		} else {
			hello.IsApproved = hello.State == cliprotocol.StateUnlocked && s.config.Approvals.IsApproved(client.session)
		}
		return hello, nil
	}
	if s.config.CheckUpdateRequired != nil {
		if err := s.config.CheckUpdateRequired(request); err != nil {
			return nil, err
		}
	}
	if request.Type == cliprotocol.TypeAgentHello {
		return s.startAgentConnection(client, request)
	}
	if (client.agent != nil) != cliprotocol.IsAgentRequest(request.Type) {
		return nil, cliprotocol.CreateError(cliprotocol.ErrorInvalidRequest, "agent requests and other requests cannot share a connection")
	}
	if hello.State == cliprotocol.StateSignedOut {
		if client.agent != nil {
			return nil, cliprotocol.CreateError(cliprotocol.ErrorSignedOut, "the Hisar app is not signed in; sign in there")
		}
		return nil, cliprotocol.CreateError(cliprotocol.ErrorSignedOut, "the Hisar app is not signed in; sign in there, or run hisar --standalone")
	}
	if client.agent != nil {
		return s.answerAgentRequest(ctx, client, request)
	}
	if request.Type == cliprotocol.TypeResolveSecret && (len(request.References) == 0 || len(request.References) > cliprotocol.MaxReferenceCount) {
		return nil, cliprotocol.CreateError(cliprotocol.ErrorInvalidRequest, fmt.Sprintf("send between 1 and %d secret references", cliprotocol.MaxReferenceCount))
	}
	if request.Type == cliprotocol.TypeCreateShareLink && request.ShareLink == nil {
		return nil, cliprotocol.CreateError(cliprotocol.ErrorInvalidRequest, "the share link is missing")
	}
	if request.Type == cliprotocol.TypeExport && request.Export == nil {
		return nil, cliprotocol.CreateError(cliprotocol.ErrorInvalidRequest, "the export is missing")
	}
	require := s.config.Approvals.Require
	if request.Type == cliprotocol.TypeCreateShareLink || request.Type == cliprotocol.TypeExport {
		// A share link hands secrets to people outside Hisar and an export writes every secret to a file, so both ask
		// even in an approved session.
		require = s.config.Approvals.RequireAlways
	}
	if err := require(client.session, func() error { return s.askApproval(ctx, client, request) }); err != nil {
		if errors.Is(err, ErrDenied) {
			return nil, cliprotocol.CreateError(cliprotocol.ErrorDenied, err.Error())
		}
		return nil, err
	}
	return s.config.Handler.HandleRequest(ctx, request)
}

func (s *Server) askApproval(ctx context.Context, client *connectionPeer, request *cliprotocol.Request) error {
	if s.config.Approver == nil {
		return ErrDenied
	}
	approvalRequest := ApprovalRequest{ClientPID: client.peer.PID, TerminalName: client.session.TerminalName}
	approvalRequest.ClientPath, _ = s.config.PeerResolver.GetProcessPath(client.peer.PID)
	approvalRequest.ParentPID, _ = s.config.PeerResolver.GetParentPID(client.peer.PID)
	if request.Type == cliprotocol.TypeResolveSecret {
		approvalRequest.IsSecretRequest = true
		approvalRequest.CommandLine = request.CommandLine
		approvalRequest.AllSecretName = s.describeAllSecret(request.References)
	}
	if request.Type == cliprotocol.TypeCreateShareLink {
		approvalRequest.ShareLink = s.describeShareLink(request)
	}
	if request.Type == cliprotocol.TypeExport {
		approvalRequest.Export = &ExportDescription{IsPlain: request.Export.Format == cliprotocol.ExportFormatPlain, Path: request.Export.Path}
	}
	return s.config.Approver.ApproveCommandLine(ctx, approvalRequest)
}

func (s *Server) describeAllSecret(allReference []string) []string {
	if describer, isDescriber := s.config.Handler.(SecretDescriber); isDescriber {
		return describer.DescribeAllSecret(allReference)
	}
	allName := make([]string, 0, len(allReference))
	for _, text := range allReference {
		if reference, err := secretreference.Parse(text); err == nil {
			allName = append(allName, reference.Describe())
		} else {
			allName = append(allName, text)
		}
	}
	return allName
}

func (s *Server) describeShareLink(request *cliprotocol.Request) *ShareLinkDescription {
	description := &ShareLinkDescription{Title: request.Query, AllFieldLabel: append([]string{}, request.ShareLink.Fields...), Detail: describeShareLinkLimit(request.ShareLink)}
	if request.ShareLink.Text != "" || request.Query == "" {
		description.Title, description.AllFieldLabel = request.ShareLink.Title, []string{"text"}
		if description.Title == "" {
			description.Title = "Shared text"
		}
	} else if describer, isDescriber := s.config.Handler.(ShareLinkDescriber); isDescriber {
		description.Title, description.AllFieldLabel = describer.DescribeShareLink(request)
	}
	if len(description.AllFieldLabel) == 0 {
		description.AllFieldLabel = []string{"username", "password"}
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
	allPart := []string{"Expires in " + expiry, views}
	if change.Passphrase != "" {
		allPart = append(allPart, "passphrase")
	}
	return strings.Join(allPart, " · ")
}
