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
	if _, err := testService.service.resolveAllSecret([]string{"hisar://Personal/GitHub/password"}); err == nil || convertCommandLineError(err).(*cliprotocol.ResponseError).Code != cliprotocol.ErrorLocked {
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
	bridge := CreateCommandLineBridge(service, clisocket.CreateSessionApprovals(0, nil))
	allReference := []string{"hisar://" + testService.personalID + "/" + github.ID + "/PASSWORD", "hisar://Personal/Nothing/password", "hisar://Work/AWS%20prod/secret%20key", "not a reference"}
	allExpected := []string{"Personal › GitHub › password", "Personal › Nothing › password", "Work › AWS prod › secret key", "not a reference"}
	if allName := bridge.DescribeAllSecret(allReference); !slices.Equal(allName, allExpected) {
		t.Errorf("unlocked: %q", allName)
	}
	service.Lock()
	allExpected[0] = testService.personalID + " › " + github.ID + " › PASSWORD"
	if allName := bridge.DescribeAllSecret(allReference); !slices.Equal(allName, allExpected) {
		t.Errorf("locked: %q", allName)
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
	allCase := []struct {
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
	for _, testCase := range allCase {
		reference, err := service.CopySecretReference(personalID, testCase.itemID, testCase.fieldName)
		if err != nil || reference != testCase.expected {
			t.Errorf("%s: %q, %v; want %q", testCase.fieldName, reference, err, testCase.expected)
			continue
		}
		if copied, _ := testService.clipboard.GetText(); copied != testCase.expected {
			t.Errorf("clipboard holds %q", copied)
		}
		allResolved, err := secretreference.ResolveAll(serviceSecretSource{service: service, ctx: t.Context()}, []string{reference}, service.config.Now())
		if err != nil || allResolved[0].Value == "" {
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
