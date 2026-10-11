package vault

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/sharelink"
)

var shareLinkURLPattern = regexp.MustCompile(`https?://\S+/s/([A-Za-z0-9_-]+)#\S+`)

// checkShareLinkThroughApp creates links with the built hisar through the app, then opens them the way a recipient's hisar does.
func checkShareLinkThroughApp(t *testing.T, approver *unlockingApprover, binaryPath, socketPath, statePath string) {
	t.Helper()
	ctx := context.Background()
	run := func(input string, allArg ...string) string {
		t.Helper()
		output, err := runCommandLineBinary(t, binaryPath, socketPath, statePath, input, allArg...)
		if err != nil {
			t.Fatalf("hisar %s: %v\n%s", strings.Join(allArg, " "), err, output)
		}
		return output
	}
	approvalCount := approver.getRequestCount()
	output := run("", "share-link", "github", "--field", "password", "--views", "2", "--expires", "1d")
	match := shareLinkURLPattern.FindStringSubmatch(output)
	if match == nil || !strings.Contains(output, `Share link for "GitHub" (password)`) {
		t.Fatalf("share-link:\n%s", output)
	}
	if count := approver.getRequestCount(); count != approvalCount+1 {
		t.Errorf("a share link in an approved session asked %d times", count-approvalCount)
	}
	approver.mutex.Lock()
	description := approver.allRequest[len(approver.allRequest)-1].ShareLink
	approver.mutex.Unlock()
	if description == nil || description.Title != "GitHub" || !slices.Equal(description.AllFieldLabel, []string{"password"}) || description.Detail != "Expires in 1 day · 2 views" {
		t.Errorf("approval description %+v", description)
	}

	link, shareLinkID := match[0], match[1]
	fetched, err := sharelink.Fetch(ctx, nil, link)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := fetched.Open(nil)
	if err != nil || payload.Title != "GitHub" || len(payload.Fields) != 1 || payload.Fields[0].Value != "from-the-command-line" {
		t.Fatalf("opened %+v %v", payload, err)
	}
	if output := run("", "share-links"); !strings.Contains(output, shareLinkID) || !strings.Contains(output, "1/2") || strings.Contains(output, link) {
		t.Errorf("share-links:\n%s", output)
	}
	run("", "share-link", "revoke", shareLinkID)
	if _, err := sharelink.Fetch(ctx, nil, link); !errors.Is(err, sharelink.ErrGone) {
		t.Errorf("revoked link: %v", err)
	}

	output = run("correct horse battery\ncorrect horse battery\n", "share-link", "github", "--passphrase")
	match = shareLinkURLPattern.FindStringSubmatch(output)
	if match == nil {
		t.Fatalf("share-link --passphrase:\n%s", output)
	}
	fetched, err = sharelink.Fetch(ctx, nil, match[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fetched.Open([]byte("wrong horse battery")); !errors.Is(err, sharelink.ErrWrongKey) {
		t.Errorf("wrong passphrase: %v", err)
	}
	if payload, err := fetched.Open([]byte("correct horse battery")); err != nil || len(payload.Fields) != 2 || payload.Fields[0].Value != "alice" {
		t.Errorf("passphrase link %+v %v", payload, err)
	}
	if _, err := sharelink.Fetch(ctx, nil, match[0]); !errors.Is(err, sharelink.ErrGone) {
		t.Errorf("a one-view link opened twice: %v", err)
	}
}
