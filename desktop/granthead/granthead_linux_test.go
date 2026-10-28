package granthead

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/app"
	"github.com/zalando/go-keyring"
)

// useSecretService makes a test use the session's Secret Service when HISAR_TEST_SECRET_SERVICE=1 (CI's desktop-linux
// job starts GNOME Keyring for it), and go-keyring's in-memory one otherwise.
func useSecretService(t *testing.T) {
	if os.Getenv("HISAR_TEST_SECRET_SERVICE") != "1" {
		keyring.MockInit()
	}
}

func TestAgentGrantHeadRoundtrip(t *testing.T) {
	useSecretService(t)
	service := fmt.Sprintf("Hisar agent grants test.%d", time.Now().UnixNano())
	heads := New(service)
	t.Cleanup(func() { heads.DeleteAgentGrantHead() })

	if _, err := heads.LoadAgentGrantHead(); !errors.Is(err, app.ErrNoAgentGrantHead) {
		t.Fatalf("load before save: %v", err)
	}
	first := bytes.Repeat([]byte{7}, 32)
	if err := heads.SaveAgentGrantHead(first); err != nil {
		t.Fatal(err)
	}
	if loaded, err := heads.LoadAgentGrantHead(); err != nil || !bytes.Equal(loaded, first) {
		t.Fatalf("load: %x %v", loaded, err)
	}
	second := bytes.Repeat([]byte{9}, 32)
	if err := heads.SaveAgentGrantHead(second); err != nil {
		t.Fatal(err)
	}
	if loaded, err := heads.LoadAgentGrantHead(); err != nil || !bytes.Equal(loaded, second) {
		t.Errorf("save did not replace the head: %x %v", loaded, err)
	}
	if err := heads.DeleteAgentGrantHead(); err != nil {
		t.Fatal(err)
	}
	if _, err := heads.LoadAgentGrantHead(); !errors.Is(err, app.ErrNoAgentGrantHead) {
		t.Errorf("load after delete: %v", err)
	}
	if err := heads.DeleteAgentGrantHead(); err != nil {
		t.Errorf("delete of a missing head: %v", err)
	}
}

// Another program may overwrite the head with anything: it then matches no sealed head, so the saved grants are dropped,
// and the next save replaces it.
func TestAHeadAnotherProgramRewroteIsNotTheSavedOne(t *testing.T) {
	useSecretService(t)
	service := fmt.Sprintf("Hisar agent grants test.%d", time.Now().UnixNano())
	heads := New(service)
	t.Cleanup(func() { heads.DeleteAgentGrantHead() })
	saved := bytes.Repeat([]byte{7}, 32)
	if err := heads.SaveAgentGrantHead(saved); err != nil {
		t.Fatal(err)
	}
	other := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32))
	for _, rewritten := range []string{"not base64", other} {
		if err := keyring.Set(service, account, rewritten); err != nil {
			t.Fatal(err)
		}
		if loaded, err := heads.LoadAgentGrantHead(); err != nil || bytes.Equal(loaded, saved) {
			t.Errorf("load of the head rewritten as %q: %x %v", rewritten, loaded, err)
		}
	}
	if err := heads.SaveAgentGrantHead(saved); err != nil {
		t.Fatal(err)
	}
	if loaded, err := heads.LoadAgentGrantHead(); err != nil || !bytes.Equal(loaded, saved) {
		t.Errorf("load after saving over the rewritten head: %x %v", loaded, err)
	}
}

// Without a Secret Service go-keyring reports D-Bus errors; each one keeps no head, so the menu offers only the
// choices that end with the session.
func TestWithoutASecretServiceNoHeadIsKept(t *testing.T) {
	keyring.MockInitWithError(errors.New("The name org.freedesktop.secrets was not provided by any .service files"))
	heads := New("Hisar agent grants test")
	if _, err := heads.LoadAgentGrantHead(); !errors.Is(err, app.ErrAgentGrantHeadUnavailable) {
		t.Errorf("load: %v", err)
	}
	if err := heads.SaveAgentGrantHead(bytes.Repeat([]byte{7}, 32)); !errors.Is(err, app.ErrAgentGrantHeadUnavailable) {
		t.Errorf("save: %v", err)
	}
	if err := heads.DeleteAgentGrantHead(); !errors.Is(err, app.ErrAgentGrantHeadUnavailable) {
		t.Errorf("delete: %v", err)
	}
}
