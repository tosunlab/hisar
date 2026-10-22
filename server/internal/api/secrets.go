package api

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/bytemare/opaque"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/pake"
)

type Secrets struct {
	OpaqueKeyMaterial []byte `json:"opaqueKeyMaterial"`
	HMACKey           []byte `json:"hmacKey"`
}

func LoadOrCreateSecrets(path string, allowCreate bool) (*Secrets, error) {
	fileContent, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		if !allowCreate {
			return nil, fmt.Errorf("%s is missing but the database has accounts or enrolled server secrets; restore the matching database and hisar-secrets.json backup together; do not generate replacement keys or delete account data", path)
		}
		generated := generateSecrets()
		encoded, err := json.MarshalIndent(generated, "", "  ")
		if err != nil {
			return nil, err
		}
		file, err := os.CreateTemp(filepath.Dir(path), ".hisar-secrets-*")
		if err != nil {
			return nil, err
		}
		defer os.Remove(file.Name())
		if _, err := file.Write(encoded); err != nil {
			file.Close()
			return nil, err
		}
		if err := file.Sync(); err != nil {
			file.Close()
			return nil, err
		}
		if err := file.Close(); err != nil {
			return nil, err
		}
		// Link publishes only a complete file and never replaces another start's keys.
		if err := os.Link(file.Name(), path); err != nil && !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		directory, err := os.Open(filepath.Dir(path))
		if err != nil {
			return nil, err
		}
		err = directory.Sync()
		directory.Close()
		if err != nil {
			return nil, err
		}
		return LoadOrCreateSecrets(path, false)
	}
	if err != nil {
		return nil, err
	}
	var secrets Secrets
	if err := json.Unmarshal(fileContent, &secrets); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(secrets.HMACKey) != 32 {
		return nil, fmt.Errorf("%s: incomplete secrets", path)
	}
	if _, err := pake.NewConfig().DecodeServerKeyMaterial(secrets.OpaqueKeyMaterial); err != nil {
		return nil, fmt.Errorf("%s: invalid OPAQUE server secrets: %w", path, err)
	}
	return &secrets, nil
}

func (secrets *Secrets) fingerprint() []byte {
	return crypt.ServerSecretsFingerprint(secrets.OpaqueKeyMaterial, secrets.HMACKey)
}

func generateSecrets() *Secrets {
	opaqueConfiguration := pake.NewConfig()
	privateKey, publicKey := opaqueConfiguration.KeyGen()
	serverKeyMaterial := &opaque.ServerKeyMaterial{
		PrivateKey:     privateKey,
		PublicKeyBytes: publicKey.Encode(),
		OPRFGlobalSeed: opaqueConfiguration.GenerateOPRFSeed(),
		Identity:       pake.ServerIdentity(),
	}
	hmacKey := make([]byte, 32)
	rand.Read(hmacKey)
	return &Secrets{OpaqueKeyMaterial: serverKeyMaterial.Encode(), HMACKey: hmacKey}
}
