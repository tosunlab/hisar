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

func TestSignUpFinishingAfterSignOutKeepsRecoveryKeyWithoutOpeningSession(t *testing.T) {
	s := createUnlockedTestService(t).service
	server := startRegistrationServerWithVaults(t)
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
	if _, err := s.SignOut(); err != nil {
		t.Fatal(err)
	}
	close(paused.resume)
	finished := <-done
	if finished.err == nil {
		t.Fatal("late registration did not report its interruption")
	}
	if s.GetStatus().State != StateSignedOut || finished.result.Status.State != StateSignedOut {
		t.Fatal("late registration reopened a session")
	}
	if finished.result.SecretKey == "" {
		t.Fatal("created account lost its recovery key")
	}
	if !strings.Contains(finished.err.Error(), finished.result.SecretKey) || strings.Contains(finished.err.Error(), "try again") {
		t.Fatal("interrupted registration did not give recoverable instructions")
	}
	key, err := crypt.ParseSecretKey(finished.result.SecretKey)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := s.config.SecretKeyStore.LoadSecretKey(key.AccountID)
	if err != nil || stored.Encode() != key.Encode() {
		t.Fatalf("created account's recovery key was not saved: %v", err)
	}
}
