package hisarkit

import (
	"slices"
	"testing"
)

// checkCredentialExchangeGroupsFlow runs inside TestIntegrationMobileFlow against the real server: a Credential Exchange
// with two collections and an uncollected item previews three groups in Personal, and the one collection mapped to a
// shared vault is stored there alone.
func checkCredentialExchangeGroupsFlow(t *testing.T, hisar *Hisar, personalID string) {
	t.Helper()
	team := decodeJSON[struct {
		ID string `json:"id"`
	}](t)(hisar.CreateVault("CXF Team"))
	credentialData := []byte(`{"version":{"major":1,"minor":0},"exporterRpId":"example.com","exporterDisplayName":"Provider","timestamp":1767225600,` +
		`"accounts":[{"id":"AQ","username":"alice","collections":[` +
		`{"id":"Yw","title":"Work","items":[{"item":"dw"}]},{"id":"Zg","title":"Family","items":[{"item":"ZG"}]}],"items":[` +
		`{"id":"dw","title":"CXF work","credentials":[{"type":"basic-auth","password":{"fieldType":"concealed-string","value":"cxf-work-secret"}}]},` +
		`{"id":"ZG","title":"CXF family","credentials":[{"type":"basic-auth","password":{"fieldType":"concealed-string","value":"cxf-family-secret"}}]},` +
		`{"id":"cA","title":"CXF own","credentials":[{"type":"basic-auth","password":{"fieldType":"concealed-string","value":"cxf-own-secret"}}]}]}]}`)
	type preview struct {
		VaultID string `json:"vaultId"`
		Groups  []struct {
			ID      string     `json:"id"`
			Name    string     `json:"name"`
			VaultID string     `json:"vaultId"`
			Members []struct{} `json:"members"`
		} `json:"groups"`
	}
	prepared := decodeJSON[preview](t)(hisar.PrepareCredentialExchangeImport(team.ID, credentialData))
	workID := ""
	for _, group := range prepared.Groups {
		if group.VaultID != personalID {
			t.Errorf("%s starts in %s, not Personal", group.Name, group.VaultID)
		}
		if group.Name == "alice / Work" {
			workID = group.ID
		}
	}
	if len(prepared.Groups) != 3 || workID == "" {
		t.Fatalf("Credential Exchange groups = %+v", prepared.Groups)
	}
	if _, err := hisar.PreviewImport(team.ID); err == nil {
		t.Error("one vault was chosen for every collection")
	}
	mapped := decodeJSON[preview](t)(hisar.MapImportGroup(workID, team.ID))
	if mapped.VaultID != "" {
		t.Errorf("mixed destinations preview as vault %q", mapped.VaultID)
	}
	progress := decodeJSON[struct {
		IsDone bool `json:"isDone"`
	}](t)(hisar.StartImport(mapped.VaultID, true))
	for !progress.IsDone {
		progress = decodeJSON[struct {
			IsDone bool `json:"isDone"`
		}](t)(hisar.ContinueImport())
	}
	hisar.CancelImport()
	readPasswords := func(vaultID string) []string {
		t.Helper()
		type listed struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}
		passwords := []string{}
		for _, summary := range decodeJSON[[]listed](t)(hisar.ListAllItem(vaultID)) {
			detail := decodeJSON[struct {
				Password string `json:"password"`
			}](t)(hisar.GetItem(vaultID, summary.ID))
			if slices.Contains([]string{"CXF work", "CXF family", "CXF own"}, summary.Title) {
				passwords = append(passwords, summary.Title+"="+detail.Password)
			}
		}
		slices.Sort(passwords)
		return passwords
	}
	if got, want := readPasswords(team.ID), []string{"CXF work=cxf-work-secret"}; !slices.Equal(got, want) {
		t.Errorf("CXF Team holds %v, want %v", got, want)
	}
	if got, want := readPasswords(personalID), []string{"CXF family=cxf-family-secret", "CXF own=cxf-own-secret"}; !slices.Equal(got, want) {
		t.Errorf("Personal holds %v, want %v", got, want)
	}
}
