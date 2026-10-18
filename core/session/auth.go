package session

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
)

// accountCredential is what signup generates: a new Secret Key, salt, keyset and OPAQUE record.
type accountCredential struct {
	secretKey        crypt.SecretKey
	salt             []byte
	argon2Params     crypt.Argon2Params
	keyset           *crypt.Keyset
	encryptedKeyset  []byte
	record           []byte
	accountUnlockKey []byte
}

// createAccountCredential runs OPAQUE registration through register, which sends the request and returns the server's response.
func createAccountCredential(accountID string, password []byte, register func(registrationRequest []byte) ([]byte, error)) (*accountCredential, error) {
	secretKey, err := crypt.GenerateSecretKey(accountID)
	if err != nil {
		return nil, err
	}
	credential := &accountCredential{secretKey: secretKey, salt: crypt.GenerateSalt(), argon2Params: crypt.DefaultArgon2Params()}
	derivedKeys, err := crypt.DeriveKeys(password, credential.salt, credential.argon2Params, secretKey)
	if err != nil {
		return nil, err
	}
	defer derivedKeys.Wipe()

	if credential.keyset, err = crypt.GenerateKeyset(); err != nil {
		return nil, err
	}
	if err := credential.registerOpaque(derivedKeys, accountID, register); err != nil {
		credential.keyset.Wipe()
		return nil, err
	}
	credential.accountUnlockKey = append([]byte(nil), derivedKeys.AccountUnlockKey...)
	return credential, nil
}

func SignUp(ctx context.Context, apiClient *apiclient.Client, invite, email, name string, password []byte, device apitypes.DeviceInfo) (*Session, crypt.SecretKey, error) {
	newSession, secretKey, accountUnlockKey, err := SignUpKeepingAccountUnlockKey(ctx, apiClient, invite, email, name, password, device)
	clear(accountUnlockKey)
	return newSession, secretKey, err
}

// SignUpKeepingAccountUnlockKey also returns a copy of the account unlock key; the caller wipes it.
func SignUpKeepingAccountUnlockKey(ctx context.Context, apiClient *apiclient.Client, invite, email, name string, password []byte, device apitypes.DeviceInfo) (*Session, crypt.SecretKey, []byte, error) {
	accountID := crypt.GenerateAccountID()
	credential, err := createAccountCredential(accountID, password, func(registrationRequest []byte) ([]byte, error) {
		startResponse, err := apiClient.StartRegistration(ctx, &apitypes.RegisterStartRequest{Invite: invite, Email: email, AccountID: accountID, Request: registrationRequest})
		if err != nil {
			return nil, err
		}
		return startResponse.Response, nil
	})
	if err != nil {
		return nil, crypt.SecretKey{}, nil, err
	}
	secretKey := credential.secretKey

	sessionResponse, err := apiClient.FinishRegistration(ctx, &apitypes.RegisterFinishRequest{
		Invite:     invite,
		Email:      email,
		Name:       name,
		AccountID:  accountID,
		Record:     credential.record,
		Salt:       credential.salt,
		Argon2:     credential.argon2Params,
		EncKeyset:  credential.encryptedKeyset,
		PublicKeys: credential.keyset.GetPublicKeys(accountID),
		Device:     device,
	})
	if err != nil {
		credential.keyset.Wipe()
		clear(credential.accountUnlockKey)
		return nil, secretKey, nil, err
	}
	apiClient.Token = sessionResponse.Token

	newSession := &Session{Client: apiClient, AccountID: accountID, DeviceID: sessionResponse.DeviceID, Email: email, Name: name, Keyset: credential.keyset}
	if _, err := newSession.CreateVault(ctx, "Personal", apitypes.VaultPersonal); err != nil {
		clear(credential.accountUnlockKey)
		return nil, secretKey, nil, fmt.Errorf("create personal vault: %w", err)
	}
	return newSession, secretKey, credential.accountUnlockKey, nil
}

func LogIn(ctx context.Context, apiClient *apiclient.Client, email string, password []byte, secretKey crypt.SecretKey, device apitypes.DeviceInfo) (*Session, error) {
	prelogin, err := apiClient.FetchPrelogin(ctx, email)
	if err != nil {
		return nil, err
	}
	derivedKeys, err := crypt.DeriveKeys(password, prelogin.Salt, prelogin.Argon2, secretKey)
	if err != nil {
		return nil, err
	}
	defer derivedKeys.Wipe()

	login, keyExchange1, err := startOpaqueLogin(derivedKeys.AuthKey)
	if err != nil {
		return nil, err
	}
	startResponse, err := apiClient.StartLogin(ctx, &apitypes.LoginStartRequest{Email: email, KE1: keyExchange1})
	if err != nil {
		login.clear()
		return nil, err
	}
	keyExchange3, err := login.finish(startResponse.KE2, secretKey.AccountID)
	login.clear()
	if err != nil {
		return nil, err
	}
	sessionResponse, err := apiClient.FinishLogin(ctx, &apitypes.LoginFinishRequest{LoginID: startResponse.LoginID, KE3: keyExchange3, Device: device})
	if apiclient.IsStatus(err, http.StatusUnauthorized) {
		return nil, ErrBadCredentials
	}
	if err != nil {
		return nil, err
	}
	apiClient.Token = sessionResponse.Token
	if sessionResponse.AccountID != secretKey.AccountID {
		return nil, ErrBadCredentials
	}
	signedIn, err := UnlockWithAccountUnlockKey(ctx, apiClient, nil, derivedKeys.AccountUnlockKey, secretKey.AccountID)
	if err != nil {
		return nil, err
	}
	signedIn.DeviceID = sessionResponse.DeviceID
	return signedIn, nil
}

// deriveAccountKeys fetches the account, from accountCache when the server is unreachable, and derives its keys; the caller wipes them.
func deriveAccountKeys(ctx context.Context, apiClient *apiclient.Client, accountCache Cache, password []byte, secretKey crypt.SecretKey) (*apitypes.Account, bool, *crypt.DerivedKeys, error) {
	account, isOffline, err := getAccount(ctx, apiClient, accountCache, secretKey.AccountID)
	if err != nil {
		return nil, false, nil, err
	}
	if account.AccountID != secretKey.AccountID {
		return nil, false, nil, ErrBadCredentials
	}
	derivedKeys, err := crypt.DeriveKeys(password, account.Salt, account.Argon2, secretKey)
	if err != nil {
		return nil, false, nil, err
	}
	return account, isOffline, derivedKeys, nil
}

// Unlock decrypts the keyset; with a non-nil accountCache it works offline from the cached account.
func Unlock(ctx context.Context, apiClient *apiclient.Client, accountCache Cache, password []byte, secretKey crypt.SecretKey) (*Session, error) {
	account, isOffline, derivedKeys, err := deriveAccountKeys(ctx, apiClient, accountCache, password, secretKey)
	if err != nil {
		return nil, err
	}
	defer derivedKeys.Wipe()
	return decryptAccountKeyset(account, apiClient, accountCache, isOffline, derivedKeys.AccountUnlockKey)
}

// DeriveAccountUnlockKey returns a copy of the account unlock key after checking it opens the keyset; the caller wipes it.
func DeriveAccountUnlockKey(ctx context.Context, apiClient *apiclient.Client, accountCache Cache, password []byte, secretKey crypt.SecretKey) ([]byte, error) {
	account, _, derivedKeys, err := deriveAccountKeys(ctx, apiClient, accountCache, password, secretKey)
	if err != nil {
		return nil, err
	}
	defer derivedKeys.Wipe()
	keyset, err := crypt.DecryptKeyset(derivedKeys.AccountUnlockKey, account.AccountID, account.EncKeyset)
	if err != nil {
		return nil, ErrBadCredentials
	}
	keyset.Wipe()
	return append([]byte(nil), derivedKeys.AccountUnlockKey...), nil
}

// UnlockWithAccountUnlockKey decrypts the keyset with an already derived account unlock key.
func UnlockWithAccountUnlockKey(ctx context.Context, apiClient *apiclient.Client, accountCache Cache, accountUnlockKey []byte, accountID string) (*Session, error) {
	account, isOffline, err := getAccount(ctx, apiClient, accountCache, accountID)
	if err != nil {
		return nil, err
	}
	if account.AccountID != accountID {
		return nil, ErrBadCredentials
	}
	return decryptAccountKeyset(account, apiClient, accountCache, isOffline, accountUnlockKey)
}

func decryptAccountKeyset(account *apitypes.Account, apiClient *apiclient.Client, accountCache Cache, isOffline bool, accountUnlockKey []byte) (*Session, error) {
	keyset, err := crypt.DecryptKeyset(accountUnlockKey, account.AccountID, account.EncKeyset)
	if err != nil {
		return nil, ErrBadCredentials
	}
	ownPublicKeyBundle := keyset.GetPublicKeys(account.AccountID)
	if !bytes.Equal(ownPublicKeyBundle.KEM, account.PublicKeys.KEM) || !bytes.Equal(ownPublicKeyBundle.Sign, account.PublicKeys.Sign) {
		keyset.Wipe()
		return nil, ErrKeyMismatch
	}
	unlockedSession := &Session{Client: apiClient, AccountID: account.AccountID, Email: account.Email, Name: account.Name, Role: account.Role, Keyset: keyset, Cache: accountCache}
	unlockedSession.isOffline.Store(isOffline)
	return unlockedSession, nil
}
