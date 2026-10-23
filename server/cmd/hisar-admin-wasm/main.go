//go:build js && wasm

// Command hisar-admin-wasm is the admin dashboard's crypto: Hisar's own sign-in, re-authentication and the first admin's
// sign-up, compiled to WebAssembly and run in the dashboard's Web Worker. It sets globalThis.hisar, whose calls each
// return a Promise.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"syscall/js"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/emergencykit"
	"github.com/tryhisar/hisar/core/session"
)

var dashboardDevice = apitypes.DeviceInfo{Name: "Admin dashboard", Platform: "web/admin"}

type signInResult struct {
	Token     string `json:"token"`
	AccountID string `json:"accountId"`
	DeviceID  string `json:"deviceId"`
	Role      string `json:"role"`
	Name      string `json:"name"`
}

// signUpResult carries the Secret Key even when the Personal vault or the Emergency Kit failed, because the account
// exists by then and the page must show the key. The kit itself is set on the result as a Uint8Array.
type signUpResult struct {
	Token              string `json:"token"`
	AccountID          string `json:"accountId"`
	DeviceID           string `json:"deviceId"`
	SecretKey          string `json:"secretKey"`
	PersonalVaultError string `json:"personalVaultError,omitempty"`
	EmergencyKitError  string `json:"emergencyKitError,omitempty"`
}

const invalidSetupLinkMessage = "This setup link is no longer valid. Use the link in the latest server log."

func main() {
	hisar := js.Global().Get("Object").New()
	hisar.Set("signIn", exportCall(signIn))
	hisar.Set("reauthenticate", exportCall(reauthenticate))
	hisar.Set("signUp", exportCall(signUp))
	js.Global().Set("hisar", hisar)
	select {}
}

// exportCall turns call into a JavaScript function that returns a Promise and runs call in a goroutine, because
// net/http waits on fetch, which needs the JavaScript event loop to keep running.
func exportCall(call func(ctx context.Context, arguments []string) (js.Value, error)) js.Func {
	return js.FuncOf(func(_ js.Value, values []js.Value) any {
		arguments := make([]string, len(values))
		for index, value := range values {
			arguments[index] = value.String()
		}
		executor := js.FuncOf(func(_ js.Value, settle []js.Value) any {
			resolve, reject := settle[0], settle[1]
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				result, err := call(ctx, arguments)
				if err != nil {
					reject.Invoke(js.Global().Get("Error").New(describeError(err)))
					return
				}
				resolve.Invoke(result)
			}()
			return nil
		})
		defer executor.Release()
		return js.Global().Get("Promise").New(executor)
	})
}

func describeError(err error) string {
	switch {
	case errors.Is(err, session.ErrBadCredentials):
		return "incorrect email, master password or Secret Key"
	case apiclient.IsStatus(err, http.StatusTooManyRequests):
		return "too many attempts; wait a minute and try again"
	case apiclient.IsUnreachable(err):
		return "cannot reach the Hisar server"
	}
	return err.Error()
}

func toJavaScript(value any) (js.Value, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return js.Undefined(), err
	}
	return js.Global().Get("JSON").Call("parse", string(encoded)), nil
}

func newClient() *apiclient.Client {
	return apiclient.New(js.Global().Get("location").Get("origin").String())
}

func parseSecretKey(text string) (crypt.SecretKey, error) {
	secretKey, err := crypt.ParseSecretKey(strings.TrimSpace(text))
	if err != nil {
		return crypt.SecretKey{}, errors.New("the Secret Key is not valid; check it against your Emergency Kit")
	}
	return secretKey, nil
}

// signIn(email, secretKey, password) signs in as the dashboard device and keeps only the session token.
func signIn(ctx context.Context, arguments []string) (js.Value, error) {
	secretKey, err := parseSecretKey(arguments[1])
	if err != nil {
		return js.Undefined(), err
	}
	password := []byte(arguments[2])
	defer clear(password)
	signedIn, err := session.LogIn(ctx, newClient(), strings.TrimSpace(arguments[0]), password, secretKey, dashboardDevice)
	if err != nil {
		return js.Undefined(), err
	}
	signedIn.Lock()
	return toJavaScript(signInResult{Token: signedIn.Client.Token, AccountID: signedIn.AccountID, DeviceID: signedIn.DeviceID, Role: signedIn.Role, Name: signedIn.Name})
}

// reauthenticate(token, email, secretKey, password) returns a re-authentication proof for the signed-in account.
func reauthenticate(ctx context.Context, arguments []string) (js.Value, error) {
	secretKey, err := parseSecretKey(arguments[2])
	if err != nil {
		return js.Undefined(), err
	}
	password := []byte(arguments[3])
	defer clear(password)
	client := newClient()
	client.Token = arguments[0]
	proof, err := session.Reauthenticate(ctx, client, arguments[1], password, secretKey)
	if err != nil {
		return js.Undefined(), err
	}
	return toJavaScript(proof)
}

// signUp(inviteToken, email, name, password) signs the first admin up with the setup invite as the dashboard device and
// makes the Emergency Kit. Once the account exists, which the client's token shows, it always answers the Secret Key:
// with personalVaultError when the Personal vault failed and emergencyKitError when the kit failed, because the page is
// then the only place the key can be read.
func signUp(ctx context.Context, arguments []string) (js.Value, error) {
	inviteToken, email, name := arguments[0], strings.TrimSpace(arguments[1]), strings.TrimSpace(arguments[2])
	password := []byte(arguments[3])
	defer clear(password)
	client := newClient()
	signedUp, secretKey, err := session.SignUp(ctx, client, inviteToken, email, name, password, dashboardDevice)
	if err != nil && client.Token == "" {
		if apiclient.IsStatus(err, http.StatusForbidden) {
			return js.Undefined(), errors.New(invalidSetupLinkMessage)
		}
		return js.Undefined(), err
	}
	signedUp.Lock()
	created := signUpResult{Token: client.Token, AccountID: signedUp.AccountID, DeviceID: signedUp.DeviceID, SecretKey: secretKey.Encode()}
	if err != nil {
		created.PersonalVaultError = describeError(err)
	}
	document, err := emergencykit.Generate(emergencykit.Kit{Name: name, Email: email, Server: client.BaseURL, SecretKey: created.SecretKey, CreatedAt: time.Now()})
	if err != nil {
		created.EmergencyKitError = err.Error()
	}
	result, err := toJavaScript(created)
	if err != nil {
		return js.Undefined(), err
	}
	if document != nil {
		emergencyKit := js.Global().Get("Uint8Array").New(len(document))
		js.CopyBytesToJS(emergencyKit, document)
		clear(document)
		result.Set("emergencyKitPdf", emergencyKit)
	}
	return result, nil
}
