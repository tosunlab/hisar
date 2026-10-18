package app

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/item"
)

const testOneTimePassword = "otpauth://totp/Example:alice?secret=JBSWY3DPEHPK3PXP&issuer=Example"

type staticPeerResolver struct{}

func (staticPeerResolver) ResolvePeer(net.Conn) (clisocket.Peer, error) {
	return clisocket.Peer{UID: os.Getuid(), PID: 4242}, nil
}

func (staticPeerResolver) GetTerminalSession(int) (clisocket.TerminalSession, error) {
	return clisocket.TerminalSession{SessionID: 4200, TerminalDevice: 7, LeaderStartTime: 1}, nil
}

func (staticPeerResolver) GetProcessPath(int) (string, error) { return "/usr/local/bin/hisar", nil }

func (staticPeerResolver) GetParentPID(int) (int, error) { return 4000, nil }

type countingApprover struct {
	mutex sync.Mutex
	count int
}

func (approver *countingApprover) ApproveCommandLine(context.Context, clisocket.ApprovalRequest) error {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	approver.count++
	return nil
}

func (approver *countingApprover) getCount() int {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	return approver.count
}

type commandLineTest struct {
	testService unlockedTestService
	approvals   *clisocket.SessionApprovals
	approver    *countingApprover
	connection  net.Conn
	nextID      int
}

func createCommandLineTest(t *testing.T) *commandLineTest {
	t.Helper()
	test := &commandLineTest{testService: createUnlockedTestService(t), approvals: clisocket.NewSessionApprovals(0, nil), approver: &countingApprover{}}
	bridge := NewCommandLineBridge(test.testService.service, test.approvals)
	server := clisocket.NewServer(clisocket.ServerConfig{Handler: bridge, Approver: test.approver, PeerResolver: staticPeerResolver{}, Approvals: test.approvals})
	serverSide, clientSide := net.Pipe()
	go server.ServeConnection(context.Background(), serverSide)
	t.Cleanup(func() { clientSide.Close() })
	test.connection = clientSide
	return test
}

func (test *commandLineTest) send(t *testing.T, request cliprotocol.Request, result any) string {
	t.Helper()
	test.nextID++
	request.ID = "request-" + string(rune('a'+test.nextID))
	test.connection.SetDeadline(time.Now().Add(10 * time.Second))
	if err := cliprotocol.WriteJSON(test.connection, request); err != nil {
		t.Fatal(err)
	}
	var response cliprotocol.Response
	if err := cliprotocol.ReadJSON(test.connection, &response); err != nil {
		t.Fatal(err)
	}
	if response.Error != nil {
		return response.Error.Code + ": " + response.Error.Message
	}
	if result != nil {
		if err := json.Unmarshal(response.Result, result); err != nil {
			t.Fatal(err)
		}
	}
	return ""
}

func (test *commandLineTest) mustSend(t *testing.T, request cliprotocol.Request, result any) {
	t.Helper()
	if failure := test.send(t, request, result); failure != "" {
		t.Fatalf("%s: %s", request.Type, failure)
	}
}

func TestCommandLineItemsThroughTheApp(t *testing.T) {
	test := createCommandLineTest(t)
	service := test.testService.service
	saved, err := service.SaveItem(test.testService.personalID, ItemDetail{
		Title: "GitHub", Username: "alice", URLs: []string{"https://github.com"}, Password: "first-password", Notes: "work account",
		Fields: []ItemField{{Name: "PIN", Value: "1234", Concealed: true}}, OneTimePassword: testOneTimePassword,
	})
	if err != nil {
		t.Fatal(err)
	}

	var hello cliprotocol.HelloResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeHello}, &hello)
	if hello.State != cliprotocol.StateUnlocked || hello.Email != "a@example.com" || hello.IsApproved {
		t.Errorf("hello %+v", hello)
	}
	if !hello.HasFeature(cliprotocol.FeatureSecureNotes) {
		t.Errorf("hello does not announce secure notes: %+v", hello.Features)
	}

	var account cliprotocol.AccountResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeAccount}, &account)
	if account.Name != "Alice" || account.Role != "admin" || account.AccountID != test.testService.accountID {
		t.Errorf("account %+v", account)
	}

	var vaultList cliprotocol.VaultListResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeListVault}, &vaultList)
	if len(vaultList.Vaults) != 2 || vaultList.Vaults[0].Name != "Personal" || vaultList.Vaults[0].ItemCount != 1 {
		t.Errorf("vaults %+v", vaultList)
	}

	var itemList cliprotocol.ItemListResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeListItem, VaultID: test.testService.personalID}, &itemList)
	if len(itemList.Items) != 1 || itemList.Items[0].Title != "GitHub" || itemList.Items[0].Password != "" || !itemList.Items[0].HasOneTimePassword {
		t.Errorf("items %+v", itemList)
	}

	service.mutex.Lock()
	delete(service.itemsByVaultID, test.testService.personalID)
	service.mutex.Unlock()

	var itemResult cliprotocol.ItemResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeGetItem, VaultID: test.testService.personalID, ItemID: saved.ID}, &itemResult)
	if itemResult.Item.Password != "first-password" || itemResult.Item.Notes != "work account" || len(itemResult.Item.Fields) != 1 {
		t.Errorf("item %+v", itemResult.Item)
	}
	var codeResult cliprotocol.CodeResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeGetCode, VaultID: test.testService.personalID, ItemID: saved.ID}, &codeResult)
	if len(codeResult.Code) != 6 || codeResult.Period != 30 || codeResult.ExpiresAt == "" {
		t.Errorf("code %+v", codeResult)
	}
}

func TestCommandLineNeverSendsSecretsItDoesNotShow(t *testing.T) {
	test := createCommandLineTest(t)
	service := test.testService.service
	saved, err := service.SaveItem(test.testService.personalID, ItemDetail{Title: "Example", Username: "alice", Password: "pw", OneTimePassword: testOneTimePassword})
	if err != nil {
		t.Fatal(err)
	}
	service.mutex.Lock()
	cachedItem, _ := service.findItem(test.testService.personalID, saved.ID)
	cachedItem.Details.Passkeys = []item.Passkey{{CredentialID: []byte{1}, RelyingPartyID: "example.com", UserName: "alice@example.com", PrivateKey: []byte("PASSKEY-PRIVATE-KEY")}}
	service.mutex.Unlock()

	test.connection.SetDeadline(time.Now().Add(10 * time.Second))
	cliprotocol.WriteJSON(test.connection, cliprotocol.Request{ID: "raw", Type: cliprotocol.TypeGetItem, VaultID: test.testService.personalID, ItemID: saved.ID})
	message, err := cliprotocol.ReadMessage(test.connection)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"JBSWY3DPEHPK3PXP", "otpauth", "PASSKEY-PRIVATE-KEY", "UEFTU0tFWS1QUklWQVRFLUtFWQ"} {
		if strings.Contains(string(message), secret) {
			t.Errorf("items.get sent %q: %s", secret, message)
		}
	}
	if !strings.Contains(string(message), `"relyingPartyID":"example.com"`) {
		t.Errorf("passkey summary missing: %s", message)
	}
}

func TestCommandLineSaveKeepsWhatTheCommandLineDoesNotEdit(t *testing.T) {
	test := createCommandLineTest(t)
	service := test.testService.service
	vaultID := test.testService.personalID
	saved, err := service.SaveItem(vaultID, ItemDetail{
		Title: "Bank", Username: "alice", Password: "old", Fields: []ItemField{{Name: "PIN", Value: "1234"}}, OneTimePassword: testOneTimePassword,
	})
	if err != nil {
		t.Fatal(err)
	}
	var itemResult cliprotocol.ItemResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID, Item: &cliprotocol.ItemChange{
		ID: saved.ID, Revision: saved.Revision, Title: "Bank", Username: "alice", URLs: []string{"https://bank.example.com"}, Password: "new", Notes: "edited",
	}}, &itemResult)
	stored, err := service.GetItem(vaultID, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Password != "new" || stored.Notes != "edited" || len(stored.Fields) != 1 || stored.OneTimePassword == "" || stored.URLs[0] != "https://bank.example.com" {
		t.Errorf("stored %+v", stored)
	}
	if failure := test.send(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID, Item: &cliprotocol.ItemChange{
		ID: saved.ID, Revision: saved.Revision, Title: "Bank", Password: "stale",
	}}, nil); !strings.Contains(failure, "changed on another device") {
		t.Errorf("stale revision: %q", failure)
	}

	var created cliprotocol.ItemResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID, Item: &cliprotocol.ItemChange{Title: "New login", Username: "bob", Password: "generated"}}, &created)
	if created.Item.ID == "" || created.Item.Password != "generated" {
		t.Errorf("created %+v", created)
	}
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeDeleteItem, VaultID: vaultID, ItemID: created.Item.ID}, nil)
	if failure := test.send(t, cliprotocol.Request{Type: cliprotocol.TypeGetItem, VaultID: vaultID, ItemID: created.Item.ID}, nil); !strings.HasPrefix(failure, cliprotocol.ErrorNotFound) {
		t.Errorf("deleted item: %q", failure)
	}
	if failure := test.send(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID}, nil); !strings.HasPrefix(failure, cliprotocol.ErrorInvalidRequest) {
		t.Errorf("save without item: %q", failure)
	}
}

func TestCommandLineListsTagsAndKeepsThemUnlessSent(t *testing.T) {
	test := createCommandLineTest(t)
	service := test.testService.service
	vaultID := test.testService.personalID
	saved, err := service.SaveItem(vaultID, ItemDetail{Title: "GitHub", Username: "alice", Tags: []string{"work", "dev"}})
	if err != nil {
		t.Fatal(err)
	}
	var hello cliprotocol.HelloResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeHello}, &hello)
	if !hello.HasFeature(cliprotocol.FeatureTags) {
		t.Errorf("hello does not announce tags: %+v", hello.Features)
	}
	var itemList cliprotocol.ItemListResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeListItem, VaultID: vaultID}, &itemList)
	if len(itemList.Items) != 1 || !slices.Equal(itemList.Items[0].Tags, []string{"work", "dev"}) {
		t.Errorf("items %+v", itemList)
	}
	var itemResult cliprotocol.ItemResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeGetItem, VaultID: vaultID, ItemID: saved.ID}, &itemResult)
	if !slices.Equal(itemResult.Item.Tags, []string{"work", "dev"}) {
		t.Errorf("item %+v", itemResult.Item)
	}

	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID, Item: &cliprotocol.ItemChange{
		ID: saved.ID, Revision: saved.Revision, Title: "GitHub", Username: "alice", Notes: "edited by an older hisar",
	}}, &itemResult)
	if !slices.Equal(itemResult.Item.Tags, []string{"work", "dev"}) {
		t.Errorf("a save without tags dropped them: %+v", itemResult.Item)
	}
	tags := []string{"personal"}
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID, Item: &cliprotocol.ItemChange{
		ID: saved.ID, Revision: itemResult.Item.Revision, Title: "GitHub", Username: "alice", Tags: &tags,
	}}, &itemResult)
	if !slices.Equal(itemResult.Item.Tags, []string{"personal"}) {
		t.Errorf("a save with tags did not replace them: %+v", itemResult.Item)
	}
	tags = []string{}
	var cleared cliprotocol.ItemResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID, Item: &cliprotocol.ItemChange{
		ID: saved.ID, Revision: itemResult.Item.Revision, Title: "GitHub", Username: "alice", Tags: &tags,
	}}, &cleared)
	if len(cleared.Item.Tags) != 0 {
		t.Errorf("a save with no tags kept them: %+v", cleared.Item)
	}
}

func TestCommandLineImportOnlyCreatesItems(t *testing.T) {
	test := createCommandLineTest(t)
	service := test.testService.service
	vaultID := test.testService.personalID
	existing, err := service.SaveItem(vaultID, ItemDetail{Title: "Keep me", Password: "original"})
	if err != nil {
		t.Fatal(err)
	}
	var result cliprotocol.ImportResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeImportItem, VaultID: vaultID, ImportedItems: []item.Item{
		{ID: existing.ID, Revision: existing.Revision, Overview: item.Overview{Title: "Imported"}, Details: item.Details{Password: "imported"}},
		{Overview: item.Overview{Title: "Second"}, Details: item.Details{Notes: "note"}},
	}}, &result)
	if result.SavedCount != 2 || len(result.Failures) != 0 {
		t.Errorf("import %+v", result)
	}
	if kept, err := service.GetItem(vaultID, existing.ID); err != nil || kept.Title != "Keep me" || kept.Password != "original" {
		t.Errorf("import replaced an existing item: %+v %v", kept, err)
	}
	tooManies := make([]item.Item, cliprotocol.MaxImportBatch+1)
	if failure := test.send(t, cliprotocol.Request{Type: cliprotocol.TypeImportItem, VaultID: vaultID, ImportedItems: tooManies}, nil); !strings.HasPrefix(failure, cliprotocol.ErrorInvalidRequest) {
		t.Errorf("oversized batch: %q", failure)
	}
}

func TestCommandLineSharingAndDevices(t *testing.T) {
	test := createCommandLineTest(t)
	var peopleList cliprotocol.PeopleListResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeListPeople}, &peopleList)
	if len(peopleList.People) != 1 || !peopleList.People[0].IsCurrentUser || peopleList.People[0].SecurityCode == "" {
		t.Errorf("people %+v", peopleList)
	}
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeInvite, Email: "new@example.com"}, nil)
	if invited := test.testService.apiServer.invitedEmails; len(invited) != 1 || invited[0] != "new@example.com" {
		t.Errorf("invited %v", invited)
	}
	test.testService.apiServer.mutex.Lock()
	test.testService.apiServer.role, test.testService.apiServer.isInviteAdminsOnly = apitypes.RoleMember, true
	test.testService.apiServer.mutex.Unlock()
	if failure := test.send(t, cliprotocol.Request{Type: cliprotocol.TypeInvite, Email: "refused@example.com"}, nil); !strings.HasSuffix(failure, ": Only admins can invite people to the Hisar server.") {
		t.Errorf("a member's invite under admins only: %q", failure)
	}
	test.testService.apiServer.mutex.Lock()
	test.testService.apiServer.isInviteAdminsOnly = false
	test.testService.apiServer.mutex.Unlock()
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeShareVault, VaultID: test.testService.familyID, Email: "bob@example.com", Role: "reader"}, nil)
	test.testService.apiServer.mutex.Lock()
	test.testService.apiServer.receivedShareInvitations = []apitypes.ReceivedShareInvitation{{ID: "from-carol", VaultID: "vault-2", Role: "editor", InvitedByName: "Carol", InvitedByEmail: "c@example.com"}}
	test.testService.apiServer.mutex.Unlock()
	var invitationList cliprotocol.InvitationListResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeListInvitation}, &invitationList)
	if len(invitationList.Sent) != 1 || invitationList.Sent[0].Email != "bob@example.com" || invitationList.Sent[0].VaultName != "Family" || invitationList.Sent[0].State != "pending" ||
		len(invitationList.Received) != 1 || invitationList.Received[0].InvitedByName != "Carol" {
		t.Fatalf("invitations %+v", invitationList)
	}
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeAcceptInvitation, InvitationID: "from-carol"}, nil)
	if failure := test.send(t, cliprotocol.Request{Type: cliprotocol.TypeDeclineInvitation, InvitationID: "from-carol"}, nil); !strings.Contains(failure, "no longer exists") {
		t.Errorf("decline an answered invitation: %q", failure)
	}
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeCancelInvitation, InvitationID: invitationList.Sent[0].ID}, nil)
	if count := test.testService.apiServer.countShareInvitation(); count != 0 {
		t.Errorf("%d invitations after cancelling", count)
	}
	var deviceList cliprotocol.DeviceListResult
	if failure := test.send(t, cliprotocol.Request{Type: cliprotocol.TypeListDevice}, &deviceList); failure != "" {
		t.Errorf("devices: %s", failure)
	}
	if failure := test.send(t, cliprotocol.Request{Type: "vaults.unknown"}, nil); !strings.HasPrefix(failure, cliprotocol.ErrorInvalidRequest) {
		t.Errorf("unknown type: %q", failure)
	}
}

func TestLockEndsCommandLineApprovals(t *testing.T) {
	test := createCommandLineTest(t)
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeListVault}, nil)
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeListVault}, nil)
	if count := test.approver.getCount(); count != 1 {
		t.Fatalf("approved %d times", count)
	}
	var hello cliprotocol.HelloResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeHello}, &hello)
	if !hello.IsApproved {
		t.Fatal("session not approved")
	}
	test.testService.service.Lock()
	if failure := test.send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault}, nil); !strings.HasPrefix(failure, cliprotocol.ErrorLocked) {
		t.Errorf("locked: %q", failure)
	}
	if count := test.approver.getCount(); count != 2 {
		t.Errorf("approved %d times after the lock", count)
	}
}

func TestConvertCommandLineError(t *testing.T) {
	cases := map[error]string{
		ErrLocked:                       cliprotocol.ErrorLocked,
		ErrSignedOut:                    cliprotocol.ErrorSignedOut,
		ErrOffline:                      cliprotocol.ErrorOffline,
		errors.Join(ErrNotFound):        cliprotocol.ErrorNotFound,
		errors.New("permission denied"): cliprotocol.ErrorInternal,
		cliprotocol.NewError(cliprotocol.ErrorDenied, "denied"): cliprotocol.ErrorDenied,
	}
	for err, want := range cases {
		var responseError *cliprotocol.ResponseError
		if !errors.As(convertCommandLineError(err), &responseError) || responseError.Code != want {
			t.Errorf("%v: %v", err, convertCommandLineError(err))
		}
	}
	if convertCommandLineError(nil) != nil {
		t.Error("nil became an error")
	}
}
