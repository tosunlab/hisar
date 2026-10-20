package localsocket

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func waitForCancel(t *testing.T, ctx context.Context, why string) {
	t.Helper()
	select {
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
		t.Fatal(why)
	}
}

// TestWatchConnectionCloseSeesAHangUpBehindUnreadBytes: the user reads one request and waits; the peer's bytes after
// it stay unread, and its hang-up still cancels.
func TestWatchConnectionCloseSeesAHangUpBehindUnreadBytes(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	reader := WatchConnectionClose(server, cancel)
	if _, err := client.Write([]byte("request+more")); err != nil {
		t.Fatal(err)
	}
	request := make([]byte, len("request"))
	if _, err := io.ReadFull(reader, request); err != nil || string(request) != "request" {
		t.Fatalf("read %q, %v", request, err)
	}
	client.Close()
	waitForCancel(t, ctx, "the hang-up behind unread bytes did not cancel")
	if rest, err := io.ReadAll(reader); string(rest) != "+more" || err != nil {
		t.Errorf("the rest = %q, %v", rest, err)
	}
}

// TestWatchConnectionCloseCancelsAPeerThatSendsTooMuch: unread bytes are bounded; past the bound the peer is cut off.
func TestWatchConnectionCloseCancelsAPeerThatSendsTooMuch(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	reader := WatchConnectionClose(server, cancel)
	go client.Write(make([]byte, maximumReadAhead+1))
	waitForCancel(t, ctx, "a peer kept sending past the read-ahead bound")
	if _, err := io.ReadAll(reader); !errors.Is(err, errReadAheadFull) {
		t.Errorf("read error = %v, want errReadAheadFull", err)
	}
}
