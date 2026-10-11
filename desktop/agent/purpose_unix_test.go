//go:build unix

package agent

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestReadSmallRegularFileDoesNotWaitForANamedPipe(t *testing.T) {
	pipePath := filepath.Join(t.TempDir(), "commit-message")
	if err := syscall.Mkfifo(pipePath, 0o600); err != nil {
		t.Fatal(err)
	}
	resultChannel := make(chan error, 1)
	go func() {
		_, err := ReadSmallRegularFile(pipePath, 64<<10)
		resultChannel <- err
	}()
	select {
	case err := <-resultChannel:
		if err == nil {
			t.Error("a named pipe was read as a regular file")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("reading a named pipe without a writer blocked")
	}
}
