package sshagent

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

const UnknownPeerName = "unknown"

type PeerInfo struct {
	PID  int
	Path string
	Name string
}

type PeerResolver interface {
	ResolvePeer(connection net.Conn) (PeerInfo, error)
}

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
	peer := PeerInfo{Name: UnknownPeerName}
	if s.peerResolver != nil {
		if resolvedPeer, err := s.peerResolver.ResolvePeer(connection); err == nil {
			peer = resolvedPeer
		}
	}
	if peer.Name == "" {
		peer.Name = UnknownPeerName
	}
	agent.ServeAgent(s.CreateAgent(connectionContext, peer), connection)
}

func ListenUnixSocket(path string) (net.Listener, error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return nil, err
	}
	if err := removeStaleSocket(path); err != nil {
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}

func removeStaleSocket(path string) error {
	fileInfo, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if fileInfo.Mode()&fs.ModeSocket == 0 {
		return fmt.Errorf("sshagent: %s exists and is not a socket", path)
	}
	if connection, err := net.DialTimeout("unix", path, time.Second); err == nil {
		connection.Close()
		return fmt.Errorf("sshagent: another agent is listening on %s", path)
	}
	return os.Remove(path)
}

func findKnownHostName(allPath []string, hostKey ssh.PublicKey) string {
	hostKeyBlob := hostKey.Marshal()
	for _, path := range allPath {
		if hostName := findKnownHostNameInFile(path, hostKeyBlob); hostName != "" {
			return hostName
		}
	}
	return ""
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
		marker, allHost, publicKey, _, _, err := ssh.ParseKnownHosts(scanner.Bytes())
		if err != nil || marker != "" || !bytes.Equal(publicKey.Marshal(), hostKeyBlob) {
			continue
		}
		for _, host := range allHost {
			if !strings.HasPrefix(host, "|") && !strings.ContainsAny(host, "*?!") {
				return host
			}
		}
	}
	return ""
}
