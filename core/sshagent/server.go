package sshagent

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/tryhisar/hisar/core/localsocket"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	stopClosingListener := context.AfterFunc(ctx, func() { listener.Close() })
	defer stopClosingListener()
	var waitGroup sync.WaitGroup
	defer waitGroup.Wait()
	for {
		connection, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		waitGroup.Go(func() { s.serveConnection(ctx, connection) })
	}
}

func (s *Server) serveConnection(ctx context.Context, connection net.Conn) {
	connectionContext, cancel := context.WithCancel(ctx)
	defer cancel()
	stopClosingConnection := context.AfterFunc(connectionContext, func() { connection.Close() })
	defer stopClosingConnection()
	defer connection.Close()
	peer := localsocket.PeerInfo{Name: localsocket.UnknownPeerName}
	if s.peerResolver != nil {
		if resolvedPeer, err := s.peerResolver.ResolvePeer(connection); err == nil {
			peer = resolvedPeer
		}
	}
	if peer.Name == "" {
		peer.Name = localsocket.UnknownPeerName
	}
	reader := localsocket.WatchConnectionClose(connection, cancel)
	agent.ServeAgent(s.CreateAgent(connectionContext, peer), struct {
		io.Reader
		io.Writer
	}{reader, connection})
}

func findKnownHostName(paths []string, hostKey ssh.PublicKey) string {
	hostKeyBlob := hostKey.Marshal()
	for _, path := range paths {
		if hostName := findKnownHostNameInFile(path, hostKeyBlob); hostName != "" {
			return hostName
		}
	}
	return ""
}

// splitKnownHostName turns a known_hosts name like "[git.example.com]:2222" into its host and port; 0 is the default port.
func splitKnownHostName(name string) (string, int) {
	bracketed, isBracketed := strings.CutPrefix(name, "[")
	if !isBracketed {
		return name, 0
	}
	host, portText, isFound := strings.Cut(bracketed, "]:")
	port, err := strconv.Atoi(portText)
	if !isFound || err != nil || port <= 0 || port > 65535 {
		return name, 0
	}
	if port == 22 {
		port = 0
	}
	return host, port
}

func findKnownHostNameInFile(path string, hostKeyBlob []byte) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		marker, hosts, publicKey, _, _, err := ssh.ParseKnownHosts(scanner.Bytes())
		if err != nil || marker != "" || !bytes.Equal(publicKey.Marshal(), hostKeyBlob) {
			continue
		}
		for _, host := range hosts {
			if !strings.HasPrefix(host, "|") && !strings.ContainsAny(host, "*?!") {
				return host
			}
		}
	}
	return ""
}
