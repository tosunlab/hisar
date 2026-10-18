// Package systemidle reads how long the session has had no keyboard or mouse input, in any app.
package systemidle

import (
	"context"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	idleCallTimeout = 500 * time.Millisecond
	// unavailableRetry is how long a desktop without an idle monitor is not asked again.
	unavailableRetry = time.Minute
)

// idleSources are the session-bus idle monitors, tried in order: GNOME's Mutter (milliseconds), then the
// freedesktop screensaver that KDE, Xfce and others provide (seconds).
var idleSources = []struct {
	service, path, method string
	unit                  time.Duration
}{
	{"org.gnome.Mutter.IdleMonitor", "/org/gnome/Mutter/IdleMonitor/Core", "org.gnome.Mutter.IdleMonitor.GetIdletime", time.Millisecond},
	{"org.freedesktop.ScreenSaver", "/org/freedesktop/ScreenSaver", "org.freedesktop.ScreenSaver.GetSessionIdleTime", time.Second},
}

var idleState struct {
	mutex            sync.Mutex
	connection       *dbus.Conn
	unavailableUntil time.Time
}

// GetIdleDuration asks the desktop's idle monitor; without one (no session bus, a desktop that has neither, or KDE on
// Wayland, which refuses), auto-lock measures activity in Hisar itself.
func GetIdleDuration() (time.Duration, bool) {
	idleState.mutex.Lock()
	defer idleState.mutex.Unlock()
	if time.Now().Before(idleState.unavailableUntil) {
		return 0, false
	}
	if idleState.connection == nil {
		connection, err := dbus.SessionBus()
		if err != nil {
			idleState.unavailableUntil = time.Now().Add(unavailableRetry)
			return 0, false
		}
		idleState.connection = connection
	}
	for _, source := range idleSources {
		ctx, cancel := context.WithTimeout(context.Background(), idleCallTimeout)
		call := idleState.connection.Object(source.service, dbus.ObjectPath(source.path)).CallWithContext(ctx, source.method, 0)
		cancel()
		if call.Err != nil {
			continue
		}
		if idle, isRead := readIdleValue(call.Body, source.unit); isRead {
			return idle, true
		}
	}
	idleState.unavailableUntil = time.Now().Add(unavailableRetry)
	return 0, false
}
