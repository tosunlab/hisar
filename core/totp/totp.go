// Package totp generates time-based one-time passwords (RFC 6238) from otpauth:// URIs or bare secrets.
package totp

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrInvalid = errors.New("totp: not a valid one-time password secret or otpauth:// link")

type Config struct {
	Secret    []byte
	Algorithm string
	Digits    int
	Period    int
	Issuer    string
	Account   string
}

const (
	defaultDigits = 6
	defaultPeriod = 30
	maxPeriod     = 300
)

// Parse reads an otpauth://totp URI or a bare base32 secret (spaces and lower case allowed).
func Parse(text string) (Config, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Config{}, ErrInvalid
	}
	if !strings.HasPrefix(strings.ToLower(text), "otpauth://") {
		secret, err := decodeSecret(text)
		if err != nil {
			return Config{}, err
		}
		return Config{Secret: secret, Algorithm: "SHA1", Digits: defaultDigits, Period: defaultPeriod}, nil
	}
	parsedURL, err := url.Parse(text)
	if err != nil || !strings.EqualFold(parsedURL.Host, "totp") {
		return Config{}, ErrInvalid
	}
	query, err := readQuery(parsedURL.RawQuery)
	if err != nil {
		return Config{}, err
	}
	secret, err := decodeSecret(query.Get("secret"))
	if err != nil {
		return Config{}, err
	}
	config := Config{Secret: secret, Algorithm: strings.ToUpper(query.Get("algorithm")), Digits: defaultDigits, Period: defaultPeriod, Issuer: query.Get("issuer")}
	if config.Algorithm == "" {
		config.Algorithm = "SHA1"
	}
	if config.Algorithm != "SHA1" && config.Algorithm != "SHA256" && config.Algorithm != "SHA512" {
		return Config{}, ErrInvalid
	}
	if digits := query.Get("digits"); digits != "" {
		if config.Digits, err = strconv.Atoi(digits); err != nil || config.Digits < 6 || config.Digits > 8 {
			return Config{}, ErrInvalid
		}
	}
	if period := query.Get("period"); period != "" {
		if config.Period, err = strconv.Atoi(period); err != nil || config.Period < 1 || config.Period > maxPeriod {
			return Config{}, ErrInvalid
		}
	}
	label := strings.TrimPrefix(parsedURL.Path, "/")
	if issuer, account, isFound := strings.Cut(label, ":"); isFound {
		config.Account = strings.TrimSpace(account)
		if config.Issuer == "" {
			config.Issuer = strings.TrimSpace(issuer)
		}
	} else {
		config.Account = label
	}
	return config, nil
}

// parameterNames are the query parameters Parse reads.
var parameterNames = map[string]bool{"secret": true, "algorithm": true, "digits": true, "period": true, "issuer": true}

// readQuery reads an otpauth:// query like url.ParseQuery, except that one of parameterNames that is repeated or does
// not decode makes the link invalid: url.ParseQuery drops such a pair, and the default, such as SHA1, would stand in.
func readQuery(rawQuery string) (url.Values, error) {
	query := url.Values{}
	for _, pair := range strings.Split(rawQuery, "&") {
		rawKey, rawValue, _ := strings.Cut(pair, "=")
		key, keyErr := url.QueryUnescape(rawKey)
		value, valueErr := url.QueryUnescape(rawValue)
		if keyErr != nil || valueErr != nil || strings.Contains(pair, ";") {
			for _, part := range strings.Split(pair, ";") {
				partKey, _, _ := strings.Cut(part, "=")
				decodedKey, err := url.QueryUnescape(partKey)
				if err != nil {
					for index := 0; index < len(partKey); index++ {
						if partKey[index] != '%' {
							continue
						}
						if index+2 >= len(partKey) ||
							!strings.ContainsRune("0123456789abcdefABCDEF", rune(partKey[index+1])) ||
							!strings.ContainsRune("0123456789abcdefABCDEF", rune(partKey[index+2])) {
							decodedKey, _ = url.QueryUnescape(partKey[:index])
							break
						}
						index += 2
					}
				}
				if parameterNames[decodedKey] {
					return nil, ErrInvalid
				}
			}
			continue
		}
		if parameterNames[key] && query.Has(key) {
			return nil, ErrInvalid
		}
		query.Add(key, value)
	}
	return query, nil
}

func decodeSecret(text string) ([]byte, error) {
	normalized := strings.ToUpper(strings.NewReplacer(" ", "", "-", "", "\t", "").Replace(text))
	normalized = strings.TrimRight(normalized, "=")
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(normalized)
	if err != nil || len(secret) < 10 {
		return nil, ErrInvalid
	}
	return secret, nil
}

// Normalize turns pasted input into the otpauth:// URI stored with the item.
func Normalize(text string) (string, error) {
	config, err := Parse(text)
	if err != nil {
		return "", err
	}
	return config.FormatURI(), nil
}

func (config Config) FormatURI() string {
	label := config.Account
	if config.Issuer != "" {
		label = config.Issuer + ":" + config.Account
	}
	query := url.Values{"secret": {base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(config.Secret)}}
	if config.Issuer != "" {
		query.Set("issuer", config.Issuer)
	}
	if config.Algorithm != "SHA1" {
		query.Set("algorithm", config.Algorithm)
	}
	if config.Digits != defaultDigits {
		query.Set("digits", strconv.Itoa(config.Digits))
	}
	if config.Period != defaultPeriod {
		query.Set("period", strconv.Itoa(config.Period))
	}
	return (&url.URL{Scheme: "otpauth", Host: "totp", Path: "/" + label, RawQuery: query.Encode()}).String()
}

// GenerateCode returns the code for now and the time it stops being valid.
func (config Config) GenerateCode(now time.Time) (code string, expiresAt time.Time) {
	period := int64(config.Period)
	counter := now.Unix() / period
	return config.generateCodeForCounter(uint64(counter)), time.Unix((counter+1)*period, 0)
}

func (config Config) generateCodeForCounter(counter uint64) string {
	var newHash func() hash.Hash
	switch config.Algorithm {
	case "SHA256":
		newHash = sha256.New
	case "SHA512":
		newHash = sha512.New
	default:
		newHash = sha1.New
	}
	mac := hmac.New(newHash, config.Secret)
	binary.Write(mac, binary.BigEndian, counter)
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	modulus := uint32(1)
	for range config.Digits {
		modulus *= 10
	}
	return fmt.Sprintf("%0*d", config.Digits, value%modulus)
}
