package session

import (
	"bytes"
	"context"
	"errors"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
)

// PendingFactor contains recovery material. Persist it only inside authenticated encryption.
// It deliberately contains neither a master password nor a derived account unlock key.
type PendingFactor struct {
	Kind                 string                             `json:"kind"`
	SecretKey            string                             `json:"secretKey"`
	PriorSecretKey       string                             `json:"priorSecretKey,omitempty"`
	EncryptedKeyset      []byte                             `json:"encryptedKeyset"`
	PriorEncryptedKeyset []byte                             `json:"priorEncryptedKeyset,omitempty"`
	Registration         *apitypes.RegisterFinishRequest    `json:"registration,omitempty"`
	Credentials          *apitypes.ChangeCredentialsRequest `json:"credentials,omitempty"`
	// PersonalVaultID is the ID a sign-up gives its Personal vault, so finishing it again never creates a second one.
	PersonalVaultID string `json:"personalVaultId,omitempty"`
}

type StagePendingFactor func(PendingFactor) error

var ErrRecoveryDeliveryRequired = errors.New("save and acknowledge the Emergency Kit before committing this operation")

func ValidatePendingFactor(password []byte, pending PendingFactor) error {
	secretKey, err := crypt.ParseSecretKey(pending.SecretKey)
	if err != nil {
		return errors.New("enter the Secret Key from this operation's saved Emergency Kit")
	}
	var salt []byte
	var params crypt.Argon2Params
	if pending.Registration != nil {
		if pending.Registration.AccountID != secretKey.AccountID {
			return ErrKeyMismatch
		}
		salt, params = pending.Registration.Salt, pending.Registration.Argon2
	} else if pending.Credentials != nil {
		salt, params = pending.Credentials.Salt, pending.Credentials.Argon2
	} else {
		return errors.New("this pending operation has no prepared credential request")
	}
	keys, err := crypt.DeriveKeys(password, salt, params, secretKey)
	if err != nil {
		return err
	}
	defer keys.Wipe()
	keyset, err := crypt.DecryptKeyset(keys.AccountUnlockKey, secretKey.AccountID, pending.EncryptedKeyset)
	if err != nil {
		return errors.New("the password and saved Secret Key do not open this exact prepared operation; nothing was sent")
	}
	defer keyset.Wipe()
	if pending.Registration != nil {
		actual := keyset.GetPublicKeys(secretKey.AccountID)
		if !bytes.Equal(actual.Sign, pending.Registration.PublicKeys.Sign) || !bytes.Equal(actual.KEM, pending.Registration.PublicKeys.KEM) {
			return ErrKeyMismatch
		}
	}
	return nil
}

// ResumePendingFactor proves the exact staged outcome before returning an installable session.
// Retrying the same registration or ciphertext never generates a different factor.
func ResumePendingFactor(ctx context.Context, client *apiclient.Client, email string, password []byte, pending PendingFactor, device apitypes.DeviceInfo) (*Session, crypt.SecretKey, error) {
	secretKey, err := crypt.ParseSecretKey(pending.SecretKey)
	if err != nil {
		return nil, secretKey, err
	}
	if err := ValidatePendingFactor(password, pending); err != nil {
		return nil, secretKey, err
	}
	login := func() (*Session, error) {
		unlocked, err := LogIn(ctx, client, email, password, secretKey, device)
		if err != nil {
			return nil, err
		}
		if unlocked.verifiedAccount == nil || !bytes.Equal(unlocked.verifiedAccount.EncKeyset, pending.EncryptedKeyset) {
			unlocked.Lock()
			return nil, errors.New("the account has another credential change; keep this pending recovery record and review the account")
		}
		return unlocked, nil
	}
	unlockCommitted := func() (*Session, error) {
		unlocked, err := Unlock(ctx, client, nil, password, secretKey)
		if err != nil {
			return nil, err
		}
		if unlocked.verifiedAccount == nil || !bytes.Equal(unlocked.verifiedAccount.EncKeyset, pending.EncryptedKeyset) {
			unlocked.Lock()
			return nil, errors.New("the account has another credential change; keep this pending recovery record and review the account")
		}
		return unlocked, nil
	}
	if client.Token != "" {
		account, err := client.GetAccount(ctx)
		if err == nil && account.AccountID == secretKey.AccountID && bytes.Equal(account.EncKeyset, pending.EncryptedKeyset) {
			unlocked, err := unlockCommitted()
			if err == nil {
				return unlocked, secretKey, nil
			}
		}
	}
	if unlocked, err := login(); err == nil {
		return unlocked, secretKey, nil
	}
	if pending.Registration != nil {
		response, err := client.FinishRegistration(ctx, pending.Registration)
		if err == nil {
			client.Token = response.Token
			unlocked, unlockErr := Unlock(ctx, client, nil, password, secretKey)
			if unlockErr != nil {
				return nil, secretKey, unlockErr
			}
			if !bytes.Equal(unlocked.verifiedAccount.EncKeyset, pending.EncryptedKeyset) {
				unlocked.Lock()
				return nil, secretKey, ErrKeyMismatch
			}
			unlocked.DeviceID = response.DeviceID
			return unlocked, secretKey, nil
		}
		if unlocked, loginErr := login(); loginErr == nil {
			return unlocked, secretKey, nil
		}
		return nil, secretKey, err
	} else if pending.Credentials != nil {
		if _, err := client.ChangeCredentials(ctx, pending.Credentials); err == nil {
			unlocked, loginErr := unlockCommitted()
			if loginErr != nil {
				unlocked, loginErr = login()
			}
			return unlocked, secretKey, loginErr
		}
		oldKey, err := crypt.ParseSecretKey(pending.PriorSecretKey)
		if err != nil {
			return nil, secretKey, err
		}
		var oldSession *Session
		if client.Token != "" {
			oldSession, err = Unlock(ctx, client, nil, password, oldKey)
		}
		if oldSession == nil {
			oldSession, err = LogIn(ctx, client, email, password, oldKey, device)
		}
		if err != nil {
			return nil, secretKey, err
		}
		defer oldSession.Lock()
		if oldSession.verifiedAccount == nil || !bytes.Equal(oldSession.verifiedAccount.EncKeyset, pending.PriorEncryptedKeyset) {
			return nil, secretKey, errors.New("the account changed after this operation was prepared; keep its Emergency Kit and review the current account")
		}
		proof, err := reauthenticate(ctx, client, email, password, oldKey, oldSession.Keyset)
		if err != nil {
			return nil, secretKey, err
		}
		request := *pending.Credentials
		request.ReauthenticationID, request.KE3 = proof.ReauthenticationID, proof.KE3
		if _, err := client.ChangeCredentials(ctx, &request); err != nil && !oldSession.holdsKeyset(ctx, pending.EncryptedKeyset) {
			return nil, secretKey, err
		}
	} else {
		return nil, secretKey, errors.New("this pending operation cannot be resumed")
	}
	unlocked, err := unlockCommitted()
	if err != nil {
		unlocked, err = login()
	}
	return unlocked, secretKey, err
}

func IsRegistrationNotCommitted(err error) bool {
	var refusal *apiclient.APIError
	return errors.As(err, &refusal) && refusal.Message == apitypes.RegistrationNotCommittedMessage
}
