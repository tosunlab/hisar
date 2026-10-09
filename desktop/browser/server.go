package browser

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"sync"

	"github.com/tryhisar/hisar/core/sshagent"
)

// Handler answers one request; returning a *ResponseError sends its code to the extension.
type Handler interface {
	HandleRequest(ctx context.Context, request *Request) (any, error)
}

type Server struct {
	handler         Handler
	peerResolver    sshagent.PeerResolver
	allowedPeerPath string
	mutex           sync.Mutex
	listener        net.Listener
}

// CreateServer accepts only peers running allowedPeerPath when that file exists, i.e. in the packaged app.
func CreateServer(handler Handler, peerResolver sshagent.PeerResolver, allowedPeerPath string) *Server {
	return &Server{handler: handler, peerResolver: peerResolver, allowedPeerPath: allowedPeerPath}
}

func (s *Server) Serve(ctx context.Context, socketPath string) error {
	listener, err := sshagent.ListenUnixSocket(socketPath)
	if err != nil {
		return err
	}
	s.mutex.Lock()
	s.listener = listener
	s.mutex.Unlock()
	go func() {
		<-ctx.Done()
		listener.Close()
	}()
	for {
		connection, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go s.serveConnection(ctx, connection)
	}
}

func (s *Server) isPeerAllowed(connection net.Conn) bool {
	if s.allowedPeerPath == "" || s.peerResolver == nil {
		return true
	}
	if _, err := os.Stat(s.allowedPeerPath); err != nil {
		return true
	}
	peer, err := s.peerResolver.ResolvePeer(connection)
	return err == nil && peer.Path == s.allowedPeerPath
}

func (s *Server) serveConnection(ctx context.Context, connection net.Conn) {
	defer connection.Close()
	if !s.isPeerAllowed(connection) {
		log.Printf("browser: refused a connection from an unknown process")
		return
	}
	firstMessage, err := ReadMessage(connection)
	if err != nil {
		return
	}
	var connect connectMessage
	if json.Unmarshal(firstMessage, &connect) != nil || connect.Type != "connect" || connect.ExtensionOrigin != AllowedOrigin {
		log.Printf("browser: refused extension origin %q", connect.ExtensionOrigin)
		return
	}
	for {
		message, err := ReadMessage(connection)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Printf("browser: %v", err)
			}
			return
		}
		response := s.answerMessage(ctx, message)
		encoded, err := json.Marshal(response)
		if err != nil {
			encoded, _ = json.Marshal(Response{ID: response.ID, Error: CreateError(ErrorInternal, err.Error())})
		}
		if err := WriteMessage(connection, encoded); err != nil {
			return
		}
	}
}

func (s *Server) answerMessage(ctx context.Context, message []byte) Response {
	var request Request
	if err := json.Unmarshal(message, &request); err != nil || request.ID == "" || request.Type == "" {
		return Response{ID: request.ID, Error: CreateError(ErrorInvalidRequest, "the request is not valid")}
	}
	result, err := s.handler.HandleRequest(ctx, &request)
	if err != nil {
		var responseError *ResponseError
		if !errors.As(err, &responseError) {
			responseError = CreateError(ErrorInternal, err.Error())
		}
		return Response{ID: request.ID, Error: responseError}
	}
	if result == nil {
		result = struct{}{}
	}
	return Response{ID: request.ID, OK: true, Result: result}
}
