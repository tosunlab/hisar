package localsocket

import (
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestListenUnixSocketPermissions(t *testing.T) {
	directory := filepath.Join(createShortTempDirectory(t), "nested", "hisar")
	socketPath := filepath.Join(directory, "agent.sock")
	listener, err := ListenUnixSocket(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	socketInfo, err := os.Lstat(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	if socketInfo.Mode()&fs.ModeSocket == 0 {
		t.Errorf("%s is not a socket", socketPath)
	}
	if mode := socketInfo.Mode().Perm(); mode != 0o600 {
		t.Errorf("socket mode = %o, want 600", mode)
	}
	directoryInfo, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	if mode := directoryInfo.Mode().Perm(); mode != 0o700 {
		t.Errorf("directory mode = %o, want 700", mode)
	}
}

func TestListenUnixSocketTightensDirectory(t *testing.T) {
	directory := filepath.Join(createShortTempDirectory(t), "open")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	listener, err := ListenUnixSocket(filepath.Join(directory, "agent.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	directoryInfo, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	if mode := directoryInfo.Mode().Perm(); mode != 0o700 {
		t.Errorf("directory mode = %o, want 700", mode)
	}
}

func TestListenUnixSocketReplacesStaleSocket(t *testing.T) {
	socketPath := filepath.Join(createShortTempDirectory(t), "agent.sock")
	staleListener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	staleListener.SetUnlinkOnClose(false)
	staleListener.Close()
	if _, err := os.Lstat(socketPath); err != nil {
		t.Fatalf("stale socket missing: %v", err)
	}
	listener, err := ListenUnixSocket(socketPath)
	if err != nil {
		t.Fatalf("stale socket not replaced: %v", err)
	}
	defer listener.Close()
	connection, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	connection.Close()
}

func TestListenUnixSocketRefusesLiveSocket(t *testing.T) {
	socketPath := filepath.Join(createShortTempDirectory(t), "agent.sock")
	listener, err := ListenUnixSocket(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			connection.Close()
		}
	}()
	if secondListener, err := ListenUnixSocket(socketPath); err == nil {
		secondListener.Close()
		t.Fatal("second listener replaced a live agent socket")
	}
}

func TestListenUnixSocketRefusesRegularFile(t *testing.T) {
	socketPath := filepath.Join(createShortTempDirectory(t), "agent.sock")
	if err := os.WriteFile(socketPath, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if listener, err := ListenUnixSocket(socketPath); err == nil {
		listener.Close()
		t.Fatal("regular file replaced by socket")
	}
	if content, err := os.ReadFile(socketPath); err != nil || string(content) != "data" {
		t.Error("regular file was modified")
	}
}
