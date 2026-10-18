package localstate

import (
	"errors"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestKeyringErrorWithoutASecretServiceSaysWhatToDo(t *testing.T) {
	dbusError := errors.New("The name org.freedesktop.secrets was not provided by any .service files")
	err := classifyKeyringError(dbusError, func() error { return errors.New("no Secret Service") })
	if !errors.Is(err, ErrNoSecretService) || !strings.Contains(err.Error(), "HISAR_SECRET_KEY") || !strings.Contains(err.Error(), "GNOME Keyring") {
		t.Fatalf("error = %v", err)
	}
	if err := classifyKeyringError(dbusError, func() error { return nil }); err != dbusError {
		t.Fatalf("with a Secret Service running, error = %v", err)
	}
	if err := classifyKeyringError(keyring.ErrNotFound, func() error { return errors.New("no Secret Service") }); err != keyring.ErrNotFound {
		t.Fatalf("a missing key became %v", err)
	}
	if err := classifyKeyringError(nil, func() error { t.Fatal("checked without an error"); return nil }); err != nil {
		t.Fatal(err)
	}
}
