package localstate

import (
	"errors"
	"slices"

	"github.com/godbus/dbus/v5"
)

const secretServiceName = "org.freedesktop.secrets"

var errSecretServiceMissing = errors.New("localstate: no Secret Service on the session bus")

// checkSecretService asks the session bus whether a Secret Service runs, or can be started on demand.
func checkSecretService() error {
	connection, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	defer connection.Close()
	bus := connection.BusObject()
	var hasOwner bool
	if err := bus.Call("org.freedesktop.DBus.NameHasOwner", 0, secretServiceName).Store(&hasOwner); err != nil {
		return err
	}
	if hasOwner {
		return nil
	}
	var activatableNames []string
	if err := bus.Call("org.freedesktop.DBus.ListActivatableNames", 0).Store(&activatableNames); err != nil {
		return err
	}
	if slices.Contains(activatableNames, secretServiceName) {
		return nil
	}
	return errSecretServiceMissing
}
