package localsocket

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func createTestPipePath(t *testing.T) string {
	t.Helper()
	return pipePrefix + "hisar-test-" + strconv.Itoa(os.Getpid()) + "-" + strings.ReplaceAll(t.Name(), "/", "-")
}

func TestDefaultPathIsAPipeNamedForTheUser(t *testing.T) {
	sid, err := getCurrentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	if path := GetDefaultPath(`C:\Users\alice`, "cli"); path != pipePrefix+"hisar-cli-"+sid.String() {
		t.Errorf("GetDefaultPath = %q", path)
	}
}

func TestPipeRoundTripNamesTheClient(t *testing.T) {
	path := createTestPipePath(t)
	listener, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			close(accepted)
			return
		}
		accepted <- connection
	}()
	client, err := Dial(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server := <-accepted
	if server == nil {
		t.Fatal("Accept failed")
	}
	defer server.Close()

	peer, err := SystemPeerResolver{}.ResolvePeer(server)
	if err != nil {
		t.Fatal(err)
	}
	executablePath, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if peer.PID != os.Getpid() || !strings.EqualFold(filepath.Clean(peer.Path), filepath.Clean(executablePath)) {
		t.Errorf("peer = %+v, want PID %d at %s", peer, os.Getpid(), executablePath)
	}
	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	message := make([]byte, 4)
	if _, err := io.ReadFull(server, message); err != nil || string(message) != "ping" {
		t.Fatalf("read %q, %v", message, err)
	}
}

func TestPipeRefusesASecondServer(t *testing.T) {
	path := createTestPipePath(t)
	listener, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if second, err := Listen(path); err == nil {
		second.Close()
		t.Fatal("a second Listen on the same pipe succeeded")
	}
}

func TestSameUserCheckRefusesTheSystemProcess(t *testing.T) {
	// PID 4 is the System process: it runs as SYSTEM, and a user cannot read its token.
	if err := checkSameUser(4); err == nil {
		t.Fatal("checkSameUser accepted the System process")
	}
	if err := checkSameUser(os.Getpid()); err != nil {
		t.Fatalf("checkSameUser refused this process: %v", err)
	}
}

func TestGetParentPIDOnWindows(t *testing.T) {
	parentPID, err := GetParentPID(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if parentPID != os.Getppid() {
		t.Errorf("GetParentPID = %d, want %d", parentPID, os.Getppid())
	}
}
