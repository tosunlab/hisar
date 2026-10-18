//go:build js && wasm

// Command hisar-admin-wasm is the admin dashboard's crypto: Hisar's own sign-in and re-authentication, compiled to
// WebAssembly and run in the dashboard's Web Worker. It sets globalThis.hisar, whose calls each return a Promise.
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

func main() {
	hisar := js.Global().Get("Object").New()
	hisar.Set("signIn", exportCall(signIn))
	hisar.Set("reauthenticate", exportCall(reauthenticate))
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

// reauthenticate(token, email, secretKey, password) returns the proof that admin changes to accounts carry.
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
