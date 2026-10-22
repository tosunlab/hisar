//go:build js && wasm

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"syscall/js"
	"time"

	"github.com/google/uuid"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/emergencykit"
	"github.com/tryhisar/hisar/core/session"
)

type recoveryMetadata struct {
	OperationID string   `json:"operationId"`
	Kind        string   `json:"kind"`
	Origin      string   `json:"origin"`
	AccountID   string   `json:"accountId"`
	Email       string   `json:"email"`
	Name        string   `json:"name"`
	Phase       string   `json:"phase"`
	KitSaved    bool     `json:"kitSaved"`
	Revision    int64    `json:"revision"`
	Problems    []string `json:"problems"`
}

var transientRecoveryFactors sync.Map

type recoveryValue struct {
	metadata recoveryMetadata
	envelope string
}

func awaitStorage(name string, arguments ...any) (js.Value, error) {
	callback := js.Global().Get(name)
	if callback.Type() != js.TypeFunction {
		return js.Undefined(), errors.New("durable recovery storage is unavailable; no remote setup change was sent")
	}
	type answer struct {
		value js.Value
		err   error
	}
	settled := make(chan answer, 1)
	success := js.FuncOf(func(_ js.Value, values []js.Value) any {
		value := js.Undefined()
		if len(values) > 0 {
			value = values[0]
		}
		settled <- answer{value: value}
		return nil
	})
	failure := js.FuncOf(func(_ js.Value, _ []js.Value) any {
		settled <- answer{err: errors.New("durable recovery storage could not finish")}
		return nil
	})
	defer success.Release()
	defer failure.Release()
	callback.Invoke(arguments...).Call("then", success).Call("catch", failure)
	result := <-settled
	return result.value, result.err
}

func saveRecovery(value *recoveryValue) error {
	value.metadata.Revision++
	metadata, err := toJavaScript(value.metadata)
	if err != nil {
		return err
	}
	_, err = awaitStorage("stageHisarRecovery", metadata, value.envelope)
	if err != nil {
		value.metadata.Revision--
	}
	return err
}

func prepareRecovery(email, name string, password []byte) (session.StagePendingFactor, *recoveryValue) {
	value := &recoveryValue{metadata: recoveryMetadata{OperationID: uuid.NewString(), Origin: newClient().BaseURL, Email: email, Name: name, Phase: "awaiting-kit", Problems: []string{"Save and acknowledge the Emergency Kit before creating the account. The browser does not store your Secret Key."}}}
	return func(pending session.PendingFactor) error {
		key, err := crypt.ParseSecretKey(pending.SecretKey)
		if err != nil {
			return err
		}
		value.metadata.AccountID, value.metadata.Kind = key.AccountID, pending.Kind
		stored := pending
		stored.SecretKey, stored.PriorSecretKey = "", ""
		plaintext, err := json.Marshal(stored)
		if err != nil {
			return err
		}
		defer clear(plaintext)
		envelope, err := crypt.ProtectPendingFactor(password, plaintext, value.metadata.OperationID, value.metadata.Origin, value.metadata.AccountID)
		if err != nil {
			return err
		}
		value.envelope = base64.StdEncoding.EncodeToString(envelope)
		if err := saveRecovery(value); err != nil {
			value.envelope = ""
			return err
		}
		transientRecoveryFactors.Store(value.metadata.OperationID, pending)
		return session.ErrRecoveryDeliveryRequired
	}, value
}

func loadRecovery(id string, password []byte) (recoveryValue, session.PendingFactor, error) {
	stored, err := awaitStorage("loadHisarRecovery", id)
	if err != nil {
		return recoveryValue{}, session.PendingFactor{}, err
	}
	if stored.IsNull() || stored.IsUndefined() {
		return recoveryValue{}, session.PendingFactor{}, errors.New("pending recovery was not found")
	}
	var value recoveryValue
	encoded := js.Global().Get("JSON").Call("stringify", stored.Get("metadata")).String()
	if err := json.Unmarshal([]byte(encoded), &value.metadata); err != nil {
		return value, session.PendingFactor{}, err
	}
	if value.metadata.OperationID != id || value.metadata.Origin != newClient().BaseURL {
		return value, session.PendingFactor{}, errors.New("pending recovery belongs to a different server or operation")
	}
	value.envelope = stored.Get("envelope").String()
	envelope, err := base64.StdEncoding.DecodeString(value.envelope)
	if err != nil {
		return value, session.PendingFactor{}, err
	}
	plaintext, err := crypt.OpenPendingFactor(password, envelope, id, value.metadata.Origin, value.metadata.AccountID)
	if err != nil {
		return value, session.PendingFactor{}, errors.New("the password could not open this pending recovery operation")
	}
	defer clear(plaintext)
	var pending session.PendingFactor
	if err := json.Unmarshal(plaintext, &pending); err != nil {
		return value, pending, err
	}
	if pending.SecretKey != "" {
		key, err := crypt.ParseSecretKey(pending.SecretKey)
		if err != nil || key.AccountID != value.metadata.AccountID {
			return value, session.PendingFactor{}, errors.New("pending recovery account does not match")
		}
	}
	if transient, ok := transientRecoveryFactors.Load(id); ok {
		factors := transient.(session.PendingFactor)
		pending.SecretKey, pending.PriorSecretKey = factors.SecretKey, factors.PriorSecretKey
	}
	return value, pending, nil
}

func listPendingRecovery(_ context.Context, _ []string) (js.Value, error) {
	values, err := awaitStorage("listHisarRecovery")
	if err != nil {
		return js.Undefined(), err
	}
	result := js.Global().Get("Array").New()
	for index := range values.Length() {
		result.Call("push", values.Index(index).Get("metadata"))
	}
	return result, nil
}

func resumePendingRecovery(ctx context.Context, arguments []string) (js.Value, error) {
	password := []byte(arguments[1])
	defer clear(password)
	value, pending, err := loadRecovery(arguments[0], password)
	if err != nil {
		return js.Undefined(), err
	}
	if len(arguments) > 2 && arguments[2] != "" {
		pending.SecretKey = arguments[2]
	}
	if len(arguments) > 3 && arguments[3] != "" {
		pending.PriorSecretKey = arguments[3]
	}
	if err := session.ValidatePendingFactor(password, pending); err != nil {
		return js.Undefined(), err
	}
	transientRecoveryFactors.Store(value.metadata.OperationID, pending)
	if value.metadata.Phase == "awaiting-kit" {
		return toJavaScript(signUpResult{PendingOperationID: value.metadata.OperationID, SetupState: "awaiting-kit", Problems: value.metadata.Problems})
	}
	if !value.metadata.KitSaved {
		return js.Undefined(), errors.New("save and acknowledge the Emergency Kit before committing")
	}
	value.metadata.Phase = "outcome-unknown"
	if err := saveRecovery(&value); err != nil {
		return js.Undefined(), err
	}
	client := newClient()
	unlocked, key, err := session.ResumePendingFactor(ctx, client, value.metadata.Email, password, pending, dashboardDevice)
	if err != nil {
		if session.IsRegistrationNotCommitted(err) {
			if _, removeErr := awaitStorage("removeHisarRecovery", value.metadata.OperationID, value.metadata.Revision); removeErr != nil {
				return js.Undefined(), removeErr
			}
			transientRecoveryFactors.Delete(value.metadata.OperationID)
			return js.Undefined(), errors.New(invalidSetupLinkMessage)
		}
		return toJavaScript(signUpResult{PendingOperationID: value.metadata.OperationID, SetupState: "outcome-unknown", Problems: []string{"The remote outcome is not confirmed. Keep this recovery record and resume it when connected."}})
	}
	defer unlocked.Lock()
	value.metadata.Phase, value.metadata.Problems = "delivery-pending", []string{}
	if err := unlocked.EnsurePersonalVault(ctx, pending.PersonalVaultID); err != nil {
		value.metadata.Phase = "needs-finishing"
		value.metadata.Problems = []string{"Personal vault setup could not finish. Resume setup again."}
	}
	if err := saveRecovery(&value); err != nil {
		value.metadata.Phase = "needs-finishing"
		value.metadata.Problems = append(value.metadata.Problems, "Recovery progress could not be saved.")
	}
	return toJavaScript(signUpResult{Token: client.Token, AccountID: unlocked.AccountID, DeviceID: unlocked.DeviceID, SecretKey: key.Encode(), PendingOperationID: value.metadata.OperationID, SetupState: value.metadata.Phase, Problems: value.metadata.Problems})
}

func exportPendingRecoveryEmergencyKit(_ context.Context, arguments []string) (js.Value, error) {
	password := []byte(arguments[1])
	defer clear(password)
	value, pending, err := loadRecovery(arguments[0], password)
	if err != nil {
		return js.Undefined(), err
	}
	if pending.SecretKey == "" {
		return js.Undefined(), errors.New("the unsaved Secret Key is no longer in memory; discard this uncommitted setup if no kit was saved")
	}
	if value.metadata.Phase == "outcome-unknown" {
		return js.Undefined(), errors.New("resume setup to confirm the account before saving its Emergency Kit")
	}
	document, err := emergencykit.Generate(emergencykit.Kit{Name: value.metadata.Name, Email: value.metadata.Email, Server: value.metadata.Origin, SecretKey: pending.SecretKey, CreatedAt: time.Now()})
	if err != nil {
		return js.Undefined(), err
	}
	defer clear(document)
	value.metadata.KitSaved = true
	if err := saveRecovery(&value); err != nil {
		return js.Undefined(), err
	}
	result := js.Global().Get("Uint8Array").New(len(document))
	js.CopyBytesToJS(result, document)
	return result, nil
}

func acknowledgePendingRecovery(_ context.Context, arguments []string) (js.Value, error) {
	password := []byte(arguments[1])
	defer clear(password)
	value, _, err := loadRecovery(arguments[0], password)
	if err != nil {
		return js.Undefined(), err
	}
	if value.metadata.Phase == "awaiting-kit" && value.metadata.KitSaved {
		value.metadata.Phase, value.metadata.Problems = "prepared", []string{"The Emergency Kit was acknowledged. Explicitly resume to finish account creation."}
		return js.Undefined(), saveRecovery(&value)
	}
	if value.metadata.Phase != "delivery-pending" || !value.metadata.KitSaved {
		return js.Undefined(), errors.New("finish setup and save the Emergency Kit before acknowledging delivery")
	}
	_, err = awaitStorage("removeHisarRecovery", arguments[0], value.metadata.Revision)
	if err == nil {
		transientRecoveryFactors.Delete(arguments[0])
	}
	return js.Undefined(), err
}

func discardPreparedRecovery(_ context.Context, arguments []string) (js.Value, error) {
	password := []byte(arguments[1])
	defer clear(password)
	value, _, err := loadRecovery(arguments[0], password)
	if err != nil {
		return js.Undefined(), err
	}
	if value.metadata.Phase != "awaiting-kit" && value.metadata.Phase != "prepared" {
		return js.Undefined(), errors.New("a remote request may have been sent; reconcile rather than discarding recovery")
	}
	_, err = awaitStorage("removeHisarRecovery", arguments[0], value.metadata.Revision)
	if err == nil {
		transientRecoveryFactors.Delete(arguments[0])
	}
	return js.Undefined(), err
}
