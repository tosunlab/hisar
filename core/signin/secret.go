package signin

import (
	"errors"
	"fmt"
	"io"
)

const concealed = "‹concealed by hisar›"

var errSecretMarshal = errors.New("signin: a secret cannot be serialized")

// Secret is a value Hisar enters into a page. Every fmt verb prints it as "‹concealed by hisar›", and it refuses to
// marshal; only Reveal gives the value, for the Driver handing it to the browser.
type Secret struct {
	// value is a pointer so that fmt, which prints an unexported Secret field without calling its methods, shows an
	// address instead of the text.
	value *string
}

func NewSecret(value string) Secret {
	return Secret{value: &value}
}

func (secret Secret) Reveal() string {
	if secret.value == nil {
		return ""
	}
	return *secret.value
}

func (secret Secret) IsZero() bool {
	return secret.Reveal() == ""
}

func (Secret) String() string { return concealed }

func (Secret) GoString() string { return concealed }

func (Secret) Format(state fmt.State, _ rune) { io.WriteString(state, concealed) }

func (Secret) MarshalJSON() ([]byte, error) { return nil, errSecretMarshal }

func (Secret) MarshalText() ([]byte, error) { return nil, errSecretMarshal }

// Login is what Run may enter. Username is not secret: search_items shows it.
type Login struct {
	Username string
	Password Secret
	// OneTimePassword is the otpauth:// URI; Run generates the code when a code step shows, never before.
	OneTimePassword Secret
}
