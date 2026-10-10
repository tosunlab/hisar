package commandline

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"sync"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/sshagent"
)

// Handler answers approved requests; returning a *cliprotocol.ResponseError sends its code to the CLI.
type Handler interface {
	GetHello() cliprotocol.HelloResult
	HandleRequest(ctx context.Context, request *cliprotocol.Request) (any, error)
}

type ServerConfig struct {
	Handler      Handler
	Approver     Approver
	PeerResolver PeerResolver
	Approvals    *SessionApprovals
	// IsEnabled follows the "Let the command-line tool use Hisar" switch.
	IsEnabled func() bool
	UID       int
}

type Server struct {
	config ServerConfig
}

type connectionPeer struct {
	peer    Peer
	session TerminalSession
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
	for {
		message, err := cliprotocol.ReadMessage(connection)
		if err != nil {
			if !errors.Is(err, io.EOF) && connectionContext.Err() == nil {
				log.Printf("commandline: %v", err)
			}
			return
		}
		response := s.answerMessage(connectionContext, client, message)
		if err := cliprotocol.WriteJSON(connection, response); err != nil {
			return
		}
	}
}

func (s *Server) answerMessage(ctx context.Context, client connectionPeer, message []byte) cliprotocol.Response {
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

func (s *Server) answerRequest(ctx context.Context, client connectionPeer, request *cliprotocol.Request) (any, error) {
	if !s.config.IsEnabled() {
		return nil, cliprotocol.CreateError(cliprotocol.ErrorIntegrationDisabled, "the Hisar app does not let the command-line tool use it")
	}
	hello := s.config.Handler.GetHello()
	if request.Type == cliprotocol.TypeHello {
		hello.ProtocolVersion = cliprotocol.ProtocolVersion
		hello.IsApproved = hello.State == cliprotocol.StateUnlocked && s.config.Approvals.IsApproved(client.session)
		return hello, nil
	}
	if hello.State == cliprotocol.StateSignedOut {
		return nil, cliprotocol.CreateError(cliprotocol.ErrorSignedOut, "the Hisar app is not signed in; sign in there, or run hisar --standalone")
	}
	if err := s.config.Approvals.Require(client.session, func() error { return s.askApproval(ctx, client) }); err != nil {
		if errors.Is(err, ErrDenied) {
			return nil, cliprotocol.CreateError(cliprotocol.ErrorDenied, ErrDenied.Error())
		}
		return nil, err
	}
	return s.config.Handler.HandleRequest(ctx, request)
}

func (s *Server) askApproval(ctx context.Context, client connectionPeer) error {
	if s.config.Approver == nil {
		return ErrDenied
	}
	request := ApprovalRequest{ClientPID: client.peer.PID, TerminalName: client.session.TerminalName}
	request.ClientPath, _ = s.config.PeerResolver.GetProcessPath(client.peer.PID)
	request.ParentPID, _ = s.config.PeerResolver.GetParentPID(client.peer.PID)
	return s.config.Approver.ApproveCommandLine(ctx, request)
}
