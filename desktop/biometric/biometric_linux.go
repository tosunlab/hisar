package biometric

import (
	"context"
	"errors"
	"fmt"
	"unsafe"

	"github.com/godbus/dbus/v5"
	"github.com/tryhisar/hisar/core/app"
)

// Name is what Hisar's sentences call the check: "fingerprint unlock is not available on this computer".
const Name = "fingerprint"

const (
	unavailableMessage = "Fingerprint unlock is not available on this computer"
	cancelledMessage   = "Fingerprint check was cancelled"
	keyNotFoundMessage = "the fingerprint key is gone because Hisar quit since; unlock with your master password"
)

// platformBiometric checks fingerprints through fprintd on the system bus.
type platformBiometric struct {
	reader *fingerprintReader
}

func (biometric *Biometric) getReader() *fingerprintReader {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	if biometric.platform.reader == nil {
		biometric.platform.reader = newFingerprintReader(openFprintdDevice)
	}
	return biometric.platform.reader
}

func (biometric *Biometric) IsAvailable() bool {
	return biometric.getReader().getUnavailableReason() == ""
}

// GetUnavailableReason says why the fingerprint cannot unlock: no fprintd, no reader, no enrolled finger or polkit.
func (biometric *Biometric) GetUnavailableReason() string {
	return biometric.getReader().getUnavailableReason()
}

// Authenticate blocks until a finger matches, three do not, or CancelPrompt ends it. fprintd shows no reason.
func (biometric *Biometric) Authenticate(reason string) error { return biometric.getReader().verify() }

func (biometric *Biometric) CancelPrompt() { biometric.getReader().cancel() }

// StoreKey keeps the key in this process only: a keyring item would let any program of the user open the vault without
// a fingerprint, and Hisar asks for the master password after every start anyway.
func (biometric *Biometric) StoreKey(accountID string, key []byte) error {
	return biometric.getReader().storeKey(accountID, key)
}

func (biometric *Biometric) LoadKey(accountID string) ([]byte, error) {
	return biometric.getReader().loadKey(accountID)
}

func (biometric *Biometric) DeleteKey(accountID string) error {
	biometric.getReader().deleteKey(accountID)
	return nil
}

// HasKey makes this an app.BiometricSessionKeyStore: the password unlock after a start seals the key again.
func (biometric *Biometric) HasKey(accountID string) bool {
	return biometric.getReader().hasKey(accountID)
}

// GetKeyPreparer is nil on Linux: a key kept in memory needs no prompt to be stored or read, so the master password
// change that seals it again asks for no finger.
func (biometric *Biometric) GetKeyPreparer() app.BiometricKeyPreparer { return nil }

func moveActivePrompt(frame PromptFrame) {}

// authenticateInWindow has no in-window view on Linux: the approval window shows its own fingerprint prompt.
func (biometric *Biometric) authenticateInWindow(reason string, window unsafe.Pointer, frame *PromptFrame, size PromptSize) error {
	return biometric.Authenticate(reason)
}

const (
	fprintdService      = "net.reactivated.Fprint"
	fprintdManagerPath  = "/net/reactivated/Fprint/Manager"
	fprintdManager      = "net.reactivated.Fprint.Manager"
	fprintdDevice       = "net.reactivated.Fprint.Device"
	fprintdVerifyStatus = fprintdDevice + ".VerifyStatus"
)

// fprintdDevice is fprintd's default device on its own system bus connection, so its VerifyStatus signals reach only
// this check.
type fprintdConnection struct {
	connection *dbus.Conn
	device     dbus.BusObject
	path       dbus.ObjectPath
	// owner is the unique bus name that owns net.reactivated.Fprint: the only sender whose VerifyStatus counts.
	owner    string
	signals  chan *dbus.Signal
	statuses chan verifyStatus
	done     chan struct{}
}

// connectSystemBus connects within ctx. dbus.WithContext would close the connection when ctx ends, while the check
// still uses it, so the connection is made apart and dropped when ctx ends first.
func connectSystemBus(ctx context.Context) (*dbus.Conn, error) {
	type connected struct {
		connection *dbus.Conn
		err        error
	}
	result := make(chan connected, 1)
	go func() {
		connection, err := dbus.ConnectSystemBus()
		result <- connected{connection, err}
	}()
	select {
	case done := <-result:
		return done.connection, done.err
	case <-ctx.Done():
		go func() {
			if late := <-result; late.connection != nil {
				late.connection.Close()
			}
		}()
		return nil, ctx.Err()
	}
}

func openFprintdDevice(ctx context.Context) (fingerprintDevice, error) {
	connection, err := connectSystemBus(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", namedError(dbusErrorServiceUnknown), err)
	}
	var path dbus.ObjectPath
	if err := connection.Object(fprintdService, fprintdManagerPath).CallWithContext(ctx, fprintdManager+".GetDefaultDevice", 0).Store(&path); err != nil {
		connection.Close()
		return nil, convertDBusError(err)
	}
	var owner string
	if err := connection.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetNameOwner", 0, fprintdService).Store(&owner); err != nil {
		connection.Close()
		return nil, convertDBusError(err)
	}
	device := &fprintdConnection{
		connection: connection, device: connection.Object(fprintdService, path), path: path, owner: owner,
		signals: make(chan *dbus.Signal, 16), statuses: make(chan verifyStatus, 16), done: make(chan struct{}),
	}
	// The system bus lets any program send a signal; the bus fills in the sender, so only fprintd's count. The daemon
	// matches the well-known name to its owner, and readVerifyStatus checks the owner again.
	rule := []dbus.MatchOption{
		dbus.WithMatchSender(fprintdService), dbus.WithMatchObjectPath(path), dbus.WithMatchInterface(fprintdDevice),
		dbus.WithMatchMember("VerifyStatus"),
	}
	if err := connection.AddMatchSignalContext(ctx, rule...); err != nil {
		connection.Close()
		return nil, convertDBusError(err)
	}
	connection.Signal(device.signals)
	go device.forwardStatuses()
	return device, nil
}

func (device *fprintdConnection) forwardStatuses() {
	defer close(device.statuses)
	for {
		select {
		case <-device.done:
			return
		case signal, isOpen := <-device.signals:
			if !isOpen {
				return
			}
			status, isFprintd := readVerifyStatus(signal, device.path, device.owner)
			if !isFprintd {
				continue
			}
			select {
			case device.statuses <- status:
			case <-device.done:
				return
			}
		}
	}
}

// readVerifyStatus is a VerifyStatus signal of the device at path, sent by owner, fprintd's unique bus name; any other
// signal, such as one another program sent to look like fprintd's, is not one.
func readVerifyStatus(signal *dbus.Signal, path dbus.ObjectPath, owner string) (verifyStatus, bool) {
	if signal == nil || owner == "" || signal.Sender != owner || signal.Path != path || signal.Name != fprintdVerifyStatus || len(signal.Body) != 2 {
		return verifyStatus{}, false
	}
	result, isResult := signal.Body[0].(string)
	isDone, isFlag := signal.Body[1].(bool)
	if !isResult || !isFlag {
		return verifyStatus{}, false
	}
	return verifyStatus{result: result, isDone: isDone}, true
}

func (device *fprintdConnection) call(ctx context.Context, method string, arguments ...any) *dbus.Call {
	return device.device.CallWithContext(ctx, fprintdDevice+"."+method, 0, arguments...)
}

func (device *fprintdConnection) ListEnrolledFingers(ctx context.Context) ([]string, error) {
	var fingers []string
	// "" is the user who calls.
	if err := device.call(ctx, "ListEnrolledFingers", "").Store(&fingers); err != nil {
		return nil, convertDBusError(err)
	}
	return fingers, nil
}

func (device *fprintdConnection) Claim(ctx context.Context) error {
	return convertDBusError(device.call(ctx, "Claim", "").Err)
}

func (device *fprintdConnection) Release(ctx context.Context) error {
	return convertDBusError(device.call(ctx, "Release").Err)
}

func (device *fprintdConnection) VerifyStart(ctx context.Context) error {
	return convertDBusError(device.call(ctx, "VerifyStart", "any").Err)
}

func (device *fprintdConnection) VerifyStop(ctx context.Context) error {
	return convertDBusError(device.call(ctx, "VerifyStop").Err)
}

func (device *fprintdConnection) Statuses() <-chan verifyStatus { return device.statuses }

func (device *fprintdConnection) Close() {
	close(device.done)
	device.connection.RemoveSignal(device.signals)
	device.connection.Close()
}

// convertDBusError keeps a D-Bus error's name as a namedError, so the reason it gives can be told apart.
func convertDBusError(err error) error {
	if err == nil {
		return nil
	}
	var dbusError dbus.Error
	if errors.As(err, &dbusError) {
		return fmt.Errorf("%w: %v", namedError(dbusError.Name), dbusError.Body)
	}
	var dbusErrorPointer *dbus.Error
	if errors.As(err, &dbusErrorPointer) {
		return fmt.Errorf("%w: %v", namedError(dbusErrorPointer.Name), dbusErrorPointer.Body)
	}
	return err
}
