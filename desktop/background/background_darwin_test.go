//go:build darwin && !ios && cgo

package background

import "testing"

func TestLoginItemOutsideAppBundleIsUnavailable(t *testing.T) {
	loginItem := NewLoginItem(t.TempDir())
	if state, err := loginItem.GetState(); err != nil || state != LoginItemUnavailable {
		t.Errorf("state %s %v", state, err)
	}
	if err := loginItem.Register(); err != ErrUnavailable {
		t.Errorf("register: %v", err)
	}
	if err := loginItem.Unregister(); err != nil {
		t.Errorf("unregister: %v", err)
	}
	if loginItem.CanOpenSystemSettings() {
		t.Error("a test binary offers System Settings")
	}
	if WasLaunchedAsLoginItem() {
		t.Error("a test binary was launched as a login item")
	}
}
