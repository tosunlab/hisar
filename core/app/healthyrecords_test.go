package app

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
)

// createHealthyBesideUnreadable saves a GitHub login and an SSH key in Personal, then makes another record of that vault
// unreadable and loads the vaults as the apps do.
func createHealthyBesideUnreadable(t *testing.T) (f unlockedTestService, login, sshKey ItemDetail) {
	t.Helper()
	f = createUnlockedTestService(t)
	login, err := f.service.SaveItem(f.personalID, ItemDetail{
		Category: item.CategoryLogin, Title: "GitHub", Username: "alice", Password: "github-password", URLs: []string{"https://github.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sshKey, err = f.service.GenerateSSHKey(f.personalID, "Deploy key", "ed25519"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.SetVaultAvailableToAgents(f.personalID, true); err != nil {
		t.Fatal(err)
	}
	injectJudgeBadItem(t, f)
	if _, err := f.service.ListVaults(); err != nil {
		t.Fatal(err)
	}
	if issues, err := f.service.GetReadIssues(); err != nil || len(issues) != 1 {
		t.Fatalf("setup did not load the unreadable record: %+v %v", issues, err)
	}
	return f, login, sshKey
}

func TestOneUnreadableRecordLeavesAutoFillUsable(t *testing.T) {
	f, login, _ := createHealthyBesideUnreadable(t)
	s := f.service
	logins, err := ListLoginsForURL(s, "https://github.com")
	if err != nil || len(logins) != 1 || logins[0].ItemID != login.ID {
		t.Fatalf("AutoFill logins beside an unreadable record: %+v %v", logins, err)
	}
	if fill, err := GetLoginForURL(s, f.personalID, login.ID, "https://github.com"); err != nil || fill.Password != "github-password" {
		t.Fatalf("AutoFill fill beside an unreadable record: %+v %v", fill, err)
	}
	if websites, err := ListLoginWebsites(s); err != nil || !slices.ContainsFunc(websites, func(website LoginWebsite) bool { return website.Host == "github.com" }) {
		t.Fatalf("login websites beside an unreadable record: %+v %v", websites, err)
	}
	if check, err := s.checkLogin(context.Background(), "https://github.com", "alice", "github-password"); err != nil || check.Status != "unchanged" || check.ItemID != login.ID {
		t.Fatalf("a saved login was not recognized: %+v %v", check, err)
	}
	for _, username := range []string{"bob", "alice"} {
		if check, err := s.checkLogin(context.Background(), "https://github.com", username, "another-password"); !errors.Is(err, session.ErrIncompleteRead) {
			t.Errorf("the save offer for %s concluded %+v from a view missing a record: %v", username, check, err)
		}
	}
}

func TestOneUnreadableRecordLeavesLaunchersAgentsAndReferencesUsable(t *testing.T) {
	f, login, _ := createHealthyBesideUnreadable(t)
	s := f.service
	launcher, err := s.listLauncherItems()
	if err != nil {
		t.Fatalf("launcher beside an unreadable record: %v", err)
	}
	index := slices.IndexFunc(launcher.Items, func(launcherItem cliprotocol.LauncherItem) bool { return launcherItem.ID == login.ID })
	if index < 0 {
		t.Fatalf("launcher left out the healthy login: %+v", launcher.Items)
	}
	reference := ""
	for _, field := range launcher.Items[index].References {
		if field.Field == "password" {
			reference = field.Reference
		}
	}
	resolved, err := s.resolveSecrets([]string{reference})
	if err != nil || len(resolved.Values) != 1 || resolved.Values[0] != "github-password" {
		t.Fatalf("the launcher's reference %q did not resolve to the login: %+v %v", reference, resolved, err)
	}
	if resolved, err := s.resolveSecrets([]string{"hisar://Personal/GitHub/password"}); err == nil {
		t.Fatalf("a title matched in a vault whose other record may have that title: %+v", resolved)
	}
	search, err := s.searchAgentItems("github", "")
	if err != nil || search.TotalCount != 1 || search.Items[0].ID != login.ID {
		t.Fatalf("agent search beside an unreadable record: %+v %v", search, err)
	}
	if summaries, err := s.listAgentVaultSummaries(); err != nil || len(summaries.Vaults) != 1 {
		t.Fatalf("agent vaults beside an unreadable record: %+v %v", summaries, err)
	}
}

func TestOneUnreadableRecordLeavesSSHKeysAndPasskeysUsable(t *testing.T) {
	f, _, sshKey := createHealthyBesideUnreadable(t)
	s := f.service
	keys, err := NewSSHKeySource(s).ListKeys(context.Background())
	if err != nil || len(keys) != 1 || keys[0].ID != sshKey.ID {
		t.Fatalf("SSH agent keys beside an unreadable record: %+v %v", keys, err)
	}
	if _, err := PreparePasskeyRegistration(s, "example.com", nil); err != nil {
		t.Fatalf("passkey registration without exclusions beside an unreadable record: %v", err)
	}
	if prepared, err := PreparePasskeyRegistration(s, "example.com", []string{encodeBase64URL([]byte("credential-elsewhere"))}); err == nil {
		t.Fatalf("an exclusion was checked against a view missing a record: %+v", prepared)
	}
	if passkeys, err := ListPasskeys(s); err == nil {
		t.Fatalf("the credential identity list was built from a view missing a record: %+v", passkeys)
	}
}
