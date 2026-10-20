package browser

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/browserprotocol"
)

type cancellablePasskeyHandler struct {
	started   chan struct{}
	cancelled chan struct{}
}

func (handler cancellablePasskeyHandler) HandleRequest(ctx context.Context, request *browserprotocol.Request) (any, error) {
	if request.Type == "passkeys.cancel" {
		close(handler.cancelled)
		return nil, nil
	}
	close(handler.started)
	select {
	case <-handler.cancelled:
		return nil, browserprotocol.NewError(browserprotocol.ErrorInvalidRequest, "ceremony cancelled")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestNativeTransportCancelsWhileVerificationIsPending(t *testing.T) {
	handler := cancellablePasskeyHandler{started: make(chan struct{}), cancelled: make(chan struct{})}
	path := startTestServer(t, handler, nil, "")
	connection, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	connection.SetDeadline(time.Now().Add(5 * time.Second))
	for _, frame := range []string{`{"type":"connect","extensionOrigin":"` + browserprotocol.AllowedOrigin + `"}`, `{"id":"create","type":"passkeys.create"}`} {
		if err := browserprotocol.WriteMessage(connection, []byte(frame)); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-handler.started:
	case <-time.After(5 * time.Second):
		t.Fatal("ceremony did not start")
	}
	if err := browserprotocol.WriteMessage(connection, []byte(`{"id":"cancel","type":"passkeys.cancel"}`)); err != nil {
		t.Fatal(err)
	}
	responses := make(map[string]browserprotocol.Response)
	for range 2 {
		message, err := browserprotocol.ReadMessage(connection)
		if err != nil {
			t.Fatalf("cancellation blocked behind verification: %v", err)
		}
		var response browserprotocol.Response
		if err := json.Unmarshal(message, &response); err != nil {
			t.Fatal(err)
		}
		responses[response.ID] = response
	}
	if !responses["cancel"].OK || responses["create"].OK {
		t.Fatalf("unexpected ceremony responses: %+v", responses)
	}
}
