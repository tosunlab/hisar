//go:build darwin || linux

package sshagent

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestSystemPeerResolver(t *testing.T) {
	listener, err := ListenUnixSocket(filepath.Join(createShortTempDirectory(t), "agent.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	clientConnection, err := net.Dial("unix", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer clientConnection.Close()
	serverConnection, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer serverConnection.Close()
	peer, err := SystemPeerResolver{}.ResolvePeer(serverConnection)
	if err != nil {
		t.Fatal(err)
	}
	if peer.PID != os.Getpid() {
		t.Errorf("pid = %d, want %d", peer.PID, os.Getpid())
	}
	executablePath, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if resolvePath(t, peer.Path) != resolvePath(t, executablePath) {
		t.Errorf("path = %q, want %q", peer.Path, executablePath)
	}
	if peer.Name != filepath.Base(peer.Path) {
		t.Errorf("name = %q", peer.Name)
	}
}

func TestSystemPeerResolverRejectsNonSocket(t *testing.T) {
	clientConnection, serverConnection := net.Pipe()
	defer clientConnection.Close()
	defer serverConnection.Close()
	if _, err := (SystemPeerResolver{}).ResolvePeer(serverConnection); err == nil {
		t.Error("resolved a peer for a pipe")
	}
}

func resolvePath(t *testing.T, path string) string {
	t.Helper()
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	resolvedPath, err := filepath.EvalSymlinks(absolutePath)
	if err != nil {
		t.Fatal(err)
	}
	return resolvedPath
}
