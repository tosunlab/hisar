package session

import (
	"github.com/bytemare/opaque"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/pake"
)

// opaqueLogin is the client side of one OPAQUE login: start sends KE1, finish answers the server's KE2 with KE3.
type opaqueLogin struct {
	client *opaque.Client
}

// startOpaqueLogin returns the login and its serialized KE1; the caller clears the login.
func startOpaqueLogin(authKey []byte) (*opaqueLogin, []byte, error) {
	client, err := pake.CreateConfig().Client()
	if err != nil {
		return nil, nil, err
	}
	keyExchange1, err := client.GenerateKE1(authKey)
	if err != nil {
		client.ClearState()
		return nil, nil, err
	}
	return &opaqueLogin{client: client}, keyExchange1.Serialize(), nil
}

// finish returns the serialized KE3; a KE2 that does not verify means the password or Secret Key is wrong.
func (login *opaqueLogin) finish(keyExchange2 []byte, accountID string) ([]byte, error) {
	decoded, err := login.client.Deserialize.KE2(keyExchange2)
	if err != nil {
		return nil, ErrBadCredentials
	}
	keyExchange3, _, _, err := login.client.GenerateKE3(decoded, []byte(accountID), pake.ServerIdentity())
	if err != nil {
		return nil, ErrBadCredentials
	}
	return keyExchange3.Serialize(), nil
}

func (login *opaqueLogin) clear() {
	login.client.ClearState()
}

// registerOpaque encrypts the keyset and runs OPAQUE registration through register, which sends the request and returns the server's response.
func (credential *accountCredential) registerOpaque(derivedKeys *crypt.DerivedKeys, accountID string, register func([]byte) ([]byte, error)) error {
	var err error
	if credential.encryptedKeyset, err = credential.keyset.Encrypt(derivedKeys.AccountUnlockKey, accountID); err != nil {
		return err
	}
	opaqueClient, err := pake.CreateConfig().Client()
	if err != nil {
		return err
	}
	defer opaqueClient.ClearState()
	registrationRequest, err := opaqueClient.RegistrationInit(derivedKeys.AuthKey)
	if err != nil {
		return err
	}
	encodedResponse, err := register(registrationRequest.Serialize())
	if err != nil {
		return err
	}
	registrationResponse, err := opaqueClient.Deserialize.RegistrationResponse(encodedResponse)
	if err != nil {
		return err
	}
	record, _, err := opaqueClient.RegistrationFinalize(registrationResponse, []byte(accountID), pake.ServerIdentity())
	if err != nil {
		return err
	}
	credential.record = record.Serialize()
	return nil
}
