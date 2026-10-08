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
	lowerClass  = "abcdefghijkmnopqrstuvwxyz"
	upperClass  = "ABCDEFGHJKLMNPQRSTUVWXYZ"
	digitClass  = "23456789"
	symbolClass = "!@#$%^&*-_=+?"
)

type GeneratorConfig struct {
	Length  int
	Digits  bool
	Symbols bool
}

var DefaultGeneratorConfig = GeneratorConfig{Length: 24, Digits: true, Symbols: true}

// GeneratePassword returns a random password containing at least one character of each enabled class.
func GeneratePassword(config GeneratorConfig) (string, error) {
	allClass := []string{lowerClass, upperClass}
	if config.Digits {
		allClass = append(allClass, digitClass)
	}
	if config.Symbols {
		allClass = append(allClass, symbolClass)
	}
	if config.Length < len(allClass) || config.Length > 256 {
		return "", errors.New("item: invalid password length")
	}
	alphabet := strings.Join(allClass, "")
	for {
		password := make([]byte, config.Length)
		for i := range password {
			password[i] = alphabet[generateRandomIndex(len(alphabet))]
		}
		if containsAllClass(string(password), allClass) {
			return string(password), nil
		}
	}
}

func containsAllClass(password string, allClass []string) bool {
	for _, class := range allClass {
		if !strings.ContainsAny(password, class) {
			return false
		}
	}
	return true
}

func generateRandomIndex(limit int) int {
	randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(limit)))
	if err != nil {
		panic(err)
	}
	return int(randomIndex.Int64())
}
