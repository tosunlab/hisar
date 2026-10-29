package biometric

import (
	"context"
	"errors"
	"log"
	"slices"
	"strings"
	"sync"
	"time"
)

// fprintd's D-Bus error names and verify results (net.reactivated.Fprint.Device).
const (
	fprintErrorNoEnrolledPrints = "net.reactivated.Fprint.Error.NoEnrolledPrints"
	fprintErrorNoSuchDevice     = "net.reactivated.Fprint.Error.NoSuchDevice"
	fprintErrorPermissionDenied = "net.reactivated.Fprint.Error.PermissionDenied"
	fprintErrorAlreadyInUse     = "net.reactivated.Fprint.Error.AlreadyInUse"
	dbusErrorServiceUnknown     = "org.freedesktop.DBus.Error.ServiceUnknown"

	verifyMatch        = "verify-match"
	verifyNoMatch      = "verify-no-match"
	verifyDisconnected = "verify-disconnected"
	// verifyRetry is no fprintd result: a scan that ended asking for the finger again.
	verifyRetry = "retry"

	// fingerprintAttempts is how many non-matching fingers one check takes, as fprintd's PAM module does.
	fingerprintAttempts = 3
	// availabilityLifetime keeps the status Hisar builds many times a second from asking fprintd each time.
	availabilityLifetime = 10 * time.Second
)

var (
	errFingerprintNoMatch      = errors.New("Fingerprint not recognized. Try again, or use your master password.")
	errFingerprintInUse        = errors.New("Fingerprint reader is in use by another program. Try again, or use your master password.")
	errFingerprintDisconnected = errors.New("Fingerprint reader was disconnected. Use your master password.")
)

// fingerprintDevice is the default fprintd device of the user's session, or a fake in tests. Errors carry fprintd's D-Bus
// error name (dbusErrorName). Every call ends when its context does.
type fingerprintDevice interface {
	ListEnrolledFingers(ctx context.Context) ([]string, error)
	Claim(ctx context.Context) error
	Release(ctx context.Context) error
	// VerifyStart starts a scan of any enrolled finger; its results arrive on statuses until one is done.
	VerifyStart(ctx context.Context) error
	VerifyStop(ctx context.Context) error
	// Statuses are the device's VerifyStatus signals, only those fprintd itself sent.
	Statuses() <-chan verifyStatus
	Close()
}

type verifyStatus struct {
	result string
	isDone bool
}

// openFingerprintDevice opens the default device; an error names why there is none.
type openFingerprintDevice func(ctx context.Context) (fingerprintDevice, error)

const (
	// availabilityTimeout bounds the check that a status may wait on, the first one only.
	availabilityTimeout = time.Second
	// callTimeout bounds each call of a scan, and the Release and VerifyStop after one ends.
	callTimeout = 5 * time.Second
	// maximumScanDuration gives the reader back to fprintd's other users (sudo, the desktop's lock screen) when nobody
	// scans: fprintd lets one program claim it at a time.
	maximumScanDuration = time.Minute
)

// fingerprintReader checks fingerprints through fprintd and keeps the biometric unlock keys in memory only: no keyring
// item can require a fingerprint on Linux, and Hisar asks for the master password after every start anyway.
type fingerprintReader struct {
	open openFingerprintDevice
	now  func() time.Time
	// scanDuration is maximumScanDuration, shorter in tests.
	scanDuration time.Duration

	mutex             sync.Mutex
	keysByAccountID   map[string][]byte
	checkedAt         time.Time
	unavailableReason string
	isChecking        bool
	cancelVerify      context.CancelFunc
	verifyMutex       sync.Mutex
}

func newFingerprintReader(open openFingerprintDevice) *fingerprintReader {
	return &fingerprintReader{open: open, now: time.Now, scanDuration: maximumScanDuration, keysByAccountID: map[string][]byte{}}
}

// getUnavailableReason is "" when a reader has an enrolled finger. Hisar's status asks under its service mutex, so
// only the first check waits, for at most availabilityTimeout; after availabilityLifetime the last answer stands while
// one check in the background asks fprintd again.
func (reader *fingerprintReader) getUnavailableReason() string {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()
	if reader.checkedAt.IsZero() {
		reader.mutex.Unlock()
		reason := reader.checkAvailability()
		reader.mutex.Lock()
		reader.checkedAt, reader.unavailableReason = reader.now(), reason
		return reason
	}
	if reader.now().Sub(reader.checkedAt) >= availabilityLifetime && !reader.isChecking {
		reader.isChecking = true
		go func() {
			reason := reader.checkAvailability()
			reader.mutex.Lock()
			defer reader.mutex.Unlock()
			reader.checkedAt, reader.unavailableReason, reader.isChecking = reader.now(), reason, false
		}()
	}
	return reader.unavailableReason
}

func (reader *fingerprintReader) checkAvailability() string {
	ctx, cancel := context.WithTimeout(context.Background(), availabilityTimeout)
	defer cancel()
	device, err := reader.open(ctx)
	if err != nil {
		return describeUnavailable(err)
	}
	defer device.Close()
	fingers, err := device.ListEnrolledFingers(ctx)
	if err != nil {
		return describeUnavailable(err)
	}
	if len(fingers) == 0 {
		return describeUnavailable(dbusError(fprintErrorNoEnrolledPrints))
	}
	return ""
}

func describeUnavailable(err error) string {
	switch dbusErrorName(err) {
	case fprintErrorNoEnrolledPrints:
		return "No fingerprint is enrolled for your account. Enroll one in your desktop's settings (Users, or fprintd-enroll), then turn this on."
	case fprintErrorNoSuchDevice:
		return "No fingerprint reader was found on this computer."
	case fprintErrorPermissionDenied:
		return "Your system does not let Hisar check fingerprints (polkit denied net.reactivated.fprint.device.verify)."
	case dbusErrorServiceUnknown:
		return "Fingerprint unlock needs fprintd, the fingerprint service of Linux desktops. Install it, enroll a finger in your desktop's settings, then turn this on."
	}
	return "Hisar could not reach fprintd, the fingerprint service, so fingerprint unlock is not available."
}

// withCallTimeout runs one call of a scan under ctx and callTimeout.
func withCallTimeout(ctx context.Context, call func(context.Context) error) error {
	callContext, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	return call(callContext)
}

// verify claims the reader, scans until a finger matches, fingerprintAttempts fingers do not, cancel ends it or
// maximumScanDuration passes, and releases the reader. Cancel also ends a call fprintd has not answered yet.
func (reader *fingerprintReader) verify() error {
	reader.verifyMutex.Lock()
	defer reader.verifyMutex.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), reader.scanDuration)
	reader.mutex.Lock()
	reader.cancelVerify = cancel
	reader.mutex.Unlock()
	defer func() {
		reader.mutex.Lock()
		reader.cancelVerify = nil
		reader.mutex.Unlock()
		cancel()
	}()

	var device fingerprintDevice
	err := withCallTimeout(ctx, func(callContext context.Context) error {
		var err error
		device, err = reader.open(callContext)
		return err
	})
	if err != nil {
		return reader.endedOrUnavailable(ctx, err)
	}
	defer device.Close()
	if err := withCallTimeout(ctx, device.Claim); err != nil {
		if dbusErrorName(err) == fprintErrorAlreadyInUse {
			return errFingerprintInUse
		}
		return reader.endedOrUnavailable(ctx, err)
	}
	// The scan's own context may have ended: the reader is given back with a deadline of its own.
	defer withCallTimeout(context.Background(), device.Release)
	for noMatchCount := 0; ; {
		if err := withCallTimeout(ctx, device.VerifyStart); err != nil {
			return reader.endedOrUnavailable(ctx, err)
		}
		result, err := waitForVerifyResult(ctx, device)
		withCallTimeout(context.Background(), device.VerifyStop)
		switch {
		case err != nil:
			return err
		case result == verifyMatch:
			return nil
		case result == verifyRetry:
		case result == verifyNoMatch:
			if noMatchCount++; noMatchCount >= fingerprintAttempts {
				return errFingerprintNoMatch
			}
		case result == verifyDisconnected:
			return errFingerprintDisconnected
		default:
			return errors.New("Fingerprint check failed (" + result + "). Use your master password.")
		}
	}
}

// endedOrUnavailable is ErrCancelled when the scan was cancelled or ran out of time during a call, else the call's
// error as unavailableError gives it.
func (reader *fingerprintReader) endedOrUnavailable(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ErrCancelled
	}
	return reader.unavailableError(err)
}

// waitForVerifyResult waits for the scan's final result. A retry result ("verify-retry-scan", "verify-swipe-too-short",
// …) that is not done only asks for the finger again; one that is done ends the scan, and verifyRetry starts it again
// without counting a finger that did not match.
func waitForVerifyResult(ctx context.Context, device fingerprintDevice) (string, error) {
	statuses := device.Statuses()
	for {
		select {
		case <-ctx.Done():
			return "", ErrCancelled
		case status, isOpen := <-statuses:
			switch {
			case !isOpen:
				return verifyDisconnected, nil
			case status.result == verifyMatch || status.result == verifyNoMatch || status.result == verifyDisconnected:
				return status.result, nil
			case !status.isDone:
			case strings.HasPrefix(status.result, "verify-retry") || slices.Contains(retryResults, status.result):
				return verifyRetry, nil
			default:
				return status.result, nil
			}
		}
	}
}

// retryResults ask for the finger again besides the verify-retry-* results.
var retryResults = []string{"verify-swipe-too-short", "verify-finger-not-centered", "verify-remove-and-retry"}

// unavailableError is an error the check cannot recover from, with the reason the next status shows.
func (reader *fingerprintReader) unavailableError(err error) error {
	log.Printf("fingerprint: %v", err)
	reason := describeUnavailable(err)
	reader.mutex.Lock()
	reader.checkedAt, reader.unavailableReason = reader.now(), reason
	reader.mutex.Unlock()
	return errors.New(reason)
}

func (reader *fingerprintReader) cancel() {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()
	if reader.cancelVerify != nil {
		reader.cancelVerify()
	}
}

func (reader *fingerprintReader) storeKey(accountID string, key []byte) error {
	if len(key) == 0 {
		return errors.New("empty key")
	}
	reader.mutex.Lock()
	defer reader.mutex.Unlock()
	clear(reader.keysByAccountID[accountID])
	reader.keysByAccountID[accountID] = append([]byte(nil), key...)
	return nil
}

func (reader *fingerprintReader) loadKey(accountID string) ([]byte, error) {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()
	key, isFound := reader.keysByAccountID[accountID]
	if !isFound {
		return nil, ErrKeyNotFound
	}
	return append([]byte(nil), key...), nil
}

func (reader *fingerprintReader) deleteKey(accountID string) {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()
	clear(reader.keysByAccountID[accountID])
	delete(reader.keysByAccountID, accountID)
}

func (reader *fingerprintReader) hasKey(accountID string) bool {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()
	_, isFound := reader.keysByAccountID[accountID]
	return isFound
}

// namedError is a D-Bus error by name, for fakes and for fingers fprintd lists as none.
type namedError string

func (err namedError) Error() string { return string(err) }

func dbusError(name string) error { return namedError(name) }

// dbusErrorName is the D-Bus error name an error wraps as a namedError, "" for none.
func dbusErrorName(err error) string {
	var named namedError
	if errors.As(err, &named) {
		return string(named)
	}
	return ""
}
