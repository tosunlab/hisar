package app

import (
	"path/filepath"
	"testing"
	"time"
)

func expectStatusChange(t *testing.T, statusChanges chan string, want string) {
	t.Helper()
	select {
	case state := <-statusChanges:
		if state != want {
			t.Errorf("status listener saw %q, want %q", state, want)
		}
	case <-time.After(5 * time.Second):
		t.Errorf("status listener not called for %q", want)
	}
}

func TestStatusListenerMayReadTheStatus(t *testing.T) {
	service, _, _ := createTestService(t, filepath.Join(t.TempDir(), "desktop.json"), createMemorySecretKeyStore())
	statusChanges := make(chan string, 1)
	AddStatusListener(service, func() { statusChanges <- service.GetStatus().State })
	service.mutex.Lock()
	service.notifyStatusListener()
	service.mutex.Unlock()
	expectStatusChange(t, statusChanges, StateSignedOut)
}
