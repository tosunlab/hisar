package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/secretreference"
	"github.com/tryhisar/hisar/core/totp"
)

func TestResolveSecretsThroughTheApp(t *testing.T) {
	test := createCommandLineTest(t)
	service := test.testService.service
	personalID := test.testService.personalID
	github, err := service.SaveItem(personalID, ItemDetail{
		Title: "GitHub", Username: "alice", Password: "github-password",
		Fields: []ItemField{{Name: "API token", Value: "token-value", Concealed: true}}, OneTimePassword: testOneTimePassword,
	})
	if err != nil {
		t.Fatal(err)
	}
	sshKey, err := service.GenerateSSHKey(personalID, "Deploy key", "ed25519")
	if err != nil {
		t.Fatal(err)
	}

	var result cliprotocol.ResolveSecretResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret, References: []string{
		"hisar://Personal/GitHub/password",
		"hisar://personal/github/API%20token",
		"hisar://" + personalID + "/" + github.ID + "/username",
		"hisar://Personal/GitHub/otp",
		"hisar://Personal/Deploy%20key/public%20key",
	}, CommandLine: []string{"npm", "test"}}, &result)
	config, _ := totp.Parse(testOneTimePassword)
	if len(result.Values) != 5 || result.Values[0] != "github-password" || result.Values[1] != "token-value" || result.Values[2] != "alice" ||
		len(result.Values[3]) != config.Digits || result.Values[4] != sshKey.PublicKey {
		t.Errorf("values = %q", result.Values)
	}

	failure := test.send(t, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret, References: []string{
		"hisar://Personal/GitHub/password", "hisar://Personal/Gitlab/password", "hisar://Personal/GitHub/token",
	}}, nil)
	if !strings.HasPrefix(failure, "not-found: ") || !strings.Contains(failure, `hisar://Personal/Gitlab/password: no item in vault "Personal" is titled "Gitlab"`) ||
		!strings.Contains(failure, `hisar://Personal/GitHub/token: item "GitHub" has no field "token"`) || strings.Contains(failure, "github-password") {
		t.Errorf("failure = %q", failure)
	}
	failure = test.send(t, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret, References: []string{"hisar://Personal/Deploy%20key/private%20key"}}, nil)
	if failure != "not-found: hisar://Personal/Deploy%20key/private%20key: SSH private keys never leave Hisar; use the Hisar SSH agent instead" || strings.Contains(failure, "BEGIN") {
		t.Errorf("private key: %q", failure)
	}
	if failure := test.send(t, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret}, nil); !strings.HasPrefix(failure, "invalid-request: ") {
		t.Errorf("no references: %q", failure)
	}
}

func TestResolveSecretsWhileLocked(t *testing.T) {
	testService := createUnlockedTestService(t)
	testService.service.Lock()
	if _, err := testService.service.resolveSecrets([]string{"hisar://Personal/GitHub/password"}); err == nil || convertCommandLineError(err).(*cliprotocol.ResponseError).Code != cliprotocol.ErrorLocked {
		t.Errorf("locked: %v", err)
	}
}

func TestDescribeSecretsForTheApprovalWindow(t *testing.T) {
	testService := createUnlockedTestService(t)
	service := testService.service
	github, err := service.SaveItem(testService.personalID, ItemDetail{Title: "GitHub", Password: "github-password"})
	if err != nil {
		t.Fatal(err)
	}
	bridge := NewCommandLineBridge(service, clisocket.NewSessionApprovals(0, nil, nil))
	references := []string{"hisar://" + testService.personalID + "/" + github.ID + "/PASSWORD", "hisar://Personal/Nothing/password", "hisar://Work/AWS%20prod/secret%20key", "not a reference"}
	expectedValues := []string{"Personal › GitHub › password", "Personal › Nothing › password", "Work › AWS prod › secret key", "not a reference"}
	if names := bridge.DescribeSecrets(references); !slices.Equal(names, expectedValues) {
		t.Errorf("unlocked: %q", names)
	}
	service.Lock()
	expectedValues[0] = testService.personalID + " › " + github.ID + " › PASSWORD"
	if names := bridge.DescribeSecrets(references); !slices.Equal(names, expectedValues) {
		t.Errorf("locked: %q", names)
	}
}

func TestCopySecretReference(t *testing.T) {
	testService := createUnlockedTestService(t)
	service := testService.service
	personalID := testService.personalID
	github, err := service.SaveItem(personalID, ItemDetail{
		Title: "GitHub", Username: "alice", Password: "github-password", OneTimePassword: testOneTimePassword,
		Fields: []ItemField{{Name: "API token", Value: "token-value"}, {Name: "password", Value: "custom"}, {Name: "pin", Value: "1"}, {Name: "PIN", Value: "2"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	firstDatabase, _ := service.SaveItem(personalID, ItemDetail{Title: "Database", Password: "first"})
	service.SaveItem(personalID, ItemDetail{Title: "database", Password: "second"})
	slashed, _ := service.SaveItem(personalID, ItemDetail{Title: "Team/Ops 100%", Password: "ops"})
	sshKey, err := service.GenerateSSHKey(personalID, "Deploy key", "ed25519")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		itemID    string
		fieldName string
		expected  string
	}{
		{github.ID, "password", "hisar://Personal/GitHub/password"},
		{github.ID, "username", "hisar://Personal/GitHub/username"},
		{github.ID, "one-time code", "hisar://Personal/GitHub/otp"},
		{github.ID, "API token", "hisar://Personal/GitHub/API%20token"},
		{firstDatabase.ID, "password", "hisar://Personal/" + firstDatabase.ID + "/password"},
		{slashed.ID, "password", "hisar://Personal/Team%2FOps%20100%25/password"},
		{sshKey.ID, "public key", "hisar://Personal/Deploy%20key/public%20key"},
		{sshKey.ID, "fingerprint", "hisar://Personal/Deploy%20key/fingerprint"},
	}
	for _, testCase := range cases {
		reference, err := service.CopySecretReference(personalID, testCase.itemID, testCase.fieldName)
		if err != nil || reference != testCase.expected {
			t.Errorf("%s: %q, %v; want %q", testCase.fieldName, reference, err, testCase.expected)
			continue
		}
		if copied, _ := testService.clipboard.GetText(); copied != testCase.expected {
			t.Errorf("clipboard holds %q", copied)
		}
		resolvedReferences, err := secretreference.ResolveAll(serviceSecretSource{service: service, ctx: t.Context()}, []string{reference}, service.config.Now())
		if err != nil || resolvedReferences[0].Value == "" {
			t.Errorf("%s does not resolve: %v", reference, err)
		}
	}
	if _, err := service.CopySecretReference(personalID, github.ID, "pin"); err == nil || !strings.Contains(err.Error(), "2 fields are labelled") {
		t.Errorf("duplicate label: %v", err)
	}
	if _, err := service.CopySecretReference(personalID, sshKey.ID, "password"); err == nil || !strings.Contains(err.Error(), "has no password") {
		t.Errorf("SSH key password: %v", err)
	}
	if reference, err := service.CopySecretReference(personalID, sshKey.ID, "private key"); err == nil || reference != "" || !strings.Contains(err.Error(), "never leave Hisar") {
		t.Errorf("private key reference: %q, %v", reference, err)
	}
	if _, err := service.CopySecretReference(personalID, "missing", "password"); err == nil {
		t.Error("a missing item was referenced")
	}
}

// TestCopySecretReferenceNeverNamesAnotherObjectsID: the resolver tries IDs before names, so a title or vault name
// that is another item's or vault's ID must not be used as a name.
func TestCopySecretReferenceNeverNamesAnotherObjectsID(t *testing.T) {
	testService := createUnlockedTestService(t)
	service, personalID := testService.service, testService.personalID
	target, err := service.SaveItem(personalID, ItemDetail{Title: "Target", Password: "target-password"})
	if err != nil {
		t.Fatal(err)
	}
	impostor, err := service.SaveItem(personalID, ItemDetail{Title: target.ID, Password: "impostor-password"})
	if err != nil {
		t.Fatal(err)
	}
	shadowVaultID := createSharedTestVault(t, service, testService.familyID)
	inside, err := service.SaveItem(shadowVaultID, ItemDetail{Title: "Inside", Password: "inside-password"})
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct{ vaultID, itemID, want string }{
		{personalID, impostor.ID, "impostor-password"},
		{shadowVaultID, inside.ID, "inside-password"},
	} {
		reference, err := service.CopySecretReference(testCase.vaultID, testCase.itemID, "password")
		if err != nil {
			t.Fatal(err)
		}
		resolvedReferences, err := secretreference.ResolveAll(serviceSecretSource{service: service, ctx: t.Context()}, []string{reference}, service.config.Now())
		if err != nil || resolvedReferences[0].Value != testCase.want {
			t.Errorf("%s resolves to %+v, %v; want %q", reference, resolvedReferences, err, testCase.want)
		}
	}
}

// A vault whose name did not open may have any name, so while one does not open a reference by vault name is refused:
// that vault may be the one it means. A reference by vault ID still resolves, also into the vault whose name is unreadable.
func TestSecretReferenceRefusesVaultNamesWhileAVaultNameDoesNotOpen(t *testing.T) {
	test := createCommandLineTest(t)
	f := test.testService
	saveTestItem(t, f.service, f.familyID, ItemDetail{Title: "Deploy", Password: "unreadable-name-password"})
	readable, err := f.service.CreateVault("Family")
	if err != nil {
		t.Fatal(err)
	}
	saveTestItem(t, f.service, readable.ID, ItemDetail{Title: "Deploy", Password: "readable-name-password"})
	judgeCorruptFamilyName(t, f)

	failure := test.send(t, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret, References: []string{"hisar://Family/Deploy/password"}}, nil)
	if !strings.HasPrefix(failure, "not-found: hisar://Family/Deploy/password: ") || !strings.Contains(failure, "use the vault's ID") || strings.Contains(failure, "name-password") {
		t.Errorf("a vault name beside a vault whose name did not open: %q", failure)
	}
	var result cliprotocol.ResolveSecretResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret, References: []string{
		"hisar://" + readable.ID + "/Deploy/password", "hisar://" + f.familyID + "/Deploy/password",
	}}, &result)
	if !slices.Equal(result.Values, []string{"readable-name-password", "unreadable-name-password"}) {
		t.Errorf("vault IDs beside a vault whose name did not open: %q", result.Values)
	}
}

// Agents resolve among the vaults available to them, so only a vault among those whose name did not open refuses their
// vault names.
func TestAgentSecretReferenceRefusesVaultNamesWhileAnAgentVaultNameDoesNotOpen(t *testing.T) {
	test := createAgentSessionTest(t, clisocket.AgentGrantOnce)
	f := test.testService
	saveTestItem(t, f.service, f.familyID, ItemDetail{Title: "Deploy token", Password: "family-token-value"})
	judgeCorruptFamilyName(t, f)
	connection := test.connect(t)
	test.runDeploy(t, connection)

	if _, err := f.service.SetVaultAvailableToAgents(f.familyID, true); err != nil {
		t.Fatal(err)
	}
	failure := connection.send(t, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret, References: []string{deployTokenReference}, CommandLine: []string{"make", "deploy"}}, nil)
	if !strings.HasPrefix(failure, "not-found: ") || !strings.Contains(failure, "use the vault's ID") || strings.Contains(failure, "token-value") {
		t.Errorf("an agent's vault name beside an agent vault whose name did not open: %q", failure)
	}
	var result cliprotocol.ResolveSecretResult
	connection.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret, References: []string{
		"hisar://" + f.personalID + "/Deploy%20token/password", "hisar://" + f.familyID + "/Deploy%20token/password",
	}, CommandLine: []string{"make", "deploy"}}, &result)
	if !slices.Equal(result.Values, []string{"deploy-token-value", "family-token-value"}) {
		t.Errorf("agent vault IDs beside an agent vault whose name did not open: %q", result.Values)
	}
}

// A reference Hisar hands out while a vault name does not open keeps finding its item once that name opens again and
// turns out to be the same as the item's vault's.
func TestHandedOutReferencesKeepTheirItemWhenAnUnreadableVaultNameReturns(t *testing.T) {
	f := createUnlockedTestService(t)
	readable, err := f.service.CreateVault("Family")
	if err != nil {
		t.Fatal(err)
	}
	deploy := saveTestItem(t, f.service, readable.ID, ItemDetail{Title: "Deploy", Password: "readable-name-password"})
	name := judgeCorruptFamilyName(t, f)
	if _, err := f.service.ListVaults(); err != nil {
		t.Fatal(err)
	}
	copied, err := f.service.CopySecretReference(readable.ID, deploy.ID, "password")
	if err != nil {
		t.Fatal(err)
	}
	launcherItems, err := f.service.listLauncherItems()
	if err != nil {
		t.Fatal(err)
	}
	references := []string{copied}
	for _, launcherItem := range launcherItems.Items {
		for _, reference := range launcherItem.References {
			if launcherItem.ID == deploy.ID && reference.Field == "password" {
				references = append(references, reference.Reference)
			}
		}
	}
	if len(references) != 2 {
		t.Fatalf("references %q", references)
	}

	f.apiServer.mutex.Lock()
	for index := range f.apiServer.vaults {
		if f.apiServer.vaults[index].ID == f.familyID {
			f.apiServer.vaults[index].EncName = name
		}
	}
	f.apiServer.mutex.Unlock()
	result, err := f.service.resolveSecrets(references)
	if err != nil || !slices.Equal(result.Values, []string{"readable-name-password", "readable-name-password"}) {
		t.Errorf("%q once both vaults are named Family: %q %v", references, result.Values, err)
	}
}
