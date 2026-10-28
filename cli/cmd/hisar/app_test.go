package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
)

// The tests run hisar as if no agent harness were in its chain, wherever they run; setOwnChain says otherwise.
func init() {
	findOwnHarness = func() (agentharness.Session, bool, error) { return agentharness.Session{}, false, nil }
}

func setOwnChain(t *testing.T, session agentharness.Session, isInside bool, err error) {
	t.Helper()
	findOwnHarness = func() (agentharness.Session, bool, error) { return session, isInside, err }
	t.Cleanup(func() {
		findOwnHarness = func() (agentharness.Session, bool, error) { return agentharness.Session{}, false, nil }
	})
}

func runInsideOmp(t *testing.T) {
	t.Helper()
	setOwnChain(t, agentharness.Session{Harness: agentharness.Harness{ID: "omp", Name: "omp"}, PID: 1226, StartTime: 100}, true, nil)
}

type fakeApp struct {
	mutex       sync.Mutex
	socketPath  string
	hello       cliprotocol.HelloResult
	helloError  *cliprotocol.ResponseError
	requests    []cliprotocol.Request
	answer      func(request cliprotocol.Request) (any, *cliprotocol.ResponseError)
	connections int
}

func createShortTempDirectory(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp("/tmp", "hisar-cli-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(directory) })
	return directory
}

func startFakeApp(t *testing.T) *fakeApp {
	t.Helper()
	app := &fakeApp{
		socketPath: filepath.Join(createShortTempDirectory(t), "cli.sock"),
		hello:      cliprotocol.HelloResult{ProtocolVersion: cliprotocol.ProtocolVersion, State: cliprotocol.StateUnlocked, Email: "alice@example.com", Server: "https://hisar.example.com", Features: cliprotocol.ListFeatures()},
		answer:     answerFakeVaultRequest,
	}
	listener, err := net.Listen("unix", app.socketPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go app.serve(connection)
		}
	}()
	t.Setenv(cliprotocol.SocketEnvironment, app.socketPath)
	t.Setenv(standaloneEnvironment, "")
	t.Setenv("HISAR_STATE", filepath.Join(createShortTempDirectory(t), "never", "state.json"))
	return app
}

func (app *fakeApp) serve(connection net.Conn) {
	defer connection.Close()
	app.mutex.Lock()
	app.connections++
	app.mutex.Unlock()
	for {
		var request cliprotocol.Request
		if err := cliprotocol.ReadJSON(connection, &request); err != nil {
			return
		}
		app.mutex.Lock()
		app.requests = append(app.requests, request)
		answer, hello, helloError := app.answer, app.hello, app.helloError
		app.mutex.Unlock()
		var result any
		var responseError *cliprotocol.ResponseError
		if request.Type == cliprotocol.TypeHello {
			result, responseError = hello, helloError
		} else {
			result, responseError = answer(request)
		}
		response := cliprotocol.Response{ID: request.ID, OK: responseError == nil, Error: responseError}
		if responseError == nil {
			response.Result, _ = json.Marshal(result)
		}
		if cliprotocol.WriteJSON(connection, response) != nil {
			return
		}
	}
}

func (app *fakeApp) listRequestTypes() []string {
	app.mutex.Lock()
	defer app.mutex.Unlock()
	types := []string{}
	for _, request := range app.requests {
		types = append(types, request.Type)
	}
	return types
}

func (app *fakeApp) findRequest(requestType string) []cliprotocol.Request {
	app.mutex.Lock()
	defer app.mutex.Unlock()
	matches := []cliprotocol.Request{}
	for _, request := range app.requests {
		if request.Type == requestType {
			matches = append(matches, request)
		}
	}
	return matches
}

func answerFakeVaultRequest(request cliprotocol.Request) (any, *cliprotocol.ResponseError) {
	switch request.Type {
	case cliprotocol.TypeAccount:
		return cliprotocol.AccountResult{AccountID: "ABC123", Name: "Alice", Email: "alice@example.com", Role: "admin", Server: "https://hisar.example.com"}, nil
	case cliprotocol.TypeListVault, cliprotocol.TypeReadVault:
		return cliprotocol.VaultListResult{Vaults: []cliprotocol.Vault{{ID: "personal-id", Name: "Personal", Type: "personal", Role: "owner"}, {ID: "family-id", Name: "Family", Type: "shared", Role: "reader"}}}, nil
	case cliprotocol.TypeListItem, cliprotocol.TypeReadItem:
		return cliprotocol.ItemListResult{Items: []cliprotocol.Item{
			{ID: "github-id", Revision: "r1", Title: "GitHub", Username: "alice", URLs: []string{"https://github.com"}, Tags: []string{"work", "dev"}, HasOneTimePassword: true},
			{ID: "bank-id", Revision: "r2", Title: "Bank", Username: "alice", Tags: []string{"Finance"}},
			{ID: "note-id", Revision: "r3", Category: item.CategorySecureNote, Title: "Recovery codes"},
			{ID: "key-id", Revision: "r4", Category: item.CategorySSHKey, Title: "Deploy key", PublicKey: "ssh-ed25519 AAAA deploy"},
			{ID: "card-id", Revision: "r5", Category: item.CategoryCreditCard, Title: "Visa"},
		}}, nil
	case cliprotocol.TypeGetItem:
		if request.ItemID == "github-id" {
			return cliprotocol.ItemResult{Item: cliprotocol.Item{
				ID: "github-id", Revision: "r1", Title: "GitHub", Username: "alice", URLs: []string{"https://github.com"}, Tags: []string{"work", "dev"}, Password: "github-password",
				Notes: "work", Fields: []cliprotocol.Field{{Name: "PIN", Value: "1234"}}, Passkeys: []cliprotocol.Passkey{{RelyingPartyID: "github.com", UserName: "alice"}}, HasOneTimePassword: true,
			}}, nil
		}
		if request.ItemID == "card-id" {
			return cliprotocol.ItemResult{Item: cliprotocol.Item{ID: "card-id", Revision: "r5", Category: item.CategoryCreditCard, Title: "Visa", Fields: []cliprotocol.Field{
				{Name: item.FieldCardNumber, Value: "4111 1111 1111 1111", Concealed: true}, {Name: item.FieldSecurityCode, Value: "123", Concealed: true},
			}}}, nil
		}
		if request.ItemID == "key-id" {
			return cliprotocol.ItemResult{Item: cliprotocol.Item{ID: "key-id", Revision: "r4", Category: item.CategorySSHKey, Title: "Deploy key", PublicKey: "ssh-ed25519 AAAA deploy"}}, nil
		}
		if request.ItemID == "note-id" {
			return cliprotocol.ItemResult{Item: cliprotocol.Item{ID: "note-id", Revision: "r3", Category: item.CategorySecureNote, Title: "Recovery codes", Notes: "one\ntwo"}}, nil
		}
		return cliprotocol.ItemResult{Item: cliprotocol.Item{ID: request.ItemID, Revision: "r2", Title: "Bank", Username: "alice", Password: "bank-password"}}, nil
	case cliprotocol.TypeGetCode:
		if request.ItemID != "github-id" {
			return nil, cliprotocol.NewError(cliprotocol.ErrorNotFound, "one-time password not found")
		}
		return cliprotocol.CodeResult{Code: "123456", ExpiresAt: "2026-10-11T10:00:30Z", Period: 30}, nil
	case cliprotocol.TypeCreateVault:
		return cliprotocol.VaultResult{Vault: cliprotocol.Vault{ID: "trip-id", Name: request.Name, Type: "shared", Role: "owner"}}, nil
	case cliprotocol.TypeSaveItem:
		return cliprotocol.ItemResult{Item: cliprotocol.Item{ID: "saved-id", Title: request.Item.Title}}, nil
	case cliprotocol.TypeDeleteItem, cliprotocol.TypeShareVault, cliprotocol.TypeUnshareVault, cliprotocol.TypeRevokeDevice:
		return struct{}{}, nil
	case cliprotocol.TypeGetVaultActions:
		return cliprotocol.VaultActionsResult{ImportScope: "reviewed-test-scope"}, nil
	case cliprotocol.TypeImportItem:
		return cliprotocol.ImportResult{SavedCount: len(request.ImportedItems), Failures: []cliprotocol.ImportFailure{}}, nil
	case cliprotocol.TypeListPeople:
		return cliprotocol.PeopleListResult{People: []cliprotocol.Person{
			{AccountID: "ABC123", Name: "Alice", Email: "alice@example.com", Role: "admin", SecurityCode: "3F2A 91C0 7B44 E1D8", IsCurrentUser: true},
			{AccountID: "BOB111", Name: "Bob", Email: "bob@example.com", Role: "member", SecurityCode: "0000 1111 2222 3333", IsKeyChanged: true},
		}}, nil
	case cliprotocol.TypeInvite, cliprotocol.TypeAcceptInvitation, cliprotocol.TypeDeclineInvitation, cliprotocol.TypeCancelInvitation:
		return struct{}{}, nil
	case cliprotocol.TypeListInvitation:
		return cliprotocol.InvitationListResult{
			Received: []cliprotocol.ReceivedInvitation{{ID: "a1b2c3d4-0000-4000-8000-000000000001", VaultID: "vault-9", Role: "editor", InvitedByName: "Carol", InvitedByEmail: "carol@example.com", ExpiresAt: "2026-10-18T09:00:00Z"}},
			Sent:     []cliprotocol.SentInvitation{{ID: "e5f6a7b8-0000-4000-8000-000000000002", VaultID: "family-id", VaultName: "Family", Email: "dave@example.com", Role: "reader", State: "pending", ExpiresAt: "2026-10-18T09:00:00Z"}},
		}, nil
	case cliprotocol.TypeListDevice:
		return cliprotocol.DeviceListResult{Devices: []cliprotocol.Device{{ID: "device-1", Name: "Mac", Platform: "darwin/desktop", LastSeenAt: "2026-10-11T09:00:00Z", IsCurrent: true}}}, nil
	}
	return nil, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, "unknown request type")
}

// runCapturingOutput runs a command as main does, with the given input, and returns stdout and stderr.
func runCapturingOutput(t *testing.T, isStandaloneForced bool, input string, args ...string) (string, string, error) {
	t.Helper()
	selected, isFound := findCommand(args[0])
	if !isFound {
		t.Fatalf("no command %q", args[0])
	}
	originalStdout, originalStderr, originalReader := os.Stdout, os.Stderr, stdinReader
	stdoutReader, stdoutWriter, _ := os.Pipe()
	stderrReader, stderrWriter, _ := os.Pipe()
	os.Stdout, os.Stderr = stdoutWriter, stderrWriter
	stdinReader = bufio.NewReader(strings.NewReader(input))
	stdoutChannel, stderrChannel := make(chan string), make(chan string)
	go func() { content, _ := io.ReadAll(stdoutReader); stdoutChannel <- string(content) }()
	go func() { content, _ := io.ReadAll(stderrReader); stderrChannel <- string(content) }()
	err := runCommand(context.Background(), selected, isStandaloneForced, flag.NewFlagSet(selected.name, flag.ContinueOnError), args[1:])
	stdoutWriter.Close()
	stderrWriter.Close()
	os.Stdout, os.Stderr, stdinReader = originalStdout, originalStderr, originalReader
	return <-stdoutChannel, <-stderrChannel, err
}

func TestChooseModeUsesTheApp(t *testing.T) {
	app := startFakeApp(t)
	mode := chooseMode(false)
	if mode.client == nil {
		t.Fatalf("standalone: %s", mode.reason)
	}
	defer mode.client.Close()
	if mode.client.hello.Email != "alice@example.com" || mode.socketPath != app.socketPath {
		t.Errorf("mode %+v", mode)
	}
}

func TestChooseModeFallsBackToStandalone(t *testing.T) {
	app := startFakeApp(t)
	if mode := chooseMode(true); mode.client != nil || mode.reason != "--standalone was given" {
		t.Errorf("--standalone: %+v", mode)
	}
	t.Setenv(standaloneEnvironment, "1")
	if mode := chooseMode(false); mode.client != nil || mode.reason != "HISAR_STANDALONE is set" {
		t.Errorf("HISAR_STANDALONE: %+v", mode)
	}
	t.Setenv(standaloneEnvironment, "")

	app.mutex.Lock()
	app.helloError = cliprotocol.NewError(cliprotocol.ErrorIntegrationDisabled, "off")
	app.mutex.Unlock()
	if mode := chooseMode(false); mode.client != nil || !strings.Contains(mode.reason, "turned off") {
		t.Errorf("switch off: %+v", mode)
	}

	t.Setenv(cliprotocol.SocketEnvironment, filepath.Join(createShortTempDirectory(t), "missing.sock"))
	if mode := chooseMode(false); mode.client != nil || !strings.Contains(mode.reason, "not running") {
		t.Errorf("missing socket: %+v", mode)
	}

	staleSocket := filepath.Join(createShortTempDirectory(t), "stale.sock")
	listener, _ := net.Listen("unix", staleSocket)
	listener.(*net.UnixListener).SetUnlinkOnClose(false)
	listener.Close()
	t.Setenv(cliprotocol.SocketEnvironment, staleSocket)
	if mode := chooseMode(false); mode.client != nil || !strings.Contains(mode.reason, "not running") {
		t.Errorf("refused connection: %+v", mode)
	}
}

func TestChooseModeRefusesOtherProtocolVersion(t *testing.T) {
	app := startFakeApp(t)
	app.mutex.Lock()
	app.hello.ProtocolVersion = cliprotocol.ProtocolVersion + 1
	app.mutex.Unlock()
	if mode := chooseMode(false); mode.client != nil || !strings.Contains(mode.reason, "protocol version") {
		t.Errorf("mode %+v", mode)
	}
}

func TestCommandsRunThroughTheApp(t *testing.T) {
	app := startFakeApp(t)
	stdout, stderr, err := runCapturingOutput(t, false, "", "vaults")
	if err != nil || !strings.Contains(stdout, "Personal") || !strings.Contains(stdout, "family-id") {
		t.Fatalf("vaults: %q %q %v", stdout, stderr, err)
	}
	if !strings.Contains(stderr, "Approve hisar in this terminal") {
		t.Errorf("no approval notice: %q", stderr)
	}
	stdout, _, err = runCapturingOutput(t, false, "", "whoami")
	if err != nil || !strings.Contains(stdout, "Alice <alice@example.com>") || !strings.Contains(stdout, "role:    admin") {
		t.Errorf("whoami: %q %v", stdout, err)
	}
	stdout, _, err = runCapturingOutput(t, false, "", "ls")
	if err != nil || !strings.Contains(stdout, "GitHub") || !strings.Contains(stdout, "bank-id") {
		t.Errorf("ls: %q %v", stdout, err)
	}
	if lines := strings.Split(stdout, "\n"); len(lines) < 4 || strings.Fields(lines[0])[3] != "TYPE" || !strings.Contains(lines[1], " login ") || !strings.Contains(lines[3], " secure-note ") {
		t.Errorf("ls types: %q", stdout)
	}
	stdout, _, err = runCapturingOutput(t, false, "", "get", "recovery")
	if err != nil || !strings.Contains(stdout, "notes:") || !strings.Contains(stdout, "one\ntwo") || strings.Contains(stdout, "username") {
		t.Errorf("get note: %q %v", stdout, err)
	}
	stdout, _, err = runCapturingOutput(t, false, "", "get", "git", "-f", "password")
	if err != nil || stdout != "github-password\n" {
		t.Errorf("get -f password: %q %v", stdout, err)
	}
	stdout, _, err = runCapturingOutput(t, false, "", "get", "github")
	if err != nil || !strings.Contains(stdout, "PIN:") || !strings.Contains(stdout, "passkey:   github.com (alice)") || !strings.Contains(stdout, "notes:     work") {
		t.Errorf("get: %q %v", stdout, err)
	}
	stdout, _, err = runCapturingOutput(t, false, "", "get", "github", "-f", "code")
	if err != nil || stdout != "123456\n" {
		t.Errorf("get -f code: %q %v", stdout, err)
	}
	if _, _, err = runCapturingOutput(t, false, "", "get", "bank", "-f", "code"); err == nil || !strings.Contains(err.Error(), `item has no field "code"`) {
		t.Errorf("code of an item without one: %v", err)
	}
	if _, _, err = runCapturingOutput(t, false, "", "ls", "-v", "Work"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("unknown vault: %v", err)
	}
	if _, err := os.Stat(os.Getenv("HISAR_STATE")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("app mode touched the state file: %v", err)
	}
	for _, request := range app.findRequest(cliprotocol.TypeGetCode) {
		if request.VaultID != "personal-id" {
			t.Errorf("code request %+v", request)
		}
	}
}

func TestCommandsPreferAVaultIDOverAName(t *testing.T) {
	app := startFakeApp(t)
	app.mutex.Lock()
	app.answer = func(request cliprotocol.Request) (any, *cliprotocol.ResponseError) {
		if request.Type == cliprotocol.TypeListVault || request.Type == cliprotocol.TypeReadVault {
			return cliprotocol.VaultListResult{Vaults: []cliprotocol.Vault{{ID: "mine-id", Name: "team-id", Type: "personal", Role: "owner"}, {ID: "team-id", Name: "Team", Type: "shared", Role: "owner"}}}, nil
		}
		return answerFakeVaultRequest(request)
	}
	app.mutex.Unlock()
	if _, _, err := runCapturingOutput(t, false, "", "ls", "-v", "team-id"); err != nil {
		t.Fatalf("ls by ID: %v", err)
	}
	if requests := app.findRequest(cliprotocol.TypeReadItem); len(requests) != 1 || requests[0].VaultID != "team-id" {
		t.Errorf("a vault named like another vault's ID took its place: item requests %+v", requests)
	}
}

func TestTextOthersWroteCannotDriveTheTerminal(t *testing.T) {
	const hostile = "Eve\x1b]52;c;ZWNobyBoaQ==\x07\u009b"
	app := startFakeApp(t)
	app.mutex.Lock()
	app.answer = func(request cliprotocol.Request) (any, *cliprotocol.ResponseError) {
		switch request.Type {
		case cliprotocol.TypeAccount:
			return cliprotocol.AccountResult{AccountID: "ABC123", Name: hostile, Email: "eve@example.com", Role: "member", Server: "https://hisar.example.com"}, nil
		case cliprotocol.TypeListVault, cliprotocol.TypeReadVault:
			return cliprotocol.VaultListResult{Vaults: []cliprotocol.Vault{{ID: "personal-id", Name: "Personal", Type: "personal", Role: "owner"}, {ID: "shared-id", Name: hostile, Type: "shared", Role: "editor"}}}, nil
		case cliprotocol.TypeListItem, cliprotocol.TypeReadItem:
			return cliprotocol.ItemListResult{Items: []cliprotocol.Item{{ID: "shared-item", Title: hostile, Username: hostile}}}, nil
		case cliprotocol.TypeGetItem:
			return cliprotocol.ItemResult{Item: cliprotocol.Item{ID: "shared-item", Title: hostile, Notes: "line one\n" + hostile, Fields: []cliprotocol.Field{{Name: hostile, Value: hostile}}}}, nil
		}
		return answerFakeVaultRequest(request)
	}
	app.mutex.Unlock()
	for _, args := range [][]string{{"whoami"}, {"vaults"}, {"ls"}, {"get", "shared-item"}} {
		stdout, _, err := runCapturingOutput(t, false, "", args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if strings.ContainsAny(stdout, "\x1b\x07\u009b") || !strings.Contains(stdout, `Eve\x1b]52;c;ZWNobyBoaQ==\x07\x9b`) {
			t.Errorf("%v: %q", args, stdout)
		}
	}
}

func TestListItemsByTagThroughTheApp(t *testing.T) {
	app := startFakeApp(t)
	stdout, _, err := runCapturingOutput(t, false, "", "ls")
	if err != nil || strings.Fields(strings.Split(stdout, "\n")[0])[4] != "TAGS" || !strings.Contains(stdout, " work,dev ") {
		t.Fatalf("ls: %q %v", stdout, err)
	}
	stdout, _, err = runCapturingOutput(t, false, "", "ls", "--tag", "work")
	if err != nil || !strings.Contains(stdout, "GitHub") || strings.Contains(stdout, "Bank") || strings.Contains(stdout, "Recovery") {
		t.Errorf("ls --tag work: %q %v", stdout, err)
	}
	stdout, _, err = runCapturingOutput(t, false, "", "ls", "--tag", "finance")
	if err != nil || !strings.Contains(stdout, "Bank") || strings.Contains(stdout, "GitHub") {
		t.Errorf("ls --tag finance ignoring case: %q %v", stdout, err)
	}
	stdout, _, err = runCapturingOutput(t, false, "", "ls", "--tag", "nothing")
	if err != nil || strings.Count(stdout, "\n") != 1 {
		t.Errorf("ls --tag without a match: %q %v", stdout, err)
	}
	stdout, _, err = runCapturingOutput(t, false, "", "get", "github")
	if err != nil || !strings.Contains(stdout, "tags:      work, dev\n") {
		t.Errorf("get: %q %v", stdout, err)
	}
	stdout, _, err = runCapturingOutput(t, false, "", "get", "recovery")
	if err != nil || strings.Contains(stdout, "tags:") {
		t.Errorf("get an item without tags: %q %v", stdout, err)
	}

	if _, _, err := runCapturingOutput(t, false, "\n\n\n\nedited\n", "edit", "github"); err != nil {
		t.Fatal(err)
	}
	edited := app.findRequest(cliprotocol.TypeSaveItem)[0]
	if edited.Item.Tags == nil || !slices.Equal(*edited.Item.Tags, []string{"work", "dev"}) {
		t.Errorf("edit sent the tags %+v", edited.Item.Tags)
	}

	app.mutex.Lock()
	app.hello.Features = []string{cliprotocol.FeatureSecureNotes, cliprotocol.FeatureCreditCards}
	app.mutex.Unlock()
	if _, _, err := runCapturingOutput(t, false, "", "ls", "--tag", "work"); err == nil || !strings.Contains(err.Error(), "does not list tags") {
		t.Errorf("ls --tag through an older app: %v", err)
	}
	if _, _, err := runCapturingOutput(t, false, "", "ls"); err == nil || !strings.Contains(err.Error(), "unreadable records") {
		t.Errorf("ls silently accepted an older app: %v", err)
	}
}

func TestAddNoteRefusesAnAppWithoutSecureNotes(t *testing.T) {
	app := startFakeApp(t)
	app.mutex.Lock()
	app.hello.Features = nil
	app.mutex.Unlock()
	_, stderr, err := runCapturingOutput(t, false, "Wi-Fi\nNetwork: home\n\n", "add", "--type", "secure-note")
	if err == nil || !strings.Contains(err.Error(), "cannot save secure-note items") {
		t.Fatalf("add --type secure-note through an older app: %v", err)
	}
	if strings.Contains(stderr, "Title") {
		t.Errorf("asked for the note before refusing: %q", stderr)
	}
	if saved := app.findRequest(cliprotocol.TypeSaveItem); len(saved) != 0 {
		t.Errorf("sent the note to an older app: %+v", saved)
	}
	if _, stderr, err := runCapturingOutput(t, false, "Netflix\nalice\nhttps://netflix.com\nnetflix-password\nshared\n", "add"); err != nil || !strings.Contains(stderr, `Saved "Netflix"`) {
		t.Errorf("a login through an older app: %q %v", stderr, err)
	}
}

func TestCreditCardsThroughTheApp(t *testing.T) {
	app := startFakeApp(t)
	input := "Visa\nAlice Smith\n\n4111111111111112\n13/27\n123\n\n1 Main St\nApt 4\n\nSpringfield\n\n12345\nus\n"
	_, stderr, err := runCapturingOutput(t, false, input, "add", "--type", "credit-card")
	if err != nil || !strings.Contains(stderr, `Saved "Visa" to Personal.`) {
		t.Fatalf("add --type credit-card: %q %v", stderr, err)
	}
	if !strings.Contains(stderr, "Note: This doesn't look like a valid card number.") || !strings.Contains(stderr, "Note: Use month and year, like 12/27.") {
		t.Errorf("no warnings: %q", stderr)
	}
	added := app.findRequest(cliprotocol.TypeSaveItem)[0].Item
	wantFields := []cliprotocol.Field{
		{Name: item.FieldCardholder, Value: "Alice Smith"},
		{Name: item.FieldCardType, Value: "Visa"},
		{Name: item.FieldCardNumber, Value: "4111 1111 1111 1112", Concealed: true},
		{Name: item.FieldExpiry, Value: "13/27"},
		{Name: item.FieldSecurityCode, Value: "123", Concealed: true},
		{Name: item.FieldStreet, Value: "1 Main St\nApt 4"},
		{Name: item.FieldCity, Value: "Springfield"},
		{Name: item.FieldPostalCode, Value: "12345"},
		{Name: item.FieldCountry, Value: "US"},
	}
	if added.Category != item.CategoryCreditCard || added.Fields == nil || !reflect.DeepEqual(*added.Fields, wantFields) {
		t.Errorf("add request %+v %+v", added, added.Fields)
	}

	stdout, _, err := runCapturingOutput(t, false, "", "get", "visa", "-f", "security code")
	if err != nil || stdout != "123\n" {
		t.Errorf("get -f security code: %q %v", stdout, err)
	}

	app.mutex.Lock()
	app.hello.Features = []string{cliprotocol.FeatureSecureNotes}
	app.mutex.Unlock()
	_, stderr, err = runCapturingOutput(t, false, input, "add", "--type", "credit-card")
	if err == nil || !strings.Contains(err.Error(), "cannot save credit-card items") || strings.Contains(stderr, "Cardholder") {
		t.Errorf("add a card through an older app: %q %v", stderr, err)
	}
}

func TestIdentitiesThroughTheApp(t *testing.T) {
	app := startFakeApp(t)
	input := "Alice\nAlice\n\nExample\n1990/4/12\nalice@example.com\n\n1 Main St\n\n\n\n\n\nU1234567\n\n\n"
	_, stderr, err := runCapturingOutput(t, false, input, "add", "--type", "identity")
	if err != nil || !strings.Contains(stderr, `Saved "Alice" to Personal.`) {
		t.Fatalf("add --type identity: %q %v", stderr, err)
	}
	if !strings.Contains(stderr, "Birth date (YYYY-MM-DD): ") {
		t.Errorf("no birth date hint: %q", stderr)
	}
	added := app.findRequest(cliprotocol.TypeSaveItem)[0].Item
	wantFields := []cliprotocol.Field{
		{Name: item.FieldFirstName, Value: "Alice"},
		{Name: item.FieldLastName, Value: "Example"},
		{Name: item.FieldBirthDate, Value: "1990-04-12"},
		{Name: item.FieldEmail, Value: "alice@example.com"},
		{Name: item.FieldStreet, Value: "1 Main St"},
		{Name: item.FieldPassportNumber, Value: "U1234567", Concealed: true},
	}
	if added.Category != item.CategoryIdentity || added.Fields == nil || !reflect.DeepEqual(*added.Fields, wantFields) {
		t.Errorf("add request %+v %+v", added, added.Fields)
	}

	app.mutex.Lock()
	app.hello.Features = []string{cliprotocol.FeatureSecureNotes, cliprotocol.FeatureCreditCards}
	app.mutex.Unlock()
	_, stderr, err = runCapturingOutput(t, false, input, "add", "--type", "identity")
	if err == nil || !strings.Contains(err.Error(), "cannot save identity items") || strings.Contains(stderr, "First name") {
		t.Errorf("add an identity through an older app: %q %v", stderr, err)
	}
}

func TestAPICredentialsThroughTheApp(t *testing.T) {
	app := startFakeApp(t)
	input := "Stripe\nStripe\nkey_1\nsk_live_123\napi.stripe.com\n2031/1/5\n"
	_, stderr, err := runCapturingOutput(t, false, input, "add", "--type", "api-credential")
	if err != nil || !strings.Contains(stderr, `Saved "Stripe" to Personal.`) {
		t.Fatalf("add --type api-credential: %q %v", stderr, err)
	}
	if !strings.Contains(stderr, "Expiry (YYYY-MM-DD): ") || strings.Contains(stderr, "sk_live_123") {
		t.Errorf("prompts: %q", stderr)
	}
	added := app.findRequest(cliprotocol.TypeSaveItem)[0].Item
	wantFields := []cliprotocol.Field{
		{Name: item.FieldService, Value: "Stripe"},
		{Name: item.FieldKeyID, Value: "key_1"},
		{Name: item.FieldSecret, Value: "sk_live_123", Concealed: true},
		{Name: item.FieldHost, Value: "api.stripe.com"},
		{Name: item.FieldExpiry, Value: "2031-01-05"},
	}
	if added.Category != item.CategoryAPICredential || added.Fields == nil || !reflect.DeepEqual(*added.Fields, wantFields) {
		t.Errorf("add request %+v %+v", added, added.Fields)
	}

	app.mutex.Lock()
	app.hello.Features = []string{cliprotocol.FeatureSecureNotes, cliprotocol.FeatureCreditCards, cliprotocol.FeatureIdentities}
	app.mutex.Unlock()
	_, stderr, err = runCapturingOutput(t, false, input, "add", "--type", "api-credential")
	if err == nil || !strings.Contains(err.Error(), "cannot save api-credential items") || strings.Contains(stderr, "Service") {
		t.Errorf("add an API credential through an older app: %q %v", stderr, err)
	}
}

func TestAddressesThroughTheApp(t *testing.T) {
	app := startFakeApp(t)
	input := "Home\n1 Main St\nApt 4\n\nSpringfield\nIL\n12345\nus\n"
	_, stderr, err := runCapturingOutput(t, false, input, "add", "--type", "address")
	if err != nil || !strings.Contains(stderr, `Saved "Home" to Personal.`) {
		t.Fatalf("add --type address: %q %v", stderr, err)
	}
	if !strings.Contains(stderr, "Street (a line at a time, then an empty line): ") {
		t.Errorf("no street hint: %q", stderr)
	}
	added := app.findRequest(cliprotocol.TypeSaveItem)[0].Item
	wantFields := []cliprotocol.Field{
		{Name: item.FieldStreet, Value: "1 Main St\nApt 4"},
		{Name: item.FieldCity, Value: "Springfield"},
		{Name: item.FieldRegion, Value: "IL"},
		{Name: item.FieldPostalCode, Value: "12345"},
		{Name: item.FieldCountry, Value: "US"},
	}
	if added.Category != item.CategoryAddress || added.Fields == nil || !reflect.DeepEqual(*added.Fields, wantFields) {
		t.Errorf("add request %+v %+v", added, added.Fields)
	}

	app.mutex.Lock()
	app.hello.Features = []string{cliprotocol.FeatureSecureNotes, cliprotocol.FeatureCreditCards, cliprotocol.FeatureIdentities, cliprotocol.FeatureAPICredentials}
	app.mutex.Unlock()
	_, stderr, err = runCapturingOutput(t, false, input, "add", "--type", "address")
	if err == nil || !strings.Contains(err.Error(), "cannot save address items") || strings.Contains(stderr, "Street") {
		t.Errorf("add an address through an older app: %q %v", stderr, err)
	}
}

func TestSoftwareLicensesThroughTheApp(t *testing.T) {
	app := startFakeApp(t)
	input := "Sketch\nSketch\n100\nTEST-LICENSE-KEY\nAlice Example\nalice@example.com\n2024/3/9\nhttps://www.sketch.com/download\n"
	_, stderr, err := runCapturingOutput(t, false, input, "add", "--type", "software-license")
	if err != nil || !strings.Contains(stderr, `Saved "Sketch" to Personal.`) {
		t.Fatalf("add --type software-license: %q %v", stderr, err)
	}
	if !strings.Contains(stderr, "Purchase date (YYYY-MM-DD): ") || strings.Contains(stderr, "TEST-LICENSE-KEY") {
		t.Errorf("prompts: %q", stderr)
	}
	added := app.findRequest(cliprotocol.TypeSaveItem)[0].Item
	wantFields := []cliprotocol.Field{
		{Name: item.FieldProduct, Value: "Sketch"},
		{Name: item.FieldVersion, Value: "100"},
		{Name: item.FieldLicenseKey, Value: "TEST-LICENSE-KEY", Concealed: true},
		{Name: item.FieldLicensedTo, Value: "Alice Example"},
		{Name: item.FieldEmail, Value: "alice@example.com"},
		{Name: item.FieldPurchaseDate, Value: "2024-03-09"},
		{Name: item.FieldDownloadURL, Value: "https://www.sketch.com/download"},
	}
	if added.Category != item.CategorySoftwareLicense || added.Fields == nil || !reflect.DeepEqual(*added.Fields, wantFields) {
		t.Errorf("add request %+v %+v", added, added.Fields)
	}

	app.mutex.Lock()
	app.hello.Features = []string{cliprotocol.FeatureSecureNotes, cliprotocol.FeatureCreditCards, cliprotocol.FeatureIdentities, cliprotocol.FeatureAPICredentials, cliprotocol.FeatureAddresses}
	app.mutex.Unlock()
	_, stderr, err = runCapturingOutput(t, false, input, "add", "--type", "software-license")
	if err == nil || !strings.Contains(err.Error(), "cannot save software-license items") || strings.Contains(stderr, "Product") {
		t.Errorf("add a software license through an older app: %q %v", stderr, err)
	}
}

func TestServersThroughTheApp(t *testing.T) {
	app := startFakeApp(t)
	input := "Build server\nbuild.example.com:2222\n\ndeploy\nserver-password\nhttps://console.example.com\nNo such key\ndeploy KEY\n"
	_, stderr, err := runCapturingOutput(t, false, input, "add", "--type", "server")
	if err != nil || !strings.Contains(stderr, `Saved "Build server" to Personal.`) {
		t.Fatalf("add --type server: %q %v", stderr, err)
	}
	if !strings.Contains(stderr, `This vault has no SSH key called "No such key".`) || strings.Contains(stderr, "server-password") {
		t.Errorf("prompts: %q", stderr)
	}
	added := app.findRequest(cliprotocol.TypeSaveItem)[0].Item
	wantFields := []cliprotocol.Field{
		{Name: item.FieldHost, Value: "build.example.com"},
		{Name: item.FieldPort, Value: "2222"},
		{Name: item.FieldSSHKey, Value: "key-id"},
	}
	if added.Category != item.CategoryServer || added.Fields == nil || !reflect.DeepEqual(*added.Fields, wantFields) || added.Username != "deploy" ||
		added.Password != "server-password" || !reflect.DeepEqual(added.URLs, []string{"https://console.example.com"}) {
		t.Errorf("add request %+v %+v", added, added.Fields)
	}

	app.mutex.Lock()
	app.hello.Features = []string{cliprotocol.FeatureSecureNotes, cliprotocol.FeatureCreditCards, cliprotocol.FeatureIdentities, cliprotocol.FeatureAPICredentials, cliprotocol.FeatureAddresses, cliprotocol.FeatureSoftwareLicenses}
	app.mutex.Unlock()
	_, stderr, err = runCapturingOutput(t, false, input, "add", "--type", "server")
	if err == nil || !strings.Contains(err.Error(), "cannot save server items") || strings.Contains(stderr, "Host") {
		t.Errorf("add a server through an older app: %q %v", stderr, err)
	}
}

func TestDatabasesThroughTheApp(t *testing.T) {
	app := startFakeApp(t)
	input := "Prod db\nPostgres\ndb.example.com:5432\n\napp\napp\ndb-password\nsslmode=require\n"
	_, stderr, err := runCapturingOutput(t, false, input, "add", "--type", "database")
	if err != nil || !strings.Contains(stderr, `Saved "Prod db" to Personal.`) {
		t.Fatalf("add --type database: %q %v", stderr, err)
	}
	if !strings.Contains(stderr, "Engine (postgresql, mysql, mariadb, sqlserver, oracle, mongodb, redis or another): ") || strings.Contains(stderr, "db-password") {
		t.Errorf("prompts: %q", stderr)
	}
	added := app.findRequest(cliprotocol.TypeSaveItem)[0].Item
	wantFields := []cliprotocol.Field{
		{Name: item.FieldEngine, Value: "postgresql"},
		{Name: item.FieldHost, Value: "db.example.com"},
		{Name: item.FieldPort, Value: "5432"},
		{Name: item.FieldDatabaseName, Value: "app"},
		{Name: item.FieldOptions, Value: "sslmode=require"},
	}
	if added.Category != item.CategoryDatabase || added.Fields == nil || !reflect.DeepEqual(*added.Fields, wantFields) || added.Username != "app" ||
		added.Password != "db-password" || len(added.URLs) != 0 {
		t.Errorf("add request %+v %+v", added, added.Fields)
	}

	app.mutex.Lock()
	app.hello.Features = []string{cliprotocol.FeatureSecureNotes, cliprotocol.FeatureCreditCards, cliprotocol.FeatureIdentities, cliprotocol.FeatureAPICredentials, cliprotocol.FeatureAddresses, cliprotocol.FeatureSoftwareLicenses, cliprotocol.FeatureServers}
	app.mutex.Unlock()
	_, stderr, err = runCapturingOutput(t, false, input, "add", "--type", "database")
	if err == nil || !strings.Contains(err.Error(), "cannot save database items") || strings.Contains(stderr, "Engine") {
		t.Errorf("add a database through an older app: %q %v", stderr, err)
	}
}

func TestEditingAServerKeepsOrRemovesItsSSHKey(t *testing.T) {
	originalReader := stdinReader
	defer func() { stdinReader = originalReader }()
	vaultItems := []item.Item{{ID: "key-id", Overview: item.Overview{Title: "Deploy key", Category: item.CategorySSHKey}}}
	for input, wantID := range map[string]string{"\n\n\n\n\n\n\n": "key-id", "\n\n\n\n\n\nnone\n": ""} {
		stdinReader = bufio.NewReader(strings.NewReader(input))
		vaultItem := item.Item{Overview: item.Overview{Title: "Build", Category: item.CategoryServer, Username: "deploy"}, Details: item.Details{Password: "pw"}}
		item.WriteServer(&vaultItem, item.Server{Host: "build.example.com", SSHKeyID: "key-id"}, nil)
		if err := promptServer(&vaultItem, vaultItems); err != nil {
			t.Fatal(err)
		}
		if server, _ := item.ReadServer(&vaultItem); server.SSHKeyID != wantID || vaultItem.Details.Password != "pw" {
			t.Errorf("input %q: SSH key %q, password %q", input, server.SSHKeyID, vaultItem.Details.Password)
		}
	}
}

func TestEditingAServerTakesAnIDThatTheLinkedKeyHasAsItsTitle(t *testing.T) {
	const keyID = "12345678-1234-4234-8234-123456789abc"
	originalReader := stdinReader
	defer func() { stdinReader = originalReader }()
	vaultItems := []item.Item{
		{ID: "shadow-id", Overview: item.Overview{Title: keyID, Category: item.CategorySSHKey}},
		{ID: keyID, Overview: item.Overview{Title: "Deploy key", Category: item.CategorySSHKey}},
	}
	stdinReader = bufio.NewReader(strings.NewReader("\n\n\n\n\n\n" + keyID + "\n"))
	vaultItem := item.Item{Overview: item.Overview{Title: "Build", Category: item.CategoryServer}}
	item.WriteServer(&vaultItem, item.Server{Host: "build.example.com", SSHKeyID: "shadow-id"}, nil)
	if err := promptServer(&vaultItem, vaultItems); err != nil {
		t.Fatal(err)
	}
	if server, _ := item.ReadServer(&vaultItem); server.SSHKeyID != keyID {
		t.Errorf("typed ID %s linked %q", keyID, server.SSHKeyID)
	}
}

func TestServerSSHKeyTakesAnIDThatAnotherKeyHasAsItsTitle(t *testing.T) {
	const keyID = "12345678-1234-4234-8234-123456789abc"
	app := startFakeApp(t)
	app.mutex.Lock()
	app.answer = func(request cliprotocol.Request) (any, *cliprotocol.ResponseError) {
		if request.Type == cliprotocol.TypeListItem {
			return cliprotocol.ItemListResult{Items: []cliprotocol.Item{
				{ID: keyID, Category: item.CategorySSHKey, Title: "Deploy"}, {ID: "shadow-id", Category: item.CategorySSHKey, Title: keyID},
				{ID: "twin-a", Category: item.CategorySSHKey, Title: "Twin"}, {ID: "twin-b", Category: item.CategorySSHKey, Title: "twin"},
			}}, nil
		}
		return answerFakeVaultRequest(request)
	}
	app.mutex.Unlock()
	input := "Build server\nbuild.example.com\n\ndeploy\nserver-password\n\ntwin\n" + keyID + "\n"
	_, stderr, err := runCapturingOutput(t, false, input, "add", "--type", "server")
	if err != nil {
		t.Fatalf("add --type server: %q %v", stderr, err)
	}
	if !strings.Contains(stderr, "twin-a") || !strings.Contains(stderr, "twin-b") {
		t.Errorf("two keys titled twin, without their IDs: %q", stderr)
	}
	added := app.findRequest(cliprotocol.TypeSaveItem)[0].Item
	if added.Fields == nil || !slices.Contains(*added.Fields, cliprotocol.Field{Name: item.FieldSSHKey, Value: keyID}) {
		t.Errorf("the server links %+v, not the key with ID %s", added.Fields, keyID)
	}
}

func TestEditingAnAddressKeepsItsStreetOnAnEmptyLine(t *testing.T) {
	originalReader := stdinReader
	defer func() { stdinReader = originalReader }()
	stdinReader = bufio.NewReader(strings.NewReader("\n\nChicago\n\n\n\n"))
	vaultItem := item.Item{Overview: item.Overview{Title: "Home", Category: item.CategoryAddress}}
	item.WriteAddress(&vaultItem, item.Address{Street: "1 Main St\nApt 4", City: "Springfield"}, []item.Field{{Name: "Gate code", Value: "1234"}})
	if err := promptAddress(&vaultItem); err != nil {
		t.Fatal(err)
	}
	address, otherFields := item.ReadAddress(&vaultItem)
	if address != (item.Address{Street: "1 Main St\nApt 4", City: "Chicago"}) || len(otherFields) != 1 {
		t.Errorf("edited address = %+v, other fields %+v", address, otherFields)
	}
}

func TestAddAndEditFollowTheItemType(t *testing.T) {
	app := startFakeApp(t)
	_, stderr, err := runCapturingOutput(t, false, "Router\n", "add", "--type", "wireless-router")
	if err == nil || !strings.Contains(err.Error(), `unknown item type "wireless-router"; use one of login, secure-note, credit-card, identity, api-credential, address, software-license, server, database`) {
		t.Errorf("add an unknown type: %v", err)
	}
	if strings.Contains(stderr, "Title") {
		t.Errorf("asked for an item of an unknown type: %q", stderr)
	}
	_, stderr, err = runCapturingOutput(t, false, "\n\n\n\n\n", "edit", "deploy")
	if err == nil || !strings.Contains(err.Error(), "hisar cannot edit ssh-key items") || strings.Contains(stderr, "Username") {
		t.Errorf("edit an SSH key: %q %v", stderr, err)
	}
	if saved := app.findRequest(cliprotocol.TypeSaveItem); len(saved) != 0 {
		t.Errorf("saved %+v", saved)
	}
	if _, _, err := runCapturingOutput(t, false, "Netflix\nalice\n\npw\n\n", "add", "--type", "login"); err != nil {
		t.Fatal(err)
	}
	if login := app.findRequest(cliprotocol.TypeSaveItem)[0]; login.Item.Category != item.CategoryLogin || login.Item.Username != "alice" {
		t.Errorf("add --type login request %+v", login.Item)
	}
}

func TestWriteCommandsRunThroughTheApp(t *testing.T) {
	app := startFakeApp(t)
	if _, stderr, err := runCapturingOutput(t, false, "Netflix\nalice\nhttps://netflix.com\nnetflix-password\nshared\n", "add"); err != nil || !strings.Contains(stderr, `Saved "Netflix" to Personal.`) {
		t.Fatalf("add: %q %v", stderr, err)
	}
	added := app.findRequest(cliprotocol.TypeSaveItem)[0]
	if added.VaultID != "personal-id" || added.Item.ID != "" || added.Item.Title != "Netflix" || added.Item.Password != "netflix-password" || added.Item.URLs[0] != "https://netflix.com" {
		t.Errorf("add request %+v %+v", added, added.Item)
	}

	if _, stderr, err := runCapturingOutput(t, false, "\n\n\n\nedited\n", "edit", "github"); err != nil || !strings.Contains(stderr, `Saved "GitHub".`) {
		t.Fatalf("edit: %q %v", stderr, err)
	}
	edited := app.findRequest(cliprotocol.TypeSaveItem)[1]
	if edited.Item.ID != "github-id" || edited.Item.Revision != "r1" || edited.Item.Password != "github-password" || edited.Item.Notes != "edited" {
		t.Errorf("edit request %+v", edited.Item)
	}

	if _, stderr, err := runCapturingOutput(t, false, "Wi-Fi\nNetwork: home\n\nPassword: correct horse\n.\nignored\n", "add", "--type", "secure-note"); err != nil || !strings.Contains(stderr, `Saved "Wi-Fi" to Personal.`) {
		t.Fatalf("add --type secure-note: %q %v", stderr, err)
	}
	if note := app.findRequest(cliprotocol.TypeSaveItem)[2]; note.Item.Category != item.CategorySecureNote || note.Item.Notes != "Network: home\n\nPassword: correct horse" || note.Item.Username != "" || note.Item.Password != "" {
		t.Errorf("add --type secure-note request %+v", note.Item)
	}
	if _, stderr, err := runCapturingOutput(t, false, "\n.\n", "edit", "recovery"); err != nil || strings.Contains(stderr, "Username") || strings.Contains(stderr, "Password") {
		t.Fatalf("edit note keeping its notes: %q %v", stderr, err)
	}
	if kept := app.findRequest(cliprotocol.TypeSaveItem)[3]; kept.Item.ID != "note-id" || kept.Item.Notes != "one\ntwo" || kept.Item.Category != item.CategorySecureNote {
		t.Errorf("edit note request %+v", kept.Item)
	}
	if _, _, err := runCapturingOutput(t, false, "Codes\n\n1. 4821-9930\n\n2. 1177-0042\n\n", "edit", "recovery"); err != nil {
		t.Fatal(err)
	}
	if replaced := app.findRequest(cliprotocol.TypeSaveItem)[4]; replaced.Item.Title != "Codes" || replaced.Item.Notes != "1. 4821-9930\n\n2. 1177-0042" {
		t.Errorf("edit note piped without a final \".\" %+v", replaced.Item)
	}

	if _, _, err := runCapturingOutput(t, false, "y\n", "rm", "bank"); err != nil {
		t.Fatal(err)
	}
	if deleted := app.findRequest(cliprotocol.TypeDeleteItem); len(deleted) != 1 || deleted[0].ItemID != "bank-id" {
		t.Errorf("rm requests %+v", deleted)
	}
	if _, _, err := runCapturingOutput(t, false, "n\n", "rm", "github"); err != nil || len(app.findRequest(cliprotocol.TypeDeleteItem)) != 1 {
		t.Errorf("declined rm deleted: %v", err)
	}

	if _, _, err := runCapturingOutput(t, false, "", "share", "Personal", "bob@example.com"); err == nil || !strings.Contains(err.Error(), "personal vaults cannot be shared") {
		t.Errorf("share personal: %v", err)
	}
	if _, _, err := runCapturingOutput(t, false, "", "share", "Family", "bob@example.com", "-r", "reader"); err != nil {
		t.Errorf("share: %v", err)
	}
	if shared := app.findRequest(cliprotocol.TypeShareVault); len(shared) != 1 || shared[0].VaultID != "family-id" || shared[0].Role != "reader" || shared[0].Email != "bob@example.com" {
		t.Errorf("share requests %+v", shared)
	}
	if _, _, err := runCapturingOutput(t, false, "", "unshare", "Family", "BOB@example.com"); err != nil {
		t.Errorf("unshare: %v", err)
	}
	if unshared := app.findRequest(cliprotocol.TypeUnshareVault); len(unshared) != 1 || unshared[0].AccountID != "BOB111" {
		t.Errorf("unshare requests %+v", unshared)
	}
	if stdout, stderr, err := runCapturingOutput(t, false, "", "invite", "carol@example.com"); err != nil || stdout != "" || !strings.HasSuffix(stderr, "Invitation sent to carol@example.com.\n") {
		t.Errorf("invite: %q %q %v", stdout, stderr, err)
	}
	if invited := app.findRequest(cliprotocol.TypeInvite); len(invited) != 1 || invited[0].Email != "carol@example.com" || invited[0].Role != "" {
		t.Errorf("invite requests %+v", invited)
	}
	if _, stderr, err := runCapturingOutput(t, false, "Family\nerin@example.com\n", "share"); err != nil || !strings.Contains(stderr, `Invitation sent to erin@example.com for "Family" as editor.`) {
		t.Errorf("share with prompts: %q %v", stderr, err)
	}
	if _, _, err := runCapturingOutput(t, false, "", "share", "Family", "erin@example.com", "-r", "admin"); err == nil || !strings.Contains(err.Error(), "owner, editor or reader") {
		t.Errorf("share with an invalid role: %v", err)
	}
	stdout, _, err := runCapturingOutput(t, false, "", "people")
	if err != nil || !strings.Contains(stdout, "Alice (you)") || !strings.Contains(stdout, "0000 1111 2222 3333 (key changed)") {
		t.Errorf("people: %q %v", stdout, err)
	}
	stdout, _, err = runCapturingOutput(t, false, "", "invitations")
	if err != nil || !strings.Contains(stdout, "a1b2c3d4") || !strings.Contains(stdout, "Carol <carol@example.com>") || !strings.Contains(stdout, "dave@example.com") || !strings.Contains(stdout, "Family") {
		t.Errorf("invitations: %q %v", stdout, err)
	}
	if _, stderr, err := runCapturingOutput(t, false, "\n", "invitations", "accept"); err != nil || !strings.Contains(stderr, "Accepted.") {
		t.Errorf("accept the only invitation: %q %v", stderr, err)
	}
	if accepted := app.findRequest(cliprotocol.TypeAcceptInvitation); len(accepted) != 1 || accepted[0].InvitationID != "a1b2c3d4-0000-4000-8000-000000000001" {
		t.Errorf("accept requests %+v", accepted)
	}
	if _, _, err := runCapturingOutput(t, false, "", "invitations", "decline", "a1b2"); err != nil || app.findRequest(cliprotocol.TypeDeclineInvitation)[0].InvitationID != "a1b2c3d4-0000-4000-8000-000000000001" {
		t.Errorf("decline by prefix: %v", err)
	}
	if _, _, err := runCapturingOutput(t, false, "", "invitations", "cancel", "e5f6"); err != nil || app.findRequest(cliprotocol.TypeCancelInvitation)[0].InvitationID != "e5f6a7b8-0000-4000-8000-000000000002" {
		t.Errorf("cancel by prefix: %v", err)
	}
	if _, _, err := runCapturingOutput(t, false, "", "invitations", "cancel", "a1b2"); err == nil || !strings.Contains(err.Error(), "no invitation") {
		t.Errorf("cancel a received invitation: %v", err)
	}
	if _, _, err := runCapturingOutput(t, false, "", "invitations", "forward", "a1b2"); err == nil || !strings.Contains(err.Error(), "unknown action") {
		t.Errorf("unknown action: %v", err)
	}
	if stdout, _, err := runCapturingOutput(t, false, "", "devices"); err != nil || !strings.Contains(stdout, "Mac (this device)") {
		t.Errorf("devices: %q %v", stdout, err)
	}
	if _, _, err := runCapturingOutput(t, false, "", "revoke-device", "device-2"); err != nil || app.findRequest(cliprotocol.TypeRevokeDevice)[0].DeviceID != "device-2" {
		t.Errorf("revoke-device: %v", err)
	}
	if _, _, err := runCapturingOutput(t, false, "", "vault-create", "Trip"); err != nil || app.findRequest(cliprotocol.TypeCreateVault)[0].Name != "Trip" {
		t.Errorf("vault-create: %v", err)
	}
}

func TestImportThroughTheAppSendsBatches(t *testing.T) {
	app := startFakeApp(t)
	var csv strings.Builder
	csv.WriteString("name,url,username,password,note\n")
	csv.WriteString("Bank,,alice,already-there,\n")
	for index := range 12 {
		csv.WriteString("Site " + string(rune('A'+index)) + ",https://site" + string(rune('a'+index)) + ".example.com,alice,password-" + string(rune('a'+index)) + ",\n")
	}
	path := filepath.Join(t.TempDir(), "export.csv")
	os.WriteFile(path, []byte(csv.String()), 0o600)
	_, stderr, err := runCapturingOutput(t, false, "", "import", path, "-y")
	if err != nil || !strings.Contains(stderr, "Imported 12 items into Personal.") {
		t.Fatalf("import: %q %v", stderr, err)
	}
	batches := app.findRequest(cliprotocol.TypeImportItem)
	if len(batches) != 2 || len(batches[0].ImportedItems) != 10 || len(batches[1].ImportedItems) != 2 {
		t.Errorf("batches %d", len(batches))
	}
	if _, err := os.Stat(path); err != nil {
		t.Error("import -y deleted the export")
	}
	if _, _, err := runCapturingOutput(t, false, "", "import", path, "-y", "-v", "Family"); err == nil || !strings.Contains(err.Error(), "you can only read") {
		t.Errorf("import into a read-only vault: %v", err)
	}
}

func TestDeniedRequestIsReportedNotRetriedStandalone(t *testing.T) {
	app := startFakeApp(t)
	app.mutex.Lock()
	app.answer = func(cliprotocol.Request) (any, *cliprotocol.ResponseError) {
		return nil, cliprotocol.NewError(cliprotocol.ErrorDenied, "Hisar denied the request (it was not answered within 2 minutes)")
	}
	app.mutex.Unlock()
	_, _, err := runCapturingOutput(t, false, "", "vaults")
	if err == nil || !strings.Contains(err.Error(), "Hisar denied the request") {
		t.Errorf("denied: %v", err)
	}
	if _, err := os.Stat(os.Getenv("HISAR_STATE")); !errors.Is(err, os.ErrNotExist) {
		t.Error("a denied request fell back to standalone")
	}
}

func TestStandaloneOnlyCommandsPointToTheApp(t *testing.T) {
	startFakeApp(t)
	for _, commandName := range []string{"login", "logout", "emergency-kit", "account"} {
		_, _, err := runCapturingOutput(t, false, "", commandName)
		if err == nil || !strings.Contains(err.Error(), "hisar --standalone "+commandName) || !strings.Contains(err.Error(), "Hisar app") {
			t.Errorf("%s: %v", commandName, err)
		}
	}
}

func TestRemovedRecoveryCommandsPointToTheEmergencyKit(t *testing.T) {
	startFakeApp(t)
	removedCommands := []string{"recover", "recovery-setup", "recovery-enable", "recovery-start", "recovery-list", "recovery-complete"}
	for _, commandName := range removedCommands {
		for _, isStandaloneForced := range []bool{false, true} {
			_, _, err := runCapturingOutput(t, isStandaloneForced, "", commandName, "anything")
			if err == nil || err.Error() != "account recovery was removed; sign in with your Emergency Kit" {
				t.Errorf("%s (standalone %v): %v", commandName, isStandaloneForced, err)
			}
		}
	}
	var usage strings.Builder
	printUsage(&usage)
	for _, line := range strings.Split(usage.String(), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		for _, commandName := range removedCommands {
			if fields[0] == commandName {
				t.Errorf("usage lists removed recovery command %s", commandName)
			}
		}
	}
}

func TestStatusShowsTheMode(t *testing.T) {
	app := startFakeApp(t)
	stdout, _, err := runCapturingOutput(t, false, "", "status")
	if err != nil || !strings.Contains(stdout, "mode:    Hisar app ("+app.socketPath+")") || !strings.Contains(stdout, "account: alice@example.com on https://hisar.example.com") || !strings.Contains(stdout, "asks for approval") {
		t.Errorf("app status: %q %v", stdout, err)
	}
	if types := app.listRequestTypes(); len(types) != 1 || types[0] != cliprotocol.TypeHello {
		t.Errorf("status needed more than hello: %v", types)
	}
	app.mutex.Lock()
	app.hello.State, app.hello.IsApproved = cliprotocol.StateLocked, false
	app.mutex.Unlock()
	if stdout, _, _ = runCapturingOutput(t, false, "", "status"); !strings.Contains(stdout, "locked; the next command asks to unlock it") {
		t.Errorf("locked status: %q", stdout)
	}

	stdout, _, err = runCapturingOutput(t, true, "", "status")
	if err != nil || !strings.Contains(stdout, "mode:    standalone (--standalone was given)") || !strings.Contains(stdout, "not signed in") {
		t.Errorf("standalone status: %q %v", stdout, err)
	}
}

func TestStandaloneModeKeepsItsOwnSignIn(t *testing.T) {
	app := startFakeApp(t)
	app.mutex.Lock()
	app.helloError = cliprotocol.NewError(cliprotocol.ErrorIntegrationDisabled, "off")
	app.mutex.Unlock()
	if _, _, err := runCapturingOutput(t, false, "", "whoami"); err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Errorf("whoami with the switch off: %v", err)
	}
	if types := app.listRequestTypes(); len(types) != 1 || types[0] != cliprotocol.TypeHello {
		t.Errorf("standalone sent %v", types)
	}
}

func TestParseGlobalOption(t *testing.T) {
	option, args, err := parseGlobalOption([]string{"--standalone", "get", "github", "-f", "password"})
	if err != nil || !option.isStandalone || strings.Join(args, " ") != "get github -f password" {
		t.Errorf("%+v %v %v", option, args, err)
	}
	if option, _, _ := parseGlobalOption([]string{"--version"}); !option.isVersion {
		t.Error("--version not read")
	}
	if _, _, err := parseGlobalOption([]string{"--other", "ls"}); err == nil {
		t.Error("unknown option accepted")
	}
}

func TestNoStandaloneInsideAHarnessOrAnUnknownChain(t *testing.T) {
	cases := []struct {
		name     string
		setChain func(t *testing.T)
		refusal  string
	}{
		{"inside omp", runInsideOmp, "hisar runs inside omp, so it uses the Hisar app only."},
		{"unknown chain", func(t *testing.T) { setOwnChain(t, agentharness.Session{}, false, agentharness.ErrUnknownChain) },
			"hisar couldn't check which program runs it, so it uses the Hisar app only. Try again."},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.setChain(t)
			statePath := filepath.Join(t.TempDir(), "state.json")
			t.Setenv("HISAR_STATE", statePath)
			t.Setenv(noLaunchEnvironment, "1")
			refusal := testCase.refusal

			app := startFakeApp(t)
			if stdout, stderr, err := runCapturingOutput(t, false, "", "vaults"); err != nil || !strings.Contains(stdout, "Personal") {
				t.Errorf("through the app: %q %q %v", stdout, stderr, err)
			}
			if _, _, err := runCapturingOutput(t, true, "", "ls"); err == nil || err.Error() != refusal {
				t.Errorf("--standalone ls: %v", err)
			}
			t.Setenv(standaloneEnvironment, "1")
			if _, _, err := runCapturingOutput(t, false, "", "ls"); err == nil || err.Error() != refusal {
				t.Errorf("HISAR_STANDALONE=1 hisar ls: %v", err)
			}
			t.Setenv(standaloneEnvironment, "")
			t.Setenv(cliprotocol.SocketEnvironment, filepath.Join(createShortTempDirectory(t), "missing.sock"))
			for _, args := range [][]string{{"ls"}, {"read", "hisar://Personal/GitHub/password"}, {"whoami"}} {
				if _, _, err := runCapturingOutput(t, false, "master password\n", args...); err == nil || err.Error() != refusal {
					t.Errorf("%q with the app not running: %v", args, err)
				}
			}
			if stdout, _, err := runCapturingOutput(t, false, "", "status"); err != nil || !strings.HasSuffix(stdout, "\n"+refusal+"\n") {
				t.Errorf("status with the app not running: %q %v", stdout, err)
			}
			if _, err := os.Stat(statePath); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("a standalone state was written: %v", err)
			}
			if types := app.listRequestTypes(); slices.Contains(types, cliprotocol.TypeListItem) {
				t.Errorf("a refused command reached the app: %v", types)
			}
		})
	}
}

// TestSignupInsideAHarnessReachesNoServer: a refused signup must not create the account first, or the invite is used
// up and the Secret Key is never shown.
func TestSignupInsideAHarnessReachesNoServer(t *testing.T) {
	cases := []struct {
		name     string
		setChain func(t *testing.T)
	}{
		{"inside omp", runInsideOmp},
		{"unknown chain", func(t *testing.T) { setOwnChain(t, agentharness.Session{}, false, agentharness.ErrUnknownChain) }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.setChain(t)
			var requestPaths []string
			var mutex sync.Mutex
			apiServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				mutex.Lock()
				requestPaths = append(requestPaths, request.URL.Path)
				mutex.Unlock()
				writer.WriteHeader(http.StatusInternalServerError)
			}))
			defer apiServer.Close()
			t.Setenv("HISAR_STATE", filepath.Join(t.TempDir(), "state.json"))
			t.Setenv(standaloneEnvironment, "1")
			for _, args := range [][]string{{"signup", apiServer.URL + "/invite/token"}, {"login"}} {
				input := "a@example.com\nA\npw-long-enough\npw-long-enough\n"
				if args[0] == "login" {
					input = apiServer.URL + "\na@example.com\nA3-ABCDEF-ABCDEF-ABCDE-FGHIJ-KLMNO-PQRST\npw-long-enough\n"
				}
				if _, _, err := runCapturingOutput(t, false, input, args...); err == nil {
					t.Errorf("%s ran inside a harness", args[0])
				}
			}
			mutex.Lock()
			defer mutex.Unlock()
			if len(requestPaths) != 0 {
				t.Errorf("the server got %q", requestPaths)
			}
		})
	}
}

func TestSignupRefusesAnExistingKitPathBeforeReachingTheServer(t *testing.T) {
	var requestPaths []string
	var mutex sync.Mutex
	apiServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mutex.Lock()
		requestPaths = append(requestPaths, request.URL.Path)
		mutex.Unlock()
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer apiServer.Close()
	t.Setenv("HISAR_STATE", filepath.Join(t.TempDir(), "state.json"))
	t.Setenv(standaloneEnvironment, "1")
	t.Setenv("HISAR_SECRET_KEY", "A3-ABCDEF-ABCDEF-ABCDE-FGHIJ-KLMNO-PQRST")
	kitPath := filepath.Join(t.TempDir(), "Hisar Emergency Kit.pdf")
	if err := os.WriteFile(kitPath, []byte("an earlier kit"), 0o600); err != nil {
		t.Fatal(err)
	}
	input := "a@example.com\nA\npw-long-enough\npw-long-enough\n"
	if _, _, err := runCapturingOutput(t, false, input, "signup", "-o", kitPath, "-y", apiServer.URL+"/invite/token"); err == nil {
		t.Fatal("signup accepted a kit path that is taken")
	}
	if content, err := os.ReadFile(kitPath); err != nil || string(content) != "an earlier kit" {
		t.Fatalf("the earlier kit changed: %q %v", content, err)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if len(requestPaths) != 0 {
		t.Errorf("signup reached the server before checking its kit path: %q", requestPaths)
	}
}

func TestSignupWithoutInviteStopsOnInviteOnlyServer(t *testing.T) {
	for _, mode := range []string{"invite", "setup"} {
		t.Run(mode, func(t *testing.T) {
			var requestPaths []string
			var mutex sync.Mutex
			apiServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				mutex.Lock()
				requestPaths = append(requestPaths, request.URL.Path)
				mutex.Unlock()
				writer.Write([]byte(`{"mode":"` + mode + `"}`))
			}))
			defer apiServer.Close()
			t.Setenv("HISAR_STATE", filepath.Join(t.TempDir(), "state.json"))
			t.Setenv(standaloneEnvironment, "1")
			input := "a@example.com\nA\npw-long-enough\npw-long-enough\n"
			kitPath := filepath.Join(t.TempDir(), "kit.pdf")
			_, stderr, err := runCapturingOutput(t, false, input, "signup", "-o", kitPath, "-server", apiServer.URL)
			if err == nil || err.Error() != "This server needs an invitation. Run: hisar signup <invite link>" {
				t.Fatalf("signup on an invite-only server: %v", err)
			}
			if strings.Contains(stderr, "Email") || strings.Contains(stderr, "assword") {
				t.Errorf("signup asked for details before learning the server needs an invitation: %q", stderr)
			}
			mutex.Lock()
			defer mutex.Unlock()
			if len(requestPaths) != 1 || requestPaths[0] != "/api/v1/auth/register/policy" {
				t.Errorf("the server got %q", requestPaths)
			}
		})
	}
}

func TestStatusInsideAHarness(t *testing.T) {
	runInsideOmp(t)
	t.Setenv(noLaunchEnvironment, "1")
	socketPath := filepath.Join(createShortTempDirectory(t), "missing.sock")
	t.Setenv(cliprotocol.SocketEnvironment, socketPath)
	stdout, _, err := runCapturingOutput(t, false, "", "status")
	expected := "mode:    none (the Hisar app is not running, or does not offer " + socketPath + ")\nhisar runs inside omp, so it uses the Hisar app only.\n"
	if err != nil || stdout != expected {
		t.Errorf("status with the app not running: %q %v", stdout, err)
	}

	app := startFakeApp(t)
	if stdout, _, err := runCapturingOutput(t, false, "", "status"); err != nil || !strings.Contains(stdout, "mode:    Hisar app ("+app.socketPath+")") {
		t.Errorf("status through the app: %q %v", stdout, err)
	}
}
