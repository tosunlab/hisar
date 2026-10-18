package serverupdate

import (
	"context"
	"net/http"
	"testing"

	"github.com/tryhisar/hisar/server/internal/desktopupdate"
	"github.com/tryhisar/hisar/server/internal/desktopupdate/fakegithub"
)

func startSource(t *testing.T, fake *fakegithub.Server) *Source {
	t.Helper()
	source, err := New(desktopupdate.GitHubConfig{APIURL: fake.URL, Repository: fake.Repository, Token: fake.Token})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func TestSourcePicksTheNewestPublishedServerRelease(t *testing.T) {
	fake := fakegithub.Start(t, fakegithub.DefaultRepository, "token")
	for _, release := range []fakegithub.Release{
		{TagName: "server-0.1.9"},
		{TagName: "server-0.1.10"},
		{TagName: "desktop-9.0.0"},
		{TagName: "cli-9.0.0"},
		{TagName: "server-0.1.11-custom"},
		{TagName: "server-0.2.0", IsDraft: true},
		{TagName: "server-0.3.0", IsPrerelease: true},
		{TagName: "server-latest"},
	} {
		fake.AddRelease(release)
	}
	source := startSource(t, fake)
	if newer := source.GetNewerVersion("0.1.0"); newer != "" {
		t.Errorf("before the first read: %q", newer)
	}
	if err := source.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	for current, expected := range map[string]string{"0.1.9": "0.1.10", "0.1.10": "", "0.2.0": "", "0.1.10-custom": "0.1.10", "dev": "", "sha-1234abcd": ""} {
		if newer := source.GetNewerVersion(current); newer != expected {
			t.Errorf("GetNewerVersion(%q) = %q, expected %q", current, newer, expected)
		}
	}

	fake.LimitRate(http.StatusTooManyRequests, 60)
	if err := source.refresh(context.Background()); err == nil {
		t.Fatal("a rate-limited read succeeded")
	}
	if newer := source.GetNewerVersion("0.1.9"); newer != "0.1.10" {
		t.Errorf("a failed read dropped the version read before: %q", newer)
	}
}

func TestSourceWithoutServerReleases(t *testing.T) {
	fake := fakegithub.Start(t, fakegithub.DefaultRepository, "")
	fake.AddRelease(fakegithub.Release{TagName: "desktop-0.1.0"})
	source := startSource(t, fake)
	if err := source.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if newer := source.GetNewerVersion("0.0.1"); newer != "" {
		t.Errorf("GetNewerVersion without server releases: %q", newer)
	}
}

func TestNewIsOff(t *testing.T) {
	if source, err := New(desktopupdate.GitHubConfig{Repository: desktopupdate.Off}); source != nil || err != nil {
		t.Errorf("New with off: %v, %v", source, err)
	}
	if _, err := New(desktopupdate.GitHubConfig{Repository: "not a repository"}); err == nil {
		t.Error("New accepted an invalid repository")
	}
}
