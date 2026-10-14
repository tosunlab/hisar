package session

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
	if params := strongerArgon2Params(weaker); params != crypt.DefaultArgon2Params {
		t.Errorf("weaker account: %+v", params)
	}
	if params := strongerArgon2Params(crypt.Argon2Params{Time: 1 << 30}); params != crypt.DefaultArgon2Params {
		t.Errorf("out of range: %+v", params)
	}
}

func TestWrongCurrentPasswordSendsNothing(t *testing.T) {
	password := []byte("current master password")
	secretKey, _ := crypt.GenerateSecretKey("ABC123")
	salt := crypt.GenerateSalt()
	derivedKeys, err := crypt.DeriveAllKey(password, salt, crypt.MinArgon2Params, secretKey)
	if err != nil {
		t.Fatal(err)
	}
	keyset, _ := crypt.GenerateKeyset()
	encryptedKeyset, _ := keyset.Encrypt(derivedKeys.AccountUnlockKey, "ABC123")
	account := apitypes.Account{AccountID: "ABC123", Email: "a@example.com", Argon2: crypt.MinArgon2Params, Salt: salt, EncKeyset: encryptedKeyset, PublicKeys: keyset.GetAllPublicKey("ABC123")}
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
	signedIn := &Session{Client: apiclient.Create(server.URL), AccountID: "ABC123", Email: "a@example.com", Keyset: keyset}
	ctx := context.Background()

	if _, err := signedIn.ChangeMasterPassword(ctx, []byte("wrong master password"), []byte("a new master password"), secretKey); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("change with a wrong password: %v", err)
	}
	if _, err := signedIn.ReplaceSecretKey(ctx, []byte("wrong master password"), secretKey); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("replace with a wrong password: %v", err)
	}
	if err := signedIn.DeleteAccount(ctx, []byte("wrong master password"), secretKey, "a@example.com"); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("delete with a wrong password: %v", err)
	}
	if err := signedIn.DeleteAccount(ctx, password, secretKey, "b@example.com"); !errors.Is(err, ErrEmailMismatch) {
		t.Errorf("delete with another email: %v", err)
	}
	otherSecretKey, _ := crypt.GenerateSecretKey("XYZ789")
	if _, err := signedIn.ReplaceSecretKey(ctx, password, otherSecretKey); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("another account's Secret Key: %v", err)
	}
	if count := reauthenticationCount.Load(); count != 0 {
		t.Errorf("%d requests reached the server after a failed local check", count)
	}
}
