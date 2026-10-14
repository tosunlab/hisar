package pake

import "github.com/bytemare/opaque"

// ServerIdentity must be non-empty: bytemare/opaque v0.18.0 cannot decode key material with an empty identity.
func ServerIdentity() []byte { return []byte("hisar") }

// CreateConfig returns the OPAQUE configuration shared by client and server.
// Key stretching is disabled because the input is already the Argon2id-derived auth key.
func CreateConfig() *opaque.Configuration {
	configuration := opaque.DefaultConfiguration()
	configuration.KSF = 0
	configuration.Context = []byte("hisar/v1/opaque")
	return configuration
}
