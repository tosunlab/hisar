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

	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/localsocket"
)

type Server struct {
	handler         browserprotocol.Handler
	peerResolver    localsocket.PeerResolver
	allowedPeerPath string
	mutex           sync.Mutex
	listener        net.Listener
}

// NewServer accepts only peers running allowedPeerPath when that file exists, i.e. in the packaged app.
func NewServer(handler browserprotocol.Handler, peerResolver localsocket.PeerResolver, allowedPeerPath string) *Server {
	return &Server{handler: handler, peerResolver: peerResolver, allowedPeerPath: allowedPeerPath}
}

func (s *Server) Serve(ctx context.Context, socketPath string) error {
	listener, err := localsocket.Listen(socketPath)
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
	firstMessage, err := browserprotocol.ReadMessage(connection)
	if err != nil {
		return
	}
	var connect browserprotocol.ConnectMessage
	if json.Unmarshal(firstMessage, &connect) != nil || connect.Type != "connect" || connect.ExtensionOrigin != browserprotocol.AllowedOrigin {
		log.Printf("browser: refused extension origin %q", connect.ExtensionOrigin)
		return
	}
	for {
		message, err := browserprotocol.ReadMessage(connection)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Printf("browser: %v", err)
			}
			return
		}
		response := s.answerMessage(ctx, message)
		encoded, err := json.Marshal(response)
		if err != nil {
			encoded, _ = json.Marshal(browserprotocol.Response{ID: response.ID, Error: browserprotocol.NewError(browserprotocol.ErrorInternal, err.Error())})
		}
		if err := browserprotocol.WriteMessage(connection, encoded); err != nil {
			return
		}
	}
}

func (s *Server) answerMessage(ctx context.Context, message []byte) browserprotocol.Response {
	return browserprotocol.Answer(ctx, s.handler, message)
}
