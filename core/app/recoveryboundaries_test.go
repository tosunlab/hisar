package app

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/session"
)

type judgeCountingTransport struct {
	base            http.RoundTripper
	vaults, changes atomic.Int32
}

func (c *judgeCountingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method == "GET" && r.URL.Path == "/api/v1/vaults" {
		c.vaults.Add(1)
	}
	if strings.HasSuffix(r.URL.Path, "/changes") {
		c.changes.Add(1)
	}
	return c.base.RoundTrip(r)
}

func TestJudgeCorruptRecordRetryIsBounded(t *testing.T) {
	f := createUnlockedTestService(t)
	if _, err := f.service.ListVaults(); err != nil {
		t.Fatal(err)
	}
	injectJudgeBadItem(t, f)
	counter := &judgeCountingTransport{base: http.DefaultTransport}
	f.service.unlockedSession.Client.HTTP = &http.Client{Transport: counter}
	if _, err := f.service.ListItems(f.personalID); err != nil {
		t.Fatal(err)
	}
	if counter.changes.Load() != 2 || counter.vaults.Load() != 1 {
		t.Fatalf("unexpected reload acquisition count, vaults=%d changes=%d", counter.vaults.Load(), counter.changes.Load())
	}
}

func TestJudgeUnreadableRotatedTargetIsNotNotFound(t *testing.T) {
	f := createUnlockedTestService(t)
	s := f.service.unlockedSession
	vaults, err := s.ListVaults(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var target *session.Vault
	for i := range vaults {
		if vaults[i].ID == f.familyID {
			target = &vaults[i]
		}
	}
	f.apiServer.mutex.Lock()
	for i := range f.apiServer.vaults {
		if f.apiServer.vaults[i].ID == f.familyID {
			f.apiServer.vaults[i].KeyGeneration = 2
			f.apiServer.vaults[i].WrappedKey = []byte("bad-key")
		}
	}
	f.apiServer.mutex.Unlock()
	_, err = s.ReadItems(context.Background(), target)
	if errors.Is(err, session.ErrNotFound) {
		t.Fatalf("reload mislabeled observed unreadable vault as authoritative not-found: %v", err)
	}
	if !errors.Is(err, session.ErrIncompleteRead) {
		t.Fatalf("unreadable target must remain incomplete: %v", err)
	}
}

func TestJudgeStrictSocketListRefusesFilteredHealthyOnly(t *testing.T) {
	f := createUnlockedTestService(t)
	injectJudgeBadItem(t, f)
	bridge := NewCommandLineBridge(f.service, clisocket.NewSessionApprovals(0, nil, nil))
	ctx := bridge.BindSession(context.Background())
	epoch := ctx.Value(sessionEpochKey{}).(uint64)
	for _, kind := range []string{cliprotocol.TypeListVault, cliprotocol.TypeListItem} {
		_, err := bridge.answer(ctx, epoch, &cliprotocol.Request{Type: kind, VaultID: f.personalID})
		if !errors.Is(err, session.ErrIncompleteRead) {
			t.Fatalf("strict %s accepted partial inventory: %v", kind, err)
		}
	}
	result, err := bridge.answer(ctx, epoch, &cliprotocol.Request{Type: cliprotocol.TypeReadItem, VaultID: f.personalID})
	if err != nil {
		t.Fatal(err)
	}
	read := result.(cliprotocol.ItemListResult)
	if len(read.Issues) != 1 || read.Issues[0].ItemID != "judge-bad" {
		t.Fatalf("typed read hid bad record: %+v", read)
	}
	f.service.Lock()
	if _, err = bridge.answer(ctx, epoch, &cliprotocol.Request{Type: cliprotocol.TypeReadItem, VaultID: f.personalID}); err == nil {
		t.Fatal("ended epoch published typed view")
	}
}
