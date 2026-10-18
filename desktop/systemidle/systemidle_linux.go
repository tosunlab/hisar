// Package systemidle reads how long the session has had no keyboard or mouse input, in any app.
package systemidle

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	idleCallTimeout = 500 * time.Millisecond
	// connectTimeout limits dialing the session bus, which runs under the app's lock from the auto-lock timer.
	connectTimeout = 2 * time.Second
	// unavailableRetry is how long a desktop without an idle monitor is not asked again.
	unavailableRetry = time.Minute
)

const (
	mutterService      = "org.gnome.Mutter.IdleMonitor"
	screenSaverService = "org.freedesktop.ScreenSaver"
	// kdeScreenSaverService is KDE's own name for the screen locker, which also owns screenSaverService.
	kdeScreenSaverService = "org.kde.screensaver"
)

// idleSources are the session-bus idle monitors, tried in order: GNOME's Mutter (milliseconds), then the
// freedesktop screensaver that KDE, Xfce and others provide (seconds, but milliseconds from KDE: getScreenSaverUnit).
var idleSources = []struct {
	service, path, method string
}{
	{mutterService, "/org/gnome/Mutter/IdleMonitor/Core", "org.gnome.Mutter.IdleMonitor.GetIdletime"},
	{screenSaverService, "/org/freedesktop/ScreenSaver", "org.freedesktop.ScreenSaver.GetSessionIdleTime"},
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
	// A connection the bus dropped stays closed, so dial again rather than fail on it forever.
	if idleState.connection != nil && !idleState.connection.Connected() {
		idleState.connection.Close()
		idleState.connection = nil
	}
	if idleState.connection == nil {
		connection, err := connectSessionBus()
		if err != nil {
			idleState.unavailableUntil = time.Now().Add(unavailableRetry)
			return 0, false
		}
		idleState.connection = connection
	}
	for _, source := range idleSources {
		unit := time.Millisecond
		if source.service == screenSaverService {
			unit = getScreenSaverUnit(getNameOwner(idleState.connection, screenSaverService), getNameOwner(idleState.connection, kdeScreenSaverService))
		}
		ctx, cancel := context.WithTimeout(context.Background(), idleCallTimeout)
		call := idleState.connection.Object(source.service, dbus.ObjectPath(source.path)).CallWithContext(ctx, source.method, 0)
		cancel()
		if call.Err != nil {
			continue
		}
		if idle, isRead := readIdleValue(call.Body, unit); isRead {
			return idle, true
		}
	}
	idleState.unavailableUntil = time.Now().Add(unavailableRetry)
	return 0, false
}

// connectSessionBus dials a connection of Hisar's own, so it can be closed and dialed again, and gives up after
// connectTimeout; a dial that finishes later is closed.
func connectSessionBus() (*dbus.Conn, error) {
	type result struct {
		connection *dbus.Conn
		err        error
	}
	results := make(chan result, 1)
	go func() {
		connection, err := dbus.ConnectSessionBus()
		results <- result{connection: connection, err: err}
	}()
	select {
	case connected := <-results:
		return connected.connection, connected.err
	case <-time.After(connectTimeout):
		go func() {
			if late := <-results; late.connection != nil {
				late.connection.Close()
			}
		}()
		return nil, errors.New("systemidle: the session bus did not answer")
	}
}

// getNameOwner returns the unique bus name that owns name, or "" when nobody does.
func getNameOwner(connection *dbus.Conn, name string) string {
	ctx, cancel := context.WithTimeout(context.Background(), idleCallTimeout)
	defer cancel()
	var owner string
	if connection.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetNameOwner", 0, name).Store(&owner) != nil {
		return ""
	}
	return owner
}
