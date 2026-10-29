package biometric

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeFingerprintDevice plays fprintd: each VerifyStart sends the next scan's statuses.
type fakeFingerprintDevice struct {
	mutex      sync.Mutex
	fingers    []string
	listError  error
	claimError error
	scans      [][]verifyStatus
	startCount int
	isClaimed  bool
	stopCount  int
	statuses   chan verifyStatus
	closeCount int
	openCount  int
}

func createFakeDevice(scans ...[]verifyStatus) *fakeFingerprintDevice {
	return &fakeFingerprintDevice{fingers: []string{"right-index-finger"}, scans: scans, statuses: make(chan verifyStatus, 16)}
}

func (device *fakeFingerprintDevice) open(context.Context) (fingerprintDevice, error) {
	device.mutex.Lock()
	defer device.mutex.Unlock()
	device.openCount++
	return device, nil
}

func (device *fakeFingerprintDevice) ListEnrolledFingers() ([]string, error) {
	return device.fingers, device.listError
}

func (device *fakeFingerprintDevice) Claim() error {
	device.isClaimed = device.claimError == nil
	return device.claimError
}

func (device *fakeFingerprintDevice) Release() error {
	device.isClaimed = false
	return nil
}

func (device *fakeFingerprintDevice) VerifyStart() error {
	device.mutex.Lock()
	defer device.mutex.Unlock()
	if device.startCount < len(device.scans) {
		for _, status := range device.scans[device.startCount] {
			device.statuses <- status
		}
	}
	device.startCount++
	return nil
}

func (device *fakeFingerprintDevice) VerifyStop() error {
	device.stopCount++
	return nil
}

func (device *fakeFingerprintDevice) Statuses() <-chan verifyStatus { return device.statuses }

func (device *fakeFingerprintDevice) Close() { device.closeCount++ }

func TestFingerprintMatches(t *testing.T) {
	device := createFakeDevice([]verifyStatus{{result: verifyMatch, isDone: true}})
	if err := newFingerprintReader(device.open).verify(); err != nil {
		t.Fatal(err)
	}
	if device.isClaimed || device.stopCount != 1 || device.closeCount != 1 {
		t.Errorf("the reader was not given back: %+v", device)
	}
}

// A scan the reader asks to repeat does not count as a finger that did not match; three that do not match fail.
func TestFingerprintTakesThreeFingersAndRetriesScans(t *testing.T) {
	noMatch := []verifyStatus{{result: verifyNoMatch, isDone: true}}
	device := createFakeDevice(
		[]verifyStatus{{result: "verify-retry-scan", isDone: false}, {result: verifyNoMatch, isDone: true}},
		[]verifyStatus{{result: "verify-swipe-too-short", isDone: true}},
		noMatch,
		[]verifyStatus{{result: verifyMatch, isDone: true}},
	)
	if err := newFingerprintReader(device.open).verify(); err != nil || device.startCount != 4 {
		t.Fatalf("the fourth scan, a third finger, matched: %v after %d scans", err, device.startCount)
	}
	device = createFakeDevice(noMatch, noMatch, noMatch, []verifyStatus{{result: verifyMatch, isDone: true}})
	if err := newFingerprintReader(device.open).verify(); !errors.Is(err, errFingerprintNoMatch) || device.startCount != 3 {
		t.Errorf("three fingers that do not match: %v after %d scans", err, device.startCount)
	}
}

func TestFingerprintCheckIsCancelled(t *testing.T) {
	device := createFakeDevice()
	reader := newFingerprintReader(device.open)
	result := make(chan error, 1)
	go func() { result <- reader.verify() }()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		device.mutex.Lock()
		isStarted := device.startCount > 0
		device.mutex.Unlock()
		if isStarted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the scan never started")
		}
	}
	reader.cancel()
	select {
	case err := <-result:
		if !errors.Is(err, ErrCancelled) || device.isClaimed {
			t.Errorf("cancel: %v, claimed %v", err, device.isClaimed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancel did not end the check")
	}
}

func TestFingerprintReaderInUseOrGone(t *testing.T) {
	device := createFakeDevice()
	device.claimError = dbusError(fprintErrorAlreadyInUse)
	if err := newFingerprintReader(device.open).verify(); !errors.Is(err, errFingerprintInUse) {
		t.Errorf("in use: %v", err)
	}
	device = createFakeDevice([]verifyStatus{{result: verifyDisconnected, isDone: true}})
	if err := newFingerprintReader(device.open).verify(); !errors.Is(err, errFingerprintDisconnected) {
		t.Errorf("disconnected: %v", err)
	}
}

func TestFingerprintUnavailableReasons(t *testing.T) {
	for _, testCase := range []struct {
		openError, listError error
		fingers              []string
		want                 string
	}{
		{fingers: []string{"right-index-finger"}},
		{fingers: []string{}, want: "No fingerprint is enrolled"},
		{listError: dbusError(fprintErrorNoEnrolledPrints), want: "No fingerprint is enrolled"},
		{openError: dbusError(fprintErrorNoSuchDevice), want: "No fingerprint reader was found"},
		{openError: dbusError(dbusErrorServiceUnknown), want: "Fingerprint unlock needs fprintd"},
		{openError: dbusError(fprintErrorPermissionDenied), want: "polkit denied"},
		{openError: errors.New("no system bus"), want: "could not reach fprintd"},
	} {
		device := createFakeDevice()
		device.fingers, device.listError = testCase.fingers, testCase.listError
		open := device.open
		if testCase.openError != nil {
			open = func(context.Context) (fingerprintDevice, error) { return nil, testCase.openError }
		}
		reason := newFingerprintReader(open).getUnavailableReason()
		if (testCase.want == "") != (reason == "") || !strings.Contains(reason, testCase.want) {
			t.Errorf("%v %v %v: %q", testCase.openError, testCase.listError, testCase.fingers, reason)
		}
	}
}

// The status asks many times a second, so fprintd is asked again only after availabilityLifetime.
func TestFingerprintAvailabilityIsAskedAtMostEveryTenSeconds(t *testing.T) {
	device := createFakeDevice()
	reader := newFingerprintReader(device.open)
	now := time.Unix(1_700_000_000, 0)
	reader.now = func() time.Time { return now }
	for range 5 {
		reader.getUnavailableReason()
	}
	now = now.Add(availabilityLifetime)
	device.fingers = nil
	if reason := reader.getUnavailableReason(); !strings.Contains(reason, "No fingerprint is enrolled") || device.openCount != 2 {
		t.Errorf("after %v: %q, %d opens", availabilityLifetime, reason, device.openCount)
	}
}

func TestFingerprintKeysStayInMemory(t *testing.T) {
	reader := newFingerprintReader(createFakeDevice().open)
	key := []byte("wrapping key")
	if err := reader.storeKey("account", key); err != nil {
		t.Fatal(err)
	}
	key[0] = 'X'
	if loaded, err := reader.loadKey("account"); err != nil || !bytes.Equal(loaded, []byte("wrapping key")) || !reader.hasKey("account") {
		t.Fatalf("load: %q %v", loaded, err)
	}
	reader.deleteKey("account")
	if _, err := reader.loadKey("account"); !errors.Is(err, ErrKeyNotFound) || reader.hasKey("account") {
		t.Errorf("after delete: %v", err)
	}
}
