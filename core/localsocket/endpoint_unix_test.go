//go:build !windows

package localsocket

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultPathIsUnderDotHisar(t *testing.T) {
	if path := GetDefaultPath("/home/alice", "cli"); path != "/home/alice/.hisar/cli.sock" {
		t.Errorf("GetDefaultPath = %q", path)
	}
}

func TestListenAndDialUseTheSocket(t *testing.T) {
	socketPath := filepath.Join(createShortTempDirectory(t), "hisar", "cli.sock")
	listener, err := Listen(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		if connection, err := listener.Accept(); err == nil {
			connection.Close()
		}
	}()
	connection, err := Dial(socketPath, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	connection.Close()
}
