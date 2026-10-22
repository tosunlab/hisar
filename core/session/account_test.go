package session

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
)

func TestStrongerArgon2ParamsNeverLowersTheCost(t *testing.T) {
	stronger := crypt.Argon2Params{Time: 40, MemoryKiB: 128 * 1024, Threads: 2}
	if params := strongerArgon2Params(stronger); params != (crypt.Argon2Params{Time: 40, MemoryKiB: 128 * 1024, Threads: 4}) {
		t.Errorf("stronger account: %+v", params)
	}
	weaker := crypt.Argon2Params{Time: 2, MemoryKiB: 32 * 1024, Threads: 1}
	if params := strongerArgon2Params(weaker); params != crypt.DefaultArgon2Params() {
		t.Errorf("weaker account: %+v", params)
	}
	if params := strongerArgon2Params(crypt.Argon2Params{Time: 1 << 30}); params != crypt.DefaultArgon2Params() {
		t.Errorf("out of range: %+v", params)
	}
}

func TestWrongCurrentPasswordSendsNothing(t *testing.T) {
	password := []byte("current master password")
	secretKey, _ := crypt.GenerateSecretKey("ABC123")
	salt := crypt.GenerateSalt()
	derivedKeys, err := crypt.DeriveKeys(password, salt, crypt.MinArgon2Params(), secretKey)
	if err != nil {
		t.Fatal(err)
	}
	keyset, _ := crypt.GenerateKeyset()
	encryptedKeyset, _ := keyset.Encrypt(derivedKeys.AccountUnlockKey, "ABC123")
	account := apitypes.Account{AccountID: "ABC123", Email: "a@example.com", Argon2: crypt.MinArgon2Params(), Salt: salt, EncKeyset: encryptedKeyset, PublicKeys: keyset.GetPublicKeys("ABC123")}
	var reauthenticationCount atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/account", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		json.NewEncoder(writer).Encode(account)
	})
	mux.HandleFunc("/api/v1/account/", func(writer http.ResponseWriter, request *http.Request) {
		reauthenticationCount.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	signedIn := &Session{Client: apiclient.New(server.URL), AccountID: "ABC123", Email: "a@example.com", Keyset: keyset}
	ctx := context.Background()

	if _, err := signedIn.ChangeMasterPassword(ctx, []byte("wrong master password"), []byte("a new master password"), secretKey); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("change with a wrong password: %v", err)
	}
	if _, err := signedIn.ReplaceSecretKey(ctx, []byte("wrong master password"), secretKey); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("replace with a wrong password: %v", err)
	}
	if err := signedIn.DeleteAccount(ctx, []byte("wrong master password"), secretKey, "a@example.com", "reviewed-plan"); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("delete with a wrong password: %v", err)
	}
	if err := signedIn.DeleteAccount(ctx, password, secretKey, "b@example.com", "reviewed-plan"); !errors.Is(err, ErrEmailMismatch) {
		t.Errorf("delete with another email: %v", err)
	}
	otherSecretKey, _ := crypt.GenerateSecretKey("XYZ789")
	if _, err := signedIn.ReplaceSecretKey(ctx, password, otherSecretKey); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("another account's Secret Key: %v", err)
	}
	if _, err := Reauthenticate(ctx, signedIn.Client, "a@example.com", []byte("wrong master password"), secretKey); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("reauthenticate with a wrong password: %v", err)
	}
	if _, err := Reauthenticate(ctx, signedIn.Client, "b@example.com", password, secretKey); !errors.Is(err, ErrEmailMismatch) {
		t.Errorf("reauthenticate with another email: %v", err)
	}
	if _, err := Reauthenticate(ctx, signedIn.Client, "a@example.com", password, otherSecretKey); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("reauthenticate with another account's Secret Key: %v", err)
	}
	if count := reauthenticationCount.Load(); count != 0 {
		t.Errorf("%d requests reached the server after a failed local check", count)
	}
}

func TestAccountChangesRefuseAServerKeysetThatIsNotTheSessions(t *testing.T) {
	password := []byte("current master password")
	secretKey, _ := crypt.GenerateSecretKey("ABC123")
	salt := crypt.GenerateSalt()
	derivedKeys, err := crypt.DeriveKeys(password, salt, crypt.MinArgon2Params(), secretKey)
	if err != nil {
		t.Fatal(err)
	}
	serverKeyset, _ := crypt.GenerateKeyset()
	encryptedKeyset, _ := serverKeyset.Encrypt(derivedKeys.AccountUnlockKey, "ABC123")
	account := apitypes.Account{AccountID: "ABC123", Email: "a@example.com", Argon2: crypt.MinArgon2Params(), Salt: salt, EncKeyset: encryptedKeyset, PublicKeys: serverKeyset.GetPublicKeys("ABC123")}
	var reauthenticationCount atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/account", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		json.NewEncoder(writer).Encode(account)
	})
	mux.HandleFunc("/api/v1/account/", func(writer http.ResponseWriter, request *http.Request) {
		reauthenticationCount.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	sessionKeyset, _ := crypt.GenerateKeyset()
	signedIn := &Session{Client: apiclient.New(server.URL), AccountID: "ABC123", Email: "a@example.com", Keyset: sessionKeyset}
	ctx := context.Background()

	if err := signedIn.DeleteAccount(ctx, password, secretKey, "a@example.com", "reviewed-plan"); !errors.Is(err, ErrKeyMismatch) {
		t.Errorf("delete with the server's keyset swapped: %v", err)
	}
	if _, err := signedIn.ChangeMasterPassword(ctx, password, []byte("a new master password"), secretKey); !errors.Is(err, ErrKeyMismatch) {
		t.Errorf("change the master password with the server's keyset swapped: %v", err)
	}
	if _, err := signedIn.ReplaceSecretKey(ctx, password, secretKey); !errors.Is(err, ErrKeyMismatch) {
		t.Errorf("replace the Secret Key with the server's keyset swapped: %v", err)
	}
	if count := reauthenticationCount.Load(); count != 0 {
		t.Errorf("%d requests reached the server after the keyset check failed", count)
	}
}

type accountSaveFailingCache struct {
	Cache
}

func (accountSaveFailingCache) SaveAccount(*apitypes.Account) error { return errors.New("disk full") }

func TestCredentialChangeReportsACachedAccountItCouldNotReplace(t *testing.T) {
	account := createCredentialTestAccount(t, []byte("current master password"))
	ctx := context.Background()
	testCache := openTestCache(t)
	if err := testCache.SaveAccount(&account.server.account); err != nil {
		t.Fatal(err)
	}
	account.session.Cache = testCache
	change, err := account.session.ChangeMasterPassword(ctx, account.password, []byte("the new master password"), account.secretKey)
	if err != nil {
		t.Fatal(err)
	}
	clear(change.AccountUnlockKey)
	if change.IsCachedAccountStale {
		t.Error("a saved account reported stale")
	}
	if cached, _ := testCache.LoadAccount(account.session.AccountID); cached == nil || string(cached.EncKeyset) != string(account.server.account.EncKeyset) {
		t.Error("the cache kept the keyset of the old master password")
	}

	account.session.Cache = accountSaveFailingCache{testCache}
	change, err = account.session.ChangeMasterPassword(ctx, []byte("the new master password"), []byte("a third master password"), account.secretKey)
	if err != nil {
		t.Fatal(err)
	}
	clear(change.AccountUnlockKey)
	if !change.IsCachedAccountStale {
		t.Error("a cache that still holds the old keyset was not reported")
	}
}

func TestReplacingTheSecretKeySucceedsWhenOnlyTheAnswerWasLost(t *testing.T) {
	account := createCredentialTestAccount(t, []byte("current master password"))
	ctx := context.Background()
	account.server.isUnavailable = true
	if _, err := account.session.ReplaceSecretKey(ctx, account.password, account.secretKey); !apiclient.IsUnreachable(err) || account.server.changeCount != 0 {
		t.Errorf("a change the server never made: %v, %d changes", err, account.server.changeCount)
	}
	account.server.isUnavailable, account.server.isResponseLost = false, true
	change, err := account.session.ReplaceSecretKey(ctx, account.password, account.secretKey)
	if err != nil || account.server.changeCount != 1 {
		t.Fatalf("a change the server made but whose answer was lost: %v, %d changes", err, account.server.changeCount)
	}
	clear(change.AccountUnlockKey)
	if change.SignedOutDeviceCount != -1 {
		t.Fatalf("a lost response manufactured a known device count: %d", change.SignedOutDeviceCount)
	}
	if change.SecretKey.Encode() == account.secretKey.Encode() {
		t.Fatal("the Secret Key did not change")
	}
	account.server.isResponseLost = false
	if _, err := DeriveAccountUnlockKey(ctx, account.session.Client, nil, account.password, change.SecretKey); err != nil {
		t.Errorf("the new Secret Key does not open the account: %v", err)
	}
}

func TestSignUpCompletesWhenOnlyTheRegistrationAnswerWasLost(t *testing.T) {
	ctx := context.Background()
	password := []byte("a new master password")
	server, apiClient := startCredentialTestServer(t)
	server.isUnavailable = true
	if signedUp, secretKey, err := SignUp(ctx, apiClient, "invite", "a@example.com", "Alice", password, apitypes.DeviceInfo{}); signedUp != nil || !apiclient.IsUnreachable(err) || secretKey.AccountID == "" {
		t.Errorf("a registration the server never made: %+v %v", signedUp, err)
	}
	server.isUnavailable, server.isResponseLost = false, true
	signedUp, secretKey, err := SignUp(ctx, apiClient, "invite", "a@example.com", "Alice", password, apitypes.DeviceInfo{})
	if err != nil || signedUp == nil || signedUp.Client.Token == "" {
		t.Fatalf("a registration the server made but whose answer was lost: %+v %v", signedUp, err)
	}
	defer signedUp.Lock()
	if signedUp.AccountID != server.account.AccountID || secretKey.AccountID != signedUp.AccountID || strings.Join(server.vaultTypes, ",") != apitypes.VaultPersonal {
		t.Errorf("account %s, key for %s, vaults %v", signedUp.AccountID, secretKey.AccountID, server.vaultTypes)
	}
}
