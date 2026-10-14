// Package crypt holds every cryptographic operation of Hisar and all AAD labels, so the formats can be reviewed
// in one place: XChaCha20-Poly1305 sealing, Argon2id and HKDF key derivation, the keyset (ML-KEM-768 + X25519
// HPKE, Ed25519), vault keys, items, approvals, share links, exports and the Secret Key. vectors_test.go freezes
// the formats.
package crypt
