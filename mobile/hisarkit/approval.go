package hisarkit

import (
	"errors"

	"github.com/tryhisar/hisar/core/app"
)

var errBiometricUnavailable = errors.New("Face ID is not available; approve with your master password")

// ListAllPendingApproval returns the requests other devices of the account are waiting on ([]app.PhoneApprovalRequest) as JSON.
func (hisar *Hisar) ListAllPendingApproval() (string, error) {
	if !hisar.IsMacApprovalAllowed() {
		return "[]", nil
	}
	return encodeJSON(app.ListAllPendingPhoneApproval(hisar.service))
}

// ApproveApproval asks for Face ID, then signs and sends the approval with the sheet's remember switch.
func (hisar *Hisar) ApproveApproval(requestID string, isRemembered bool) error {
	if !hisar.IsMacApprovalAllowed() {
		return errMacApprovalDisabled
	}
	if hisar.biometric == nil || !hisar.biometric.IsAvailable() {
		return errBiometricUnavailable
	}
	if err := hisar.biometric.Authenticate("approve this request from your Mac"); err != nil {
		return err
	}
	return app.AnswerPhoneApproval(hisar.service, requestID, true, isRemembered)
}

// ApproveApprovalWithPassword checks the master password instead of Face ID, then signs and sends the approval.
func (hisar *Hisar) ApproveApprovalWithPassword(requestID, password string, isRemembered bool) error {
	defer releaseMemory()
	if !hisar.IsMacApprovalAllowed() {
		return errMacApprovalDisabled
	}
	if err := app.CreateApprovalUnlocker(hisar.service, nil).VerifyPassword(password); err != nil {
		return err
	}
	return app.AnswerPhoneApproval(hisar.service, requestID, true, isRemembered)
}

// DenyApproval needs no Face ID: denying never lets anything happen.
func (hisar *Hisar) DenyApproval(requestID string) error {
	return app.AnswerPhoneApproval(hisar.service, requestID, false, false)
}

// ReconnectEvents opens a new event stream at once; the app calls it on returning to the foreground, because a stream
// from before iOS suspended the app can be dead without either side knowing.
func (hisar *Hisar) ReconnectEvents() {
	app.ReconnectEventStream(hisar.service)
}
