package app

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cache"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/session"
)

func TestAccountSwitchWithUndeletableCacheNeverUsesThePreviousAccount(t *testing.T) {
	for _, method := range []string{"SignIn", "SignUp"} {
		t.Run(method, func(t *testing.T) {
			fixture := createUnlockedTestService(t)
			service := fixture.service
			service.config.IsEventStreamDisabled = true
			directory := filepath.Join(t.TempDir(), "offline")
			if err := os.Mkdir(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, "cache.db")
			service.config.CachePath = path
			previousCache, err := cache.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := previousCache.SaveAccount(&apitypes.Account{AccountID: fixture.accountID, Email: "previous@example.com"}); err != nil {
				t.Fatal(err)
			}
			fixture.apiServer.mutex.Lock()
			err = previousCache.ReplaceVaults(fixture.apiServer.vaults, nil)
			fixture.apiServer.mutex.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			previousCache.Close()
			if err := os.Chmod(directory, 0o500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.Chmod(directory, 0o700) })
			password := "new master password"
			if method == "SignIn" {
				secretKey, err := crypt.GenerateSecretKey("ABC123")
				if err != nil {
					t.Fatal(err)
				}
				server := startRefusingServer(t, secretKey.AccountID, []byte(password), secretKey, "")
				if _, err := service.SignIn(server, "new@example.com", secretKey.Encode(), password); err != nil {
					t.Fatal(err)
				}
			} else {
				server := startRegistrationServerWithVaults(t)
				result, err := service.SignUp(server+"/invite/token", "new@example.com", "New", password)
				if err != nil || result.SecretKey == "" || result.Status.State != StateUnlocked {
					t.Fatalf("the cache failure lost the new account's Secret Key ceremony: %+v %v", result, err)
				}
			}
			service.mutex.Lock()
			newAccountID := service.state.AccountID
			attached := service.unlockedSession.Cache
			service.mutex.Unlock()
			if attached != nil {
				t.Error("the new account attached the old account's cache")
			}
			service.Lock()
			reopened, _, _ := createTestService(t, service.config.StatePath, service.config.SecretKeyStore)
			reopened.config.CachePath = path
			reopened.config.IsEventStreamDisabled = true
			t.Cleanup(func() { reopened.Lock() })
			if status, err := reopened.Unlock(password); err != nil || status.State != StateUnlocked {
				t.Fatalf("unlocking the new account after reopening: %+v %v", status, err)
			}
			reopened.mutex.Lock()
			attached = reopened.unlockedSession.Cache
			reopened.mutex.Unlock()
			if attached != nil {
				t.Error("reopening attached the old account's cache")
			}
			vaults, err := reopened.ListVaults()
			if err != nil {
				t.Fatal(err)
			}
			for _, vault := range vaults {
				if vault.ID == fixture.personalID || vault.ID == fixture.familyID {
					t.Error("the new account showed a previous account's vault")
				}
			}
			kept, err := cache.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer kept.Close()
			if err := kept.CheckAccount(fixture.accountID); err != nil {
				t.Errorf("the previous account's snapshot changed: %v", err)
			}
			if err := kept.CheckAccount(newAccountID); !errors.Is(err, cache.ErrAccountMismatch) {
				t.Errorf("the new account was accepted by the previous snapshot: %v", err)
			}
			if previous, err := kept.LoadAccount(fixture.accountID); err != nil || previous == nil || previous.Email != "previous@example.com" {
				t.Errorf("the failed removal discarded the previous snapshot: %+v %v", previous, err)
			}
		})
	}
}

func TestCacheWithNoAccountBindingIsNotAttached(t *testing.T) {
	fixture := createUnlockedTestService(t)
	service := fixture.service
	unbound, err := cache.Open(service.config.CachePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := unbound.ReplaceVaults([]apitypes.Vault{{ID: "unbound-vault"}}, nil); err != nil {
		t.Fatal(err)
	}
	unbound.Close()
	service.mutex.Lock()
	service.attachCache(context.Background(), service.unlockedSession)
	attached := service.unlockedSession.Cache
	service.mutex.Unlock()
	if attached != nil {
		t.Error("a populated cache without account binding was attached")
	}
}

type failedAccountReadTransport struct{}

func (failedAccountReadTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Path == "/api/v1/account" {
		return nil, errors.New("account fetch failed")
	}
	return http.DefaultTransport.RoundTrip(request)
}

func TestFailedCacheAccountReadDoesNotDisableLaterOfflineUnlocks(t *testing.T) {
	fixture := createLockedUnlockableService(t)
	s := fixture.service
	s.config.IsEventStreamDisabled = true
	client := s.createClient(s.state.Server)
	client.Token = s.state.Token
	key, err := s.config.SecretKeyStore.LoadSecretKey(fixture.accountID)
	if err != nil {
		t.Fatal(err)
	}
	unlocked, err := session.Unlock(context.Background(), client, nil, []byte(testMasterPassword), key)
	if err != nil {
		t.Fatal(err)
	}
	client.HTTP = &http.Client{Transport: failedAccountReadTransport{}}
	s.mutex.Lock()
	s.attachCache(context.Background(), unlocked)
	s.openSession(unlocked)
	s.mutex.Unlock()
	if _, err := s.ListVaults(); err != nil {
		t.Fatal(err)
	}
	s.Lock()
	s.mutex.Lock()
	if s.localCache != nil {
		s.localCache.Close()
		s.localCache = nil
	}
	s.mutex.Unlock()
	for attempt := 1; attempt <= 2; attempt++ {
		if _, err := s.Unlock(testMasterPassword); err != nil {
			t.Fatal(err)
		}
		s.mutex.Lock()
		attached := s.unlockedSession.Cache != nil
		s.mutex.Unlock()
		s.Lock()
		if !attached {
			t.Errorf("unlock %d: this account's own offline copy was refused", attempt)
		}
	}
	reopened, err := cache.Open(s.config.CachePath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	account, err := reopened.LoadAccount(fixture.accountID)
	if err != nil || account == nil {
		t.Errorf("the cache did not gain its account row: %v", err)
	}
}
