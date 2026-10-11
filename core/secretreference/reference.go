// Package secretreference parses, encodes and resolves hisar://<vault>/<item>/<field> secret references.
package secretreference

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const Scheme = "hisar://"

var ErrNotReference = errors.New("a secret reference starts with hisar://")

type Reference struct {
	Vault string
	Item  string
	Field string
}

func IsReference(value string) bool {
	return len(value) >= len(Scheme) && strings.EqualFold(value[:len(Scheme)], Scheme)
}

func Parse(text string) (Reference, error) {
	if !IsReference(text) {
		return Reference{}, ErrNotReference
	}
	allSegment := strings.Split(text[len(Scheme):], "/")
	if len(allSegment) != 3 {
		return Reference{}, fmt.Errorf("a secret reference has three parts, hisar://<vault>/<item>/<field>; this one has %d", len(allSegment))
	}
	allDecoded := make([]string, len(allSegment))
	for index, segment := range allSegment {
		decoded, err := decodeSegment(segment)
		if err != nil {
			return Reference{}, err
		}
		if strings.TrimSpace(decoded) == "" {
			return Reference{}, fmt.Errorf("the %s of a secret reference is empty", []string{"vault", "item", "field"}[index])
		}
		allDecoded[index] = decoded
	}
	return Reference{Vault: allDecoded[0], Item: allDecoded[1], Field: allDecoded[2]}, nil
}

func (reference Reference) String() string {
	return Scheme + EncodeSegment(reference.Vault) + "/" + EncodeSegment(reference.Item) + "/" + EncodeSegment(reference.Field)
}

func (reference Reference) Describe() string {
	return reference.Vault + " › " + reference.Item + " › " + reference.Field
}

func isKeptByte(character byte) bool {
	return 'a' <= character && character <= 'z' || 'A' <= character && character <= 'Z' || '0' <= character && character <= '9' ||
		character == '-' || character == '.' || character == '_' || character == '~'
}

// EncodeSegment keeps unreserved ASCII and every non-ASCII character, and percent-encodes every other byte.
func EncodeSegment(segment string) string {
	const hexDigit = "0123456789ABCDEF"
	var builder strings.Builder
	for index := 0; index < len(segment); index++ {
		character := segment[index]
		if isKeptByte(character) || character >= utf8.RuneSelf {
			builder.WriteByte(character)
			continue
		}
		builder.WriteByte('%')
		builder.WriteByte(hexDigit[character>>4])
		builder.WriteByte(hexDigit[character&0x0f])
	}
	return builder.String()
}

func decodeHexDigit(character byte) (byte, bool) {
	switch {
	case '0' <= character && character <= '9':
		return character - '0', true
	case 'a' <= character && character <= 'f':
		return character - 'a' + 10, true
	case 'A' <= character && character <= 'F':
		return character - 'A' + 10, true
	}
	return 0, false
}

func decodeSegment(segment string) (string, error) {
	if !strings.Contains(segment, "%") {
		if !utf8.ValidString(segment) {
			return "", errors.New("a secret reference must be UTF-8")
		}
		return segment, nil
	}
	decoded := make([]byte, 0, len(segment))
	for index := 0; index < len(segment); index++ {
		if segment[index] != '%' {
			decoded = append(decoded, segment[index])
			continue
		}
		if index+2 >= len(segment) {
			return "", fmt.Errorf("%q is not a valid percent-encoding", segment[index:])
		}
		high, isHighValid := decodeHexDigit(segment[index+1])
		low, isLowValid := decodeHexDigit(segment[index+2])
		if !isHighValid || !isLowValid {
			return "", fmt.Errorf("%q is not a valid percent-encoding", segment[index:index+3])
		}
		decoded = append(decoded, high<<4|low)
		index += 2
	}
	if !utf8.Valid(decoded) {
		return "", errors.New("a secret reference must decode to UTF-8")
	}
	return string(decoded), nil
}
