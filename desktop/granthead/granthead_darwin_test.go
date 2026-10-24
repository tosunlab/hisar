//go:build darwin && cgo

package granthead

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/app"
)

func TestAgentGrantHeadRoundtrip(t *testing.T) {
	service := fmt.Sprintf("Hisar agent grants test.%d", time.Now().UnixNano())
	t.Logf("Keychain test service: %s", service)
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
