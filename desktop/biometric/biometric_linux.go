package biometric

import (
	"context"
	"errors"
	"fmt"
	"unsafe"

	"github.com/godbus/dbus/v5"
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

func (biometric *Biometric) PrepareStoreKey(accountID string) error { return nil }
func (biometric *Biometric) PrepareLoadKey(accountID string) error  { return nil }

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
	signals    chan *dbus.Signal
	statuses   chan verifyStatus
	done       chan struct{}
}

func openFprintdDevice(ctx context.Context) (fingerprintDevice, error) {
	// No dbus.WithContext: it would close the connection when ctx ends, while the check still uses it.
	connection, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", namedError(dbusErrorServiceUnknown), err)
	}
	var path dbus.ObjectPath
	if err := connection.Object(fprintdService, fprintdManagerPath).CallWithContext(ctx, fprintdManager+".GetDefaultDevice", 0).Store(&path); err != nil {
		connection.Close()
		return nil, convertDBusError(err)
	}
	device := &fprintdConnection{
		connection: connection, device: connection.Object(fprintdService, path), path: path,
		signals: make(chan *dbus.Signal, 16), statuses: make(chan verifyStatus, 16), done: make(chan struct{}),
	}
	if err := connection.AddMatchSignal(dbus.WithMatchObjectPath(path), dbus.WithMatchInterface(fprintdDevice), dbus.WithMatchMember("VerifyStatus")); err != nil {
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
			if signal.Path != device.path || signal.Name != fprintdVerifyStatus || len(signal.Body) != 2 {
				continue
			}
			result, isResult := signal.Body[0].(string)
			isDone, isFlag := signal.Body[1].(bool)
			if !isResult || !isFlag {
				continue
			}
			select {
			case device.statuses <- verifyStatus{result: result, isDone: isDone}:
			case <-device.done:
				return
			}
		}
	}
}

func (device *fprintdConnection) call(method string, arguments ...any) *dbus.Call {
	return device.device.Call(fprintdDevice+"."+method, 0, arguments...)
}

func (device *fprintdConnection) ListEnrolledFingers() ([]string, error) {
	var fingers []string
	// "" is the user who calls.
	if err := device.call("ListEnrolledFingers", "").Store(&fingers); err != nil {
		return nil, convertDBusError(err)
	}
	return fingers, nil
}

func (device *fprintdConnection) Claim() error {
	return convertDBusError(device.call("Claim", "").Err)
}

func (device *fprintdConnection) Release() error {
	return convertDBusError(device.call("Release").Err)
}

func (device *fprintdConnection) VerifyStart() error {
	return convertDBusError(device.call("VerifyStart", "any").Err)
}

func (device *fprintdConnection) VerifyStop() error {
	return convertDBusError(device.call("VerifyStop").Err)
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
