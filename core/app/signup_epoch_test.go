package app

import (
	"net/http"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/crypt"
)

type pauseRegistrationTransport struct {
	pause *pause
}

func (transport pauseRegistrationTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := http.DefaultTransport.RoundTrip(request)
	if request.URL.Path == "/api/v1/auth/register/finish" && err == nil {
		transport.pause.wait()
	}
	return response, err
}

func TestLateSignUpNeverUndoesLockSignOutOrAccountSwitch(t *testing.T) {
	for _, intent := range []string{"lock", "sign-out", "account switch"} {
		t.Run(intent, func(t *testing.T) {
			s := createUnlockedTestService(t).service
			server, _ := startRegistrationServerWithVaults(t)
			paused := newPause()
			s.serverTransport = pauseRegistrationTransport{pause: paused}
			type outcome struct {
				result SignUpResult
				err    error
			}
			done := make(chan outcome, 1)
			go func() {
				result, err := s.SignUp(server+"/invite/token", "new@example.com", "New", testMasterPassword)
				done <- outcome{result, err}
			}()
			<-paused.reached
			switch intent {
			case "lock":
				s.Lock()
			case "sign-out":
				if _, err := s.SignOut(); err != nil {
					t.Fatal(err)
				}
			case "account switch":
				signInOtherAccount(t, s, createUnlockedTestService(t))
			}
			expected := s.GetStatus()
			if err := s.state.Save(); err != nil {
				t.Fatal(err)
			}
			close(paused.resume)
			finished := <-done
			if finished.err != nil || finished.result.PendingOperationID == "" || finished.result.SetupState != "needs-finishing" {
				t.Fatalf("late completion did not deliver a visible pending operation: phase=%s err=%v", finished.result.SetupState, finished.err)
			}
			if current := s.GetStatus(); current.State != expected.State || current.Email != expected.Email || current.Server != expected.Server {
				t.Fatalf("late signup undid explicit %s", intent)
			}
			if !strings.Contains(strings.Join(finished.result.Problems, " "), "interrupted") {
				t.Fatal("pending setup did not explain why account installation was refused")
			}
			entry, pending, err := s.loadPendingRecovery(finished.result.PendingOperationID, []byte(testMasterPassword))
			if err != nil {
				t.Fatal(err)
			}
			key, err := crypt.ParseSecretKey(pending.SecretKey)
			if err != nil || key.AccountID != entry.AccountID {
				t.Fatal("the pending factor lost its exact account identity")
			}
			if _, err := s.config.SecretKeyStore.LoadSecretKey(key.AccountID); err == nil {
				t.Fatal("late signup promoted an active Secret Key after losing its epoch")
			}
			restarted, err := NewService(s.config)
			if err != nil {
				t.Fatal(err)
			}
			if current := restarted.GetStatus(); current.Email != expected.Email || (expected.State == StateSignedOut && current.State != StateSignedOut) {
				t.Fatalf("restart undid explicit %s", intent)
			}
			kit, err := ExportPendingRecoveryEmergencyKitToDirectory(restarted, entry.OperationID, testMasterPassword, t.TempDir())
			if err != nil || kit.SecretKey != pending.SecretKey {
				t.Fatalf("protected pending kit delivery failed: %v", err)
			}
		})
	}
}
