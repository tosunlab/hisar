package main

import (
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/core/snapshot"
)

func TestCLIListsHealthyRecordsWithExplicitUnreadableIssues(t *testing.T) {
	app := startFakeApp(t)
	app.mutex.Lock()
	app.answer = func(request cliprotocol.Request) (any, *cliprotocol.ResponseError) {
		if request.Type == cliprotocol.TypeReadItem {
			return cliprotocol.ItemListResult{Items: []cliprotocol.Item{{ID: "healthy", Title: "Still available"}}, Issues: []session.ReadIssue{{VaultID: "personal-id", ItemID: "bad", Revision: "r1", Kind: "item", CanDelete: true}}}, nil
		}
		return answerFakeVaultRequest(request)
	}
	app.mutex.Unlock()
	stdout, stderr, err := runCapturingOutput(t, false, "", "ls")
	if err != nil || !strings.Contains(stdout, "Still available") || !strings.Contains(stderr, "Partial view") || !strings.Contains(stderr, "item=bad revision=r1") {
		t.Fatalf("partial CLI view stdout=%q stderr=%q err=%v", stdout, stderr, err)
	}
}

func TestCLISaysWhenAVaultNeverFinishedSyncing(t *testing.T) {
	stderrBySync := map[bool]string{}
	for _, hasCompletedSync := range []bool{false, true} {
		app := startFakeApp(t)
		app.mutex.Lock()
		app.answer = func(request cliprotocol.Request) (any, *cliprotocol.ResponseError) {
			if request.Type == cliprotocol.TypeReadItem {
				provenance := session.ReadProvenance{Source: "cached", Checkpoint: snapshot.Checkpoint{HasCompletedSync: hasCompletedSync}}
				return cliprotocol.ItemListResult{Items: []cliprotocol.Item{{ID: "cached", Title: "Cached item"}}, Provenance: &provenance}, nil
			}
			return answerFakeVaultRequest(request)
		}
		app.mutex.Unlock()
		stdout, stderr, err := runCapturingOutput(t, false, "", "ls")
		if err != nil || !strings.Contains(stdout, "Cached item") {
			t.Fatalf("completed sync %v: stdout=%q stderr=%q err=%v", hasCompletedSync, stdout, stderr, err)
		}
		stderrBySync[hasCompletedSync] = stderr
	}
	if notice, _ := strings.CutPrefix(stderrBySync[false], stderrBySync[true]); notice == "" || notice == stderrBySync[false] {
		t.Fatalf("listing a vault that never finished syncing added no notice: unsynced=%q synced=%q", stderrBySync[false], stderrBySync[true])
	}
}

func TestCLIRefusesLegacyAppForIssueAwareListings(t *testing.T) {
	app := startFakeApp(t)
	app.mutex.Lock()
	app.hello.Features = nil
	app.mutex.Unlock()
	_, _, err := runCapturingOutput(t, false, "", "ls")
	if err == nil || !strings.Contains(err.Error(), "update the Hisar app") {
		t.Fatalf("legacy app silently accepted: %v", err)
	}
}
