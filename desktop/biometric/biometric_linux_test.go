package biometric

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

// Any program on the system bus can send a VerifyStatus signal on fprintd's device path; the bus names its sender, so
// only one from fprintd's own connection counts.
func TestOnlyFprintdsVerifyStatusCounts(t *testing.T) {
	const devicePath = dbus.ObjectPath("/net/reactivated/Fprint/Device/0")
	const fprintd = ":1.7"
	match := &dbus.Signal{Sender: fprintd, Path: devicePath, Name: fprintdVerifyStatus, Body: []any{verifyMatch, true}}
	if status, isFprintd := readVerifyStatus(match, devicePath, fprintd); !isFprintd || status != (verifyStatus{result: verifyMatch, isDone: true}) {
		t.Fatalf("fprintd's own match: %+v %v", status, isFprintd)
	}
	for name, signal := range map[string]*dbus.Signal{
		"another sender":        {Sender: ":1.42", Path: devicePath, Name: fprintdVerifyStatus, Body: []any{verifyMatch, true}},
		"the well-known name":   {Sender: fprintdService, Path: devicePath, Name: fprintdVerifyStatus, Body: []any{verifyMatch, true}},
		"another device":        {Sender: fprintd, Path: "/net/reactivated/Fprint/Device/1", Name: fprintdVerifyStatus, Body: []any{verifyMatch, true}},
		"another member":        {Sender: fprintd, Path: devicePath, Name: fprintdDevice + ".VerifyFingerSelected", Body: []any{verifyMatch, true}},
		"a malformed body":      {Sender: fprintd, Path: devicePath, Name: fprintdVerifyStatus, Body: []any{verifyMatch}},
		"a body of other types": {Sender: fprintd, Path: devicePath, Name: fprintdVerifyStatus, Body: []any{true, verifyMatch}},
	} {
		if status, isFprintd := readVerifyStatus(signal, devicePath, fprintd); isFprintd {
			t.Errorf("%s counted: %+v", name, status)
		}
	}
	if _, isFprintd := readVerifyStatus(match, devicePath, ""); isFprintd {
		t.Error("a signal counted with no known owner")
	}
}
