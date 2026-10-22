package main

import (
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/session"
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
