package api

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/bytemare/opaque"
	"github.com/tryhisar/hisar/core/pake"
)

type Secrets struct {
	OpaqueKeyMaterial []byte `json:"opaqueKeyMaterial"`
	HMACKey           []byte `json:"hmacKey"`
}

func LoadOrCreateAllSecret(path string) (*Secrets, error) {
	fileContent, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		generated := generateAllSecret()
		encoded, err := json.MarshalIndent(generated, "", "  ")
		if err != nil {
			return nil, err
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		if _, err := file.Write(encoded); err != nil {
			return nil, err
		}
		return generated, file.Sync()
	}
	if err != nil {
		return nil, err
	}
	var secrets Secrets
	if err := json.Unmarshal(fileContent, &secrets); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(secrets.OpaqueKeyMaterial) == 0 || len(secrets.HMACKey) != 32 {
		return nil, fmt.Errorf("%s: incomplete secrets", path)
	}
	return &secrets, nil
}

func generateAllSecret() *Secrets {
	opaqueConfiguration := pake.CreateConfig()
	privateKey, publicKey := opaqueConfiguration.KeyGen()
	serverKeyMaterial := &opaque.ServerKeyMaterial{
		PrivateKey:     privateKey,
		PublicKeyBytes: publicKey.Encode(),
		OPRFGlobalSeed: opaqueConfiguration.GenerateOPRFSeed(),
		Identity:       pake.ServerIdentity,
	}
	hmacKey := make([]byte, 32)
	rand.Read(hmacKey)
	return &Secrets{OpaqueKeyMaterial: serverKeyMaterial.Encode(), HMACKey: hmacKey}
}
