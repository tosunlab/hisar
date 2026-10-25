package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/tryhisar/hisar/core/publicnet"
)

func TestWebPushClientRefusesLocalAddresses(t *testing.T) {
	var reached atomic.Bool
	local := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached.Store(true) }))
	t.Cleanup(local.Close)
	response, err := newWebPushClient().Post(local.URL+"/up", "application/octet-stream", nil)
	if err == nil {
		response.Body.Close()
	}
	if !errors.Is(err, publicnet.ErrNotPublic) || reached.Load() {
		t.Fatalf("the Web Push client reached a loopback endpoint: %v", err)
	}
}
