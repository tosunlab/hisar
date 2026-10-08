package pake

import "github.com/bytemare/opaque"

// ServerIdentity must be non-empty: bytemare/opaque v0.18.0 cannot decode key material with an empty identity.
var ServerIdentity = []byte("hisar")

// Config is the OPAQUE configuration shared by client and server.
// Key stretching is disabled because the input is already the Argon2id-derived auth key.
func Config() *opaque.Configuration {
	c := opaque.DefaultConfiguration()
	c.KSF = 0
	c.Context = []byte("hisar/v1/opaque")
	return c
}
