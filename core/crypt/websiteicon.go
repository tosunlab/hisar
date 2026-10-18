package crypt

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

const (
	websiteIconKeyInfo = "hisar/v1/website-icon-cache"
	websiteIconLabel   = "hisar/v1/website-icon"
)

// WebsiteIconKeys seal the device's cache of website icons, which never leaves the device: SealKey seals each icon
// record and LookupKey turns a domain into the record's key, so the cache holds no domain in plain text.
type WebsiteIconKeys struct {
	SealKey   []byte
	LookupKey []byte
}

// DeriveWebsiteIconKeys derives the website icon cache keys from K_user, bound to the account ID.
func DeriveWebsiteIconKeys(userKey []byte, accountID string) (*WebsiteIconKeys, error) {
	keyMaterial, err := hkdf.Key(sha256.New, userKey, []byte(accountID), websiteIconKeyInfo, 2*KeySize)
	if err != nil {
		return nil, err
	}
	return &WebsiteIconKeys{SealKey: keyMaterial[:KeySize], LookupKey: keyMaterial[KeySize:]}, nil
}

// Lookup returns the hex HMAC-SHA256 of the domain, the cache key of its icon.
func (keys *WebsiteIconKeys) Lookup(domain string) string {
	mac := hmac.New(sha256.New, keys.LookupKey)
	mac.Write([]byte(domain))
	return hex.EncodeToString(mac.Sum(nil))
}

// Seal seals a website icon record, bound to its lookup key so a record can't be moved to another domain.
func (keys *WebsiteIconKeys) Seal(lookup string, plaintext []byte) ([]byte, error) {
	return Seal(keys.SealKey, plaintext, encodeContext(websiteIconLabel, lookup))
}

func (keys *WebsiteIconKeys) Open(lookup string, ciphertext []byte) ([]byte, error) {
	return Open(keys.SealKey, ciphertext, encodeContext(websiteIconLabel, lookup))
}
