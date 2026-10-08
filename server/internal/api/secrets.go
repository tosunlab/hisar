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

func LoadOrCreateSecrets(path string) (*Secrets, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		sec := newSecrets()
		b, err := json.MarshalIndent(sec, "", "  ")
		if err != nil {
			return nil, err
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		if _, err := f.Write(b); err != nil {
			return nil, err
		}
		return sec, f.Sync()
	}
	if err != nil {
		return nil, err
	}
	var sec Secrets
	if err := json.Unmarshal(b, &sec); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(sec.OpaqueKeyMaterial) == 0 || len(sec.HMACKey) != 32 {
		return nil, fmt.Errorf("%s: incomplete secrets", path)
	}
	return &sec, nil
}

func newSecrets() *Secrets {
	c := pake.Config()
	sk, pk := c.KeyGen()
	skm := &opaque.ServerKeyMaterial{
		PrivateKey:     sk,
		PublicKeyBytes: pk.Encode(),
		OPRFGlobalSeed: c.GenerateOPRFSeed(),
		Identity:       pake.ServerIdentity,
	}
	h := make([]byte, 32)
	rand.Read(h)
	return &Secrets{OpaqueKeyMaterial: skm.Encode(), HMACKey: h}
}
