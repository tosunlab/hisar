package app

import (
	"context"
	"encoding/json"
	"net"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/item"
)

// raycastPeerResolver is a caller without a terminal, in launchd's session like every Mac app.
type raycastPeerResolver struct{ staticPeerResolver }

func (raycastPeerResolver) GetTerminalSession(int) (clisocket.TerminalSession, error) {
	return clisocket.TerminalSession{SessionID: 1, LeaderStartTime: 1}, nil
}

type countingLauncherApprover struct {
	mutex sync.Mutex
	count int
}

func (approver *countingLauncherApprover) ApproveLauncher(context.Context, clisocket.LauncherApprovalRequest) error {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	approver.count++
	return nil
}

func (approver *countingLauncherApprover) getCount() int {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	return approver.count
}

// launcherTest serves cli.sock to callers in a verified Raycast.app.
type launcherTest struct {
	testService      unlockedTestService
	approvals        *clisocket.SessionApprovals
	approver         *countingApprover
	launcherApprover *countingLauncherApprover
	server           *clisocket.Server
}

func createLauncherTest(t *testing.T) *launcherTest {
	t.Helper()
	test := &launcherTest{
		testService: createUnlockedTestService(t), approvals: clisocket.NewSessionApprovals(0, nil),
		approver: &countingApprover{}, launcherApprover: &countingLauncherApprover{},
	}
	test.server = clisocket.NewServer(clisocket.ServerConfig{
		Handler: NewCommandLineBridge(test.testService.service, test.approvals), Approver: test.approver, LauncherApprover: test.launcherApprover,
		IdentifyApp: func(int) (clisocket.AppIdentity, bool) {
			return clisocket.AppIdentity{Path: "/Applications/Raycast.app", TeamID: "RAYCAST123"}, true
		},
		PeerResolver: raycastPeerResolver{}, Approvals: test.approvals,
	})
	return test
}

// connect opens a command-line connection from Raycast.
func (test *launcherTest) connect(t *testing.T) *commandLineTest {
	t.Helper()
	serverSide, clientSide := net.Pipe()
	go test.server.ServeConnection(context.Background(), serverSide)
	t.Cleanup(func() { clientSide.Close() })
	return &commandLineTest{testService: test.testService, approvals: test.approvals, approver: test.approver, connection: clientSide}
}

func (test *launcherTest) connectLauncher(t *testing.T) *commandLineTest {
	t.Helper()
	launcher := test.connect(t)
	launcher.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeLauncherHello}, nil)
	return launcher
}

func saveTestItem(t *testing.T, service *Service, vaultID string, detail ItemDetail) ItemDetail {
	t.Helper()
	saved, err := service.SaveItem(vaultID, detail)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func TestLauncherListsEveryVaultWithoutSecrets(t *testing.T) {
	test := createLauncherTest(t)
	service := test.testService.service
	personalID, familyID := test.testService.personalID, test.testService.familyID
	engineeringID := createOrganizationTestVault(t, service)
	github := saveTestItem(t, service, personalID, ItemDetail{
		Title: "GitHub", Username: "alice", URLs: []string{"https://github.com"}, Tags: []string{"dev"}, Password: "github-password-1111",
		Notes: "recovery-codes-2222", OneTimePassword: testOneTimePassword, Fields: []ItemField{{Name: "PIN", Value: "pin-3333", Concealed: true}},
	})
	if err := service.SetItemFavorite(personalID, github.ID, true); err != nil {
		t.Fatal(err)
	}
	wifi := saveTestItem(t, service, familyID, ItemDetail{Category: item.CategorySecureNote, Title: "Office Wi-Fi", Notes: "wifi-password-4444"})
	bank := saveTestItem(t, service, engineeringID, ItemDetail{Title: "Bank", Password: "bank-password-5555"})
	otherBank := saveTestItem(t, service, engineeringID, ItemDetail{Title: "BANK", Username: "ops"})

	launcher := test.connectLauncher(t)
	launcher.connection.SetDeadline(time.Now().Add(10 * time.Second))
	if err := cliprotocol.WriteJSON(launcher.connection, cliprotocol.Request{ID: "list", Type: cliprotocol.TypeLauncherListItem}); err != nil {
		t.Fatal(err)
	}
	message, err := cliprotocol.ReadMessage(launcher.connection)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"github-password-1111", "recovery-codes-2222", "JBSWY3DPEHPK3PXP", "otpauth", "pin-3333", "wifi-password-4444", "bank-password-5555"} {
		if strings.Contains(string(message), secret) {
			t.Errorf("launcher.items.list sent %q", secret)
		}
	}
	var response cliprotocol.Response
	var list cliprotocol.LauncherItemListResult
	if err := json.Unmarshal(message, &response); err != nil || !response.OK || json.Unmarshal(response.Result, &list) != nil {
		t.Fatalf("launcher.items.list: %s", message)
	}

	expectedVaults := []cliprotocol.LauncherVault{{ID: engineeringID, Name: "Engineering"}, {ID: familyID, Name: "Family"}, {ID: personalID, Name: "Personal"}}
	if !slices.Equal(list.Vaults, expectedVaults) {
		t.Errorf("vaults %+v", list.Vaults)
	}
	expectedItems := []cliprotocol.LauncherItem{
		{
			ID: github.ID, VaultID: personalID, Title: "GitHub", Category: item.CategoryLogin, Username: "alice", URLs: []string{"https://github.com"},
			Tags: []string{"dev"}, IsFavorite: true, HasPassword: true, HasOneTimeCode: true,
			References: []cliprotocol.LauncherReference{
				{Field: "username", Reference: "hisar://Personal/GitHub/username"},
				{Field: "password", Reference: "hisar://Personal/GitHub/password"},
				{Field: "one-time code", Reference: "hisar://Personal/GitHub/otp"},
				{Field: "PIN", Reference: "hisar://Personal/GitHub/PIN"},
				{Field: "notes", Reference: "hisar://Personal/GitHub/notes"},
			},
		},
		{
			ID: wifi.ID, VaultID: familyID, Title: "Office Wi-Fi", Category: item.CategorySecureNote, URLs: []string{}, Tags: []string{},
			References: []cliprotocol.LauncherReference{{Field: "notes", Reference: "hisar://Family/Office%20Wi-Fi/notes"}},
		},
		{
			ID: bank.ID, VaultID: engineeringID, Title: "Bank", Category: item.CategoryLogin, URLs: []string{}, Tags: []string{}, HasPassword: true,
			References: []cliprotocol.LauncherReference{{Field: "password", Reference: "hisar://Engineering/" + bank.ID + "/password"}},
		},
		{
			ID: otherBank.ID, VaultID: engineeringID, Title: "BANK", Category: item.CategoryLogin, Username: "ops", URLs: []string{}, Tags: []string{},
			References: []cliprotocol.LauncherReference{{Field: "username", Reference: "hisar://Engineering/" + otherBank.ID + "/username"}},
		},
	}
	if len(list.Items) != len(expectedItems) {
		t.Fatalf("%d items: %+v", len(list.Items), list.Items)
	}
	for _, expected := range expectedItems {
		index := slices.IndexFunc(list.Items, func(launcherItem cliprotocol.LauncherItem) bool { return launcherItem.ID == expected.ID })
		if index < 0 || !reflect.DeepEqual(list.Items[index], expected) {
			t.Errorf("%s: %+v", expected.Title, list.Items[max(index, 0)])
		}
	}
	if count := test.launcherApprover.getCount(); count != 1 || test.approver.getCount() != 0 {
		t.Errorf("%d launcher approvals and %d command-line approvals; want only the launcher's", count, test.approver.getCount())
	}
}

func TestLauncherReadsOnePasswordOrCodeAndNamesWhatIsMissing(t *testing.T) {
	test := createLauncherTest(t)
	service := test.testService.service
	personalID, familyID := test.testService.personalID, test.testService.familyID
	github := saveTestItem(t, service, personalID, ItemDetail{Title: "GitHub", Username: "alice", Password: "github-password", OneTimePassword: testOneTimePassword})
	wifi := saveTestItem(t, service, familyID, ItemDetail{Category: item.CategorySecureNote, Title: "Office Wi-Fi", Notes: "wifi-password"})
	launcher := test.connectLauncher(t)

	var password cliprotocol.LauncherPasswordResult
	launcher.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeLauncherReadPassword, VaultID: personalID, ItemID: github.ID}, &password)
	if password.Password != "github-password" {
		t.Errorf("password %q", password.Password)
	}
	var code cliprotocol.CodeResult
	launcher.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeLauncherGetCode, VaultID: personalID, ItemID: github.ID}, &code)
	if len(code.Code) != 6 || strings.IndexFunc(code.Code, func(character rune) bool { return !unicode.IsDigit(character) }) >= 0 || code.Period != 30 || code.ExpiresAt == "" {
		t.Errorf("code %+v", code)
	}

	for _, testCase := range []struct {
		request  cliprotocol.Request
		expected string
	}{
		{cliprotocol.Request{Type: cliprotocol.TypeLauncherReadPassword, VaultID: familyID, ItemID: wifi.ID}, "not-found: “Office Wi-Fi” has no password"},
		{cliprotocol.Request{Type: cliprotocol.TypeLauncherGetCode, VaultID: familyID, ItemID: wifi.ID}, "not-found: “Office Wi-Fi” has no one-time code"},
		{cliprotocol.Request{Type: cliprotocol.TypeLauncherCopyField, VaultID: familyID, ItemID: wifi.ID, Field: cliprotocol.LauncherFieldUsername}, "not-found: “Office Wi-Fi” has no username"},
		{cliprotocol.Request{Type: cliprotocol.TypeLauncherReadPassword, VaultID: personalID, ItemID: "missing"}, "not-found: item not found"},
		{cliprotocol.Request{Type: cliprotocol.TypeLauncherGetCode, VaultID: "missing", ItemID: github.ID}, "not-found: vault not found"},
	} {
		if failure := launcher.send(t, testCase.request, nil); failure != testCase.expected {
			t.Errorf("%s of %s: %q; want %q", testCase.request.Type, testCase.request.ItemID, failure, testCase.expected)
		}
	}
	if copied, _ := test.testService.clipboard.GetText(); copied != "" {
		t.Errorf("a refused copy wrote the clipboard: %q", copied)
	}
}

func TestLauncherCopiesInTheAppAndSaysWhenItClears(t *testing.T) {
	test := createLauncherTest(t)
	service := test.testService.service
	service.mutex.Lock()
	service.config.ClipboardClearAfter = 90 * time.Second
	service.mutex.Unlock()
	personalID := test.testService.personalID
	github := saveTestItem(t, service, personalID, ItemDetail{Title: "GitHub", Username: "alice", Password: "github-password", OneTimePassword: testOneTimePassword})
	service.mutex.Lock()
	clear(service.itemsByVaultID)
	service.mutex.Unlock()
	launcher := test.connectLauncher(t)

	for _, testCase := range []struct {
		field   string
		isValue func(string) bool
	}{
		{cliprotocol.LauncherFieldPassword, func(text string) bool { return text == "github-password" }},
		{cliprotocol.LauncherFieldUsername, func(text string) bool { return text == "alice" }},
		{cliprotocol.LauncherFieldOneTimeCode, func(text string) bool {
			return len(text) == 6 && strings.IndexFunc(text, func(character rune) bool { return !unicode.IsDigit(character) }) < 0
		}},
	} {
		var copied cliprotocol.LauncherCopyResult
		launcher.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeLauncherCopyField, VaultID: personalID, ItemID: github.ID, Field: testCase.field}, &copied)
		if text, _ := test.testService.clipboard.GetText(); !testCase.isValue(text) || copied.ClearAfterSeconds != 90 {
			t.Errorf("copying the %s: clipboard %q, %+v", testCase.field, text, copied)
		}
	}
}

func TestLauncherGeneratesWithTheAppsRules(t *testing.T) {
	test := createLauncherTest(t)
	launcher := test.connectLauncher(t)
	hasSymbol := func(password string) bool { return strings.ContainsAny(password, "!@#$%^&*-_=+?") }
	hasDigit := func(password string) bool { return strings.ContainsAny(password, "0123456789") }

	var generated cliprotocol.GeneratedPasswordResult
	launcher.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeLauncherGeneratePassword}, &generated)
	if len(generated.Password) != 24 || !hasDigit(generated.Password) || !hasSymbol(generated.Password) {
		t.Errorf("default password %q", generated.Password)
	}
	launcher.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeLauncherGeneratePassword, Generator: &cliprotocol.GeneratorChange{Length: 20, Digits: true}}, &generated)
	if len(generated.Password) != 20 || !hasDigit(generated.Password) || hasSymbol(generated.Password) {
		t.Errorf("20 characters with digits and no symbols: %q", generated.Password)
	}
	for _, testCase := range []struct {
		generator cliprotocol.GeneratorChange
		expected  string
	}{
		{cliprotocol.GeneratorChange{Length: 3, Digits: true, Symbols: true}, "invalid-request: choose a length between 4 and 256"},
		{cliprotocol.GeneratorChange{Length: 1}, "invalid-request: choose a length between 2 and 256"},
		{cliprotocol.GeneratorChange{Length: 257, Digits: true}, "invalid-request: choose a length between 3 and 256"},
	} {
		if failure := launcher.send(t, cliprotocol.Request{Type: cliprotocol.TypeLauncherGeneratePassword, Generator: &testCase.generator}, nil); failure != testCase.expected {
			t.Errorf("%+v: %q", testCase.generator, failure)
		}
	}
	if count := test.launcherApprover.getCount(); count != 0 {
		t.Errorf("generating asked %d times", count)
	}
}

func TestLauncherLockLocksHisarAndEndsEveryApproval(t *testing.T) {
	test := createLauncherTest(t)
	service := test.testService.service
	launcher := test.connectLauncher(t)
	launcher.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeLauncherListItem}, nil)
	commandLine := test.connect(t)
	commandLine.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeListVault}, nil)

	launcher.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeLauncherLock}, nil)
	if state := service.GetStatus().State; state != StateLocked {
		t.Fatalf("state %q after launcher.lock", state)
	}
	if count := service.config.LockNotifier.(*fakeLockNotifier).getLockedCount(); count != 1 {
		t.Errorf("the window was told about %d locks", count)
	}
	if launchers := test.approvals.ListLaunchers(); len(launchers) != 0 {
		t.Errorf("launchers still approved: %+v", launchers)
	}
	if failure := launcher.send(t, cliprotocol.Request{Type: cliprotocol.TypeLauncherListItem}, nil); !strings.HasPrefix(failure, cliprotocol.ErrorLocked) || test.launcherApprover.getCount() != 2 {
		t.Errorf("launcher after the lock: %q, %d launcher approvals", failure, test.launcherApprover.getCount())
	}
	if failure := commandLine.send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault}, nil); !strings.HasPrefix(failure, cliprotocol.ErrorLocked) || test.approver.getCount() != 2 {
		t.Errorf("command line after the lock: %q, %d command-line approvals", failure, test.approver.getCount())
	}
}
