module github.com/tryhisar/hisar/server

go 1.26.0

require (
	github.com/bytemare/opaque v0.18.0
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.11.0
	golang.org/x/time v0.16.0
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	filippo.io/nistec v0.0.4 // indirect
	github.com/bytemare/ecc v0.9.0 // indirect
	github.com/bytemare/hash v0.6.2 // indirect
	github.com/bytemare/hash2curve v0.5.4 // indirect
	github.com/bytemare/ksf v0.5.0 // indirect
	github.com/bytemare/secp256k1 v0.3.0 // indirect
	github.com/gtank/ristretto255 v0.2.0 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/tryhisar/hisar/core v0.0.0
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/text v0.42.0 // indirect
)

replace github.com/tryhisar/hisar/core => ../core
