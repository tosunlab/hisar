package item

import (
	"crypto/rand"
	"errors"
	"math/big"
	"strings"
)

type Overview struct {
	Title    string   `json:"title"`
	Username string   `json:"username,omitempty"`
	URLs     []string `json:"urls,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

type Field struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Concealed bool   `json:"concealed,omitempty"`
}

type Details struct {
	Password string  `json:"password,omitempty"`
	Notes    string  `json:"notes,omitempty"`
	Fields   []Field `json:"fields,omitempty"`
}

type Item struct {
	ID       string
	Revision string
	Overview Overview
	Details  Details
}

const (
	lower   = "abcdefghijkmnopqrstuvwxyz"
	upper   = "ABCDEFGHJKLMNPQRSTUVWXYZ"
	digits  = "23456789"
	symbols = "!@#$%^&*-_=+?"
)

type GenOptions struct {
	Length  int
	Digits  bool
	Symbols bool
}

var DefaultGenOptions = GenOptions{Length: 24, Digits: true, Symbols: true}

// Generate returns a random password containing at least one character of each enabled class.
func Generate(o GenOptions) (string, error) {
	classes := []string{lower, upper}
	if o.Digits {
		classes = append(classes, digits)
	}
	if o.Symbols {
		classes = append(classes, symbols)
	}
	if o.Length < len(classes) || o.Length > 256 {
		return "", errors.New("item: invalid password length")
	}
	all := strings.Join(classes, "")
	for {
		b := make([]byte, o.Length)
		for i := range b {
			b[i] = all[randInt(len(all))]
		}
		if containsAll(string(b), classes) {
			return string(b), nil
		}
	}
}

func containsAll(s string, classes []string) bool {
	for _, c := range classes {
		if !strings.ContainsAny(s, c) {
			return false
		}
	}
	return true
}

func randInt(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		panic(err)
	}
	return int(v.Int64())
}
