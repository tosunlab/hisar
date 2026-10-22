package app

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/session"
)

type pausedBiometricSetup struct {
	*fakeBiometric
	phase           string
	started, resume chan struct{}
	failure         error
	stored, sealed  []byte
}

func (b *pausedBiometricSetup) pause(phase string) {
	if b.phase == phase {
		close(b.started)
		<-b.resume
	}
}

func (b *pausedBiometricSetup) Authenticate(reason string) error {
	b.pause("authenticate")
	return b.fakeBiometric.Authenticate(reason)
}

func (b *pausedBiometricSetup) StoreKey(accountID string, key []byte) error {
	b.pause("store")
	b.stored = bytes.Clone(key)
	return b.fakeBiometric.StoreKey(accountID, key)
}

func (b *pausedBiometricSetup) IsKeyProtected(string) bool { return true }

func (b *pausedBiometricSetup) MigrateKey(_ string, sealed []byte) error {
	b.sealed = bytes.Clone(sealed)
	b.pause("readback")
	return b.failure
}

func prepareBiometricSetupTest(t *testing.T, isNewAccount bool) (unlockedTestService, []byte) {
	t.Helper()
	fixture := createUnlockedTestService(t)
	prepareTestApprovalUnlock(t, fixture, "correct horse battery")
	s := fixture.service
	wrapping, err := s.config.Biometric.LoadKey(fixture.accountID)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(wrapping)
	key, err := crypt.Open(wrapping, s.state.BiometricUnlock, buildBiometricContext(fixture.accountID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DisableBiometricUnlock(); err != nil {
		t.Fatal(err)
	}
	if isNewAccount {
		s.newAccountUnlockKey = bytes.Clone(key)
	}
	return fixture, key
}

func enableBiometricSetupTest(s *Service, isNewAccount bool) (Status, error) {
	if isNewAccount {
		return s.EnableNewAccountBiometricUnlock()
	}
	return s.EnableBiometricUnlock("correct horse battery")
}

func TestBiometricSetupCannotBlockLockOrPublishAcrossSessionChanges(t *testing.T) {
	for _, isNewAccount := range []bool{false, true} {
		name := "password"
		if isNewAccount {
			name = "new account"
		}
		for _, phase := range []string{"authenticate", "store", "readback"} {
			for _, transition := range []string{"lock", "switch account", "disable"} {
				t.Run(name+"/"+phase+"/"+transition, func(t *testing.T) {
					fixture, expectedKey := prepareBiometricSetupTest(t, isNewAccount)
					defer clear(expectedKey)
					s := fixture.service
					b := &pausedBiometricSetup{fakeBiometric: s.config.Biometric.(*fakeBiometric), phase: phase, started: make(chan struct{}), resume: make(chan struct{})}
					release := sync.OnceFunc(func() { close(b.resume) })
					t.Cleanup(release)
					s.config.Biometric = b
					originalKey := s.newAccountUnlockKey
					type result struct {
						status Status
						err    error
					}
					done := make(chan result, 1)
					go func() { status, err := enableBiometricSetupTest(s, isNewAccount); done <- result{status, err} }()
					select {
					case <-b.started:
					case r := <-done:
						t.Fatalf("setup did not reach %s: %+v %v", phase, r.status, r.err)
					case <-time.After(5 * time.Second):
						t.Fatal("setup never reached OS boundary")
					}
					changed := make(chan error, 1)
					var replacement unlockedTestService
					if transition == "switch account" {
						replacement = createUnlockedTestService(t)
					}
					go func() {
						switch transition {
						case "lock":
							s.Lock()
						case "disable":
							_, err := s.DisableBiometricUnlock()
							changed <- err
							return
						case "switch account":
							s.mutex.Lock()
							err := s.resetAccount()
							s.mutex.Unlock()
							if err != nil {
								changed <- err
								return
							}
							other := replacement.service
							other.mutex.Lock()
							unlocked := other.unlockedSession
							other.unlockedSession = nil
							other.mutex.Unlock()
							OpenUnlockedSession(s, other.state.Server, other.state.Token, unlocked)
						}
						changed <- nil
					}()
					select {
					case err := <-changed:
						if err != nil {
							t.Fatal(err)
						}
					case <-time.After(time.Second):
						release()
						<-done
						<-changed
						t.Fatal("logical session change waited for the OS prompt")
					}
					if transition != "disable" && !bytes.Equal(originalKey, make([]byte, len(originalKey))) {
						t.Fatal("session change did not wipe the sign-up key")
					}
					release()
					r := <-done
					if r.err == nil || r.status.BiometricEnabled || len(s.state.BiometricUnlock) != 0 {
						t.Fatalf("late setup activated: %+v %v", r.status, r.err)
					}
					if _, err := b.LoadKey(fixture.accountID); err == nil {
						t.Fatal("late setup left its key in the keychain")
					}
					if transition == "lock" && r.status.State != StateLocked {
						t.Fatalf("late setup changed locked state: %+v", r.status)
					}
					if transition == "switch account" && s.state.AccountID != replacement.accountID {
						t.Fatal("late setup replaced the new account")
					}
					if phase == "readback" && transition == "lock" {
						opened, err := crypt.Open(b.stored, b.sealed, buildBiometricContext(fixture.accountID))
						defer clear(opened)
						if err != nil || !bytes.Equal(opened, expectedKey) {
							t.Fatalf("setup raced the session's secret wipe: %v", err)
						}
					}
				})
			}
		}
	}
}

func TestBiometricSetupErrorsPreservePasswordUnlock(t *testing.T) {
	for _, isNewAccount := range []bool{false, true} {
		for _, failure := range []string{"cancel", "failed readback"} {
			name := failure
			if isNewAccount {
				name += "/new account"
			}
			t.Run(name, func(t *testing.T) {
				fixture, key := prepareBiometricSetupTest(t, isNewAccount)
				defer clear(key)
				s := fixture.service
				secretBefore, err := s.config.SecretKeyStore.LoadSecretKey(fixture.accountID)
				if err != nil {
					t.Fatal(err)
				}
				b := &pausedBiometricSetup{fakeBiometric: s.config.Biometric.(*fakeBiometric)}
				cause := errors.New("protected readback failed")
				if failure == "cancel" {
					cause = ErrBiometricCancelled
					b.setAuthenticateError(cause)
				} else {
					b.failure = cause
				}
				s.config.Biometric = b
				status, err := enableBiometricSetupTest(s, isNewAccount)
				if !errors.Is(err, cause) || status.BiometricEnabled || len(s.state.BiometricUnlock) != 0 {
					t.Fatalf("failed setup activated: %+v %v", status, err)
				}
				secretAfter, err := s.config.SecretKeyStore.LoadSecretKey(fixture.accountID)
				if err != nil || secretAfter.Encode() != secretBefore.Encode() {
					t.Fatal("setup changed the Secret Key")
				}
				s.Lock()
				status, err = s.Unlock("correct horse battery")
				if err != nil || status.State != StateUnlocked {
					t.Fatalf("password fallback: %+v %v", status, err)
				}
				b.failure = nil
				b.setAuthenticateError(nil)
				status, err = s.EnableBiometricUnlock("correct horse battery")
				if err != nil || !status.BiometricEnabled {
					t.Fatalf("retry: %+v %v", status, err)
				}
			})
		}
	}
}

func TestCredentialResealDoesNotBlockLockOrRestoreBiometricSetup(t *testing.T) {
	const password = "correct horse battery"
	fixture := createUnlockedTestService(t)
	prepareTestApprovalUnlock(t, fixture, password)
	s := fixture.service
	b := &pausedBiometricSetup{fakeBiometric: s.config.Biometric.(*fakeBiometric), phase: "readback", started: make(chan struct{}), resume: make(chan struct{})}
	release := sync.OnceFunc(func() { close(b.resume) })
	t.Cleanup(release)
	s.config.Biometric = b
	changed := make(chan CredentialChangeResult, 1)
	go func() {
		result, err := s.changeCredentials(password, func(context.Context, *session.Session, []byte, crypt.SecretKey, session.StagePendingFactor) (*session.CredentialChange, error) {
			secret, err := s.config.SecretKeyStore.LoadSecretKey(fixture.accountID)
			return &session.CredentialChange{SecretKey: secret, AccountUnlockKey: crypt.GenerateKey()}, err
		})
		if err != nil {
			result.LocalError = err.Error()
		}
		changed <- result
	}()
	select {
	case <-b.started:
	case result := <-changed:
		t.Fatalf("reseal never reached readback: %+v", result)
	case <-time.After(5 * time.Second):
		t.Fatal("reseal stalled before readback")
	}
	locked := make(chan Status, 1)
	go func() { locked <- s.Lock() }()
	select {
	case status := <-locked:
		if status.State != StateLocked {
			t.Fatalf("lock: %+v", status)
		}
	case <-time.After(time.Second):
		release()
		<-changed
		<-locked
		t.Fatal("credential reseal blocked logical lock")
	}
	release()
	result := <-changed
	if result.Status.State != StateLocked || result.Status.BiometricEnabled {
		t.Fatalf("a reseal interrupted by lock published Touch ID: %+v", result)
	}
}

type promptingKeyPreparer struct{}

func (promptingKeyPreparer) PrepareStoreKey(string) error { return nil }
func (promptingKeyPreparer) PrepareLoadKey(string) error  { return nil }

func TestInterruptedCredentialResealStillFinishesTheCommittedChange(t *testing.T) {
	const password = "correct horse battery"
	for _, phase := range []string{"authenticate", "store", "readback"} {
		for _, transition := range []string{"lock", "disable", "sign out"} {
			t.Run(phase+"/"+transition, func(t *testing.T) {
				fixture := createUnlockedTestService(t)
				prepareTestApprovalUnlock(t, fixture, password)
				s := fixture.service
				b := &pausedBiometricSetup{fakeBiometric: s.config.Biometric.(*fakeBiometric), phase: phase, started: make(chan struct{}), resume: make(chan struct{})}
				release := sync.OnceFunc(func() { close(b.resume) })
				t.Cleanup(release)
				s.config.Biometric = b
				SetBiometricKeyPreparer(s, promptingKeyPreparer{})
				changed := make(chan CredentialChangeResult, 1)
				go func() {
					result, err := s.changeCredentials(password, func(_ context.Context, _ *session.Session, _ []byte, current crypt.SecretKey, stage session.StagePendingFactor) (*session.CredentialChange, error) {
						replacement, err := crypt.GenerateSecretKey(fixture.accountID)
						if err != nil {
							return nil, err
						}
						if err := stage(session.PendingFactor{Kind: "replace-secret-key", SecretKey: replacement.Encode(), PriorSecretKey: current.Encode()}); err != nil {
							return nil, err
						}
						return &session.CredentialChange{SecretKey: replacement, AccountUnlockKey: crypt.GenerateKey()}, nil
					})
					if err != nil {
						t.Error(err)
					}
					changed <- result
				}()
				select {
				case <-b.started:
				case <-time.After(5 * time.Second):
					t.Fatal("reseal never reached the OS boundary")
				}
				switch transition {
				case "lock":
					s.Lock()
				case "disable":
					if _, err := s.DisableBiometricUnlock(); err != nil {
						t.Fatal(err)
					}
				case "sign out":
					s.SignOut()
				}
				release()
				result := <-changed
				if result.PendingOperationID == "" || result.SetupState != "delivery-pending" || result.LocalError != "" {
					t.Fatalf("the committed change was reported as unfinished: %+v", result)
				}
				entries, err := s.ListPendingRecovery()
				if err != nil || len(entries) != 1 || entries[0].OperationID != result.PendingOperationID || entries[0].Phase != "delivery-pending" {
					t.Fatalf("the recovery record does not show the committed change: %+v %v", entries, err)
				}
				saved, err := localstate.Load(s.config.StatePath)
				if err != nil {
					t.Fatal(err)
				}
				wantOperationID := result.PendingOperationID
				if transition == "sign out" {
					wantOperationID = ""
				}
				if saved.RecoveryOperationID != wantOperationID {
					t.Errorf("saved recovery operation %q, want %q", saved.RecoveryOperationID, wantOperationID)
				}
				if s.GetStatus().BiometricEnabled || len(saved.BiometricUnlock) > 0 {
					t.Error("Touch ID stayed on with a sealed state of the replaced credentials")
				}
				if _, err := b.fakeBiometric.LoadKey(fixture.accountID); err == nil {
					t.Error("a Touch ID key outlived the reseal that did not finish")
				}
			})
		}
	}
}

func TestBiometricReSetupThatDoesNotFinishKeepsKeychainAndStateTogether(t *testing.T) {
	const password = "correct horse battery"
	for _, test := range []struct {
		phase, transition string
		keepsEarlierSetup bool
	}{
		{"authenticate", "lock", true},
		{"store", "lock", false},
		{"store", "disable", false},
		{"readback", "lock", false},
		{"readback", "disable", false},
		{"", "failed readback", false},
	} {
		t.Run(test.phase+"/"+test.transition, func(t *testing.T) {
			fixture := createUnlockedTestService(t)
			prepareTestApprovalUnlock(t, fixture, password)
			s := fixture.service
			b := &pausedBiometricSetup{fakeBiometric: s.config.Biometric.(*fakeBiometric), phase: test.phase, started: make(chan struct{}), resume: make(chan struct{})}
			release := sync.OnceFunc(func() { close(b.resume) })
			t.Cleanup(release)
			if test.transition == "failed readback" {
				b.failure = errors.New("protected readback failed")
			}
			s.config.Biometric = b
			done := make(chan error, 1)
			go func() { _, err := s.EnableBiometricUnlock(password); done <- err }()
			if test.phase != "" {
				select {
				case <-b.started:
				case <-time.After(5 * time.Second):
					t.Fatal("setup never reached the OS boundary")
				}
				switch test.transition {
				case "lock":
					s.Lock()
				case "disable":
					if _, err := s.DisableBiometricUnlock(); err != nil {
						t.Fatal(err)
					}
				}
				release()
			}
			if err := <-done; err == nil {
				t.Fatal("interrupted setup reported success")
			}
			b.phase = ""
			status := s.GetStatus()
			_, loadErr := b.LoadKey(fixture.accountID)
			if status.BiometricEnabled != test.keepsEarlierSetup || (loadErr == nil) != test.keepsEarlierSetup {
				t.Fatalf("keychain and state disagree: enabled %v, keychain error %v", status.BiometricEnabled, loadErr)
			}
			if test.keepsEarlierSetup {
				if status, err := s.UnlockWithBiometric(); err != nil || status.State != StateUnlocked {
					t.Fatalf("earlier setup stopped unlocking: %+v %v", status, err)
				}
			}
		})
	}
}
