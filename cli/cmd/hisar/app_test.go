package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

type fakeApp struct {
	mutex       sync.Mutex
	socketPath  string
	hello       cliprotocol.HelloResult
	helloError  *cliprotocol.ResponseError
	allRequest  []cliprotocol.Request
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
		hello:      cliprotocol.HelloResult{ProtocolVersion: cliprotocol.ProtocolVersion, State: cliprotocol.StateUnlocked, Email: "alice@example.com", Server: "https://hisar.example.com"},
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
		app.allRequest = append(app.allRequest, request)
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

func (app *fakeApp) listAllRequestType() []string {
	app.mutex.Lock()
	defer app.mutex.Unlock()
	allType := []string{}
	for _, request := range app.allRequest {
		allType = append(allType, request.Type)
	}
	return allType
}

func (app *fakeApp) findRequest(requestType string) []cliprotocol.Request {
	app.mutex.Lock()
	defer app.mutex.Unlock()
	allMatch := []cliprotocol.Request{}
	for _, request := range app.allRequest {
		if request.Type == requestType {
			allMatch = append(allMatch, request)
		}
	}
	return allMatch
}

func answerFakeVaultRequest(request cliprotocol.Request) (any, *cliprotocol.ResponseError) {
	switch request.Type {
	case cliprotocol.TypeAccount:
		return cliprotocol.AccountResult{AccountID: "ABC123", Name: "Alice", Email: "alice@example.com", Role: "admin", Server: "https://hisar.example.com"}, nil
	case cliprotocol.TypeListVault:
		return cliprotocol.VaultListResult{Vaults: []cliprotocol.Vault{{ID: "personal-id", Name: "Personal", Type: "personal", Role: "owner"}, {ID: "family-id", Name: "Family", Type: "shared", Role: "reader"}}}, nil
	case cliprotocol.TypeListItem:
		return cliprotocol.ItemListResult{Items: []cliprotocol.Item{
			{ID: "github-id", Revision: "r1", Title: "GitHub", Username: "alice", URLs: []string{"https://github.com"}, HasOneTimePassword: true},
			{ID: "bank-id", Revision: "r2", Title: "Bank", Username: "alice"},
		}}, nil
	case cliprotocol.TypeGetItem:
		if request.ItemID == "github-id" {
			return cliprotocol.ItemResult{Item: cliprotocol.Item{
				ID: "github-id", Revision: "r1", Title: "GitHub", Username: "alice", URLs: []string{"https://github.com"}, Password: "github-password",
				Notes: "work", Fields: []cliprotocol.Field{{Name: "PIN", Value: "1234"}}, Passkeys: []cliprotocol.Passkey{{RelyingPartyID: "github.com", UserName: "alice"}}, HasOneTimePassword: true,
			}}, nil
		}
		return cliprotocol.ItemResult{Item: cliprotocol.Item{ID: request.ItemID, Revision: "r2", Title: "Bank", Username: "alice", Password: "bank-password"}}, nil
	case cliprotocol.TypeGetCode:
		if request.ItemID != "github-id" {
			return nil, cliprotocol.CreateError(cliprotocol.ErrorNotFound, "one-time password not found")
		}
		return cliprotocol.CodeResult{Code: "123456", ExpiresAt: "2026-10-11T10:00:30Z", Period: 30}, nil
	case cliprotocol.TypeCreateVault:
		return cliprotocol.VaultResult{Vault: cliprotocol.Vault{ID: "trip-id", Name: request.Name, Type: "shared", Role: "owner"}}, nil
	case cliprotocol.TypeSaveItem:
		return cliprotocol.ItemResult{Item: cliprotocol.Item{ID: "saved-id", Title: request.Item.Title}}, nil
	case cliprotocol.TypeDeleteItem, cliprotocol.TypeShareVault, cliprotocol.TypeUnshareVault, cliprotocol.TypeRevokeDevice:
		return struct{}{}, nil
	case cliprotocol.TypeImportItem:
		return cliprotocol.ImportResult{SavedCount: len(request.ImportedItems), Failures: []cliprotocol.ImportFailure{}}, nil
	case cliprotocol.TypeListMember:
		return cliprotocol.MemberListResult{Members: []cliprotocol.Member{{AccountID: "ABC123", Name: "Alice", Email: "alice@example.com", Role: "admin"}, {AccountID: "BOB111", Name: "Bob", Email: "bob@example.com", Role: "member"}}}, nil
	case cliprotocol.TypeInvite:
		return cliprotocol.InviteResult{URL: "https://hisar.example.com/invite/token"}, nil
	case cliprotocol.TypeListDevice:
		return cliprotocol.DeviceListResult{Devices: []cliprotocol.Device{{ID: "device-1", Name: "Mac", Platform: "darwin/desktop", LastSeenAt: "2026-10-11T09:00:00Z", IsCurrent: true}}}, nil
	}
	return nil, cliprotocol.CreateError(cliprotocol.ErrorInvalidRequest, "unknown request type")
}

// runCapturingOutput runs a command as main does, with the given input, and returns stdout and stderr.
func runCapturingOutput(t *testing.T, isStandaloneForced bool, input string, allArg ...string) (string, string, error) {
	t.Helper()
	selected, isFound := findCommand(allArg[0])
	if !isFound {
		t.Fatalf("no command %q", allArg[0])
	}
	originalStdout, originalStderr, originalReader := os.Stdout, os.Stderr, stdinReader
	stdoutReader, stdoutWriter, _ := os.Pipe()
	stderrReader, stderrWriter, _ := os.Pipe()
	os.Stdout, os.Stderr = stdoutWriter, stderrWriter
	stdinReader = bufio.NewReader(strings.NewReader(input))
	stdoutChannel, stderrChannel := make(chan string), make(chan string)
	go func() { content, _ := io.ReadAll(stdoutReader); stdoutChannel <- string(content) }()
	go func() { content, _ := io.ReadAll(stderrReader); stderrChannel <- string(content) }()
	err := runCommand(context.Background(), selected, isStandaloneForced, flag.NewFlagSet(selected.name, flag.ContinueOnError), allArg[1:])
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
	app.helloError = cliprotocol.CreateError(cliprotocol.ErrorIntegrationDisabled, "off")
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
	if stdout, _, err := runCapturingOutput(t, false, "", "invite", "carol@example.com"); err != nil || stdout != "https://hisar.example.com/invite/token\n" {
		t.Errorf("invite: %q %v", stdout, err)
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
	allBatch := app.findRequest(cliprotocol.TypeImportItem)
	if len(allBatch) != 2 || len(allBatch[0].ImportedItems) != 10 || len(allBatch[1].ImportedItems) != 2 {
		t.Errorf("batches %d", len(allBatch))
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
		return nil, cliprotocol.CreateError(cliprotocol.ErrorDenied, "Hisar denied the request (it was declined, or not answered within a minute)")
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
	for _, commandName := range []string{"login", "logout", "emergency-kit", "recovery-setup"} {
		_, _, err := runCapturingOutput(t, false, "", commandName)
		if err == nil || !strings.Contains(err.Error(), "hisar --standalone "+commandName) || !strings.Contains(err.Error(), "Hisar app") {
			t.Errorf("%s: %v", commandName, err)
		}
	}
}

func TestStatusShowsTheMode(t *testing.T) {
	app := startFakeApp(t)
	stdout, _, err := runCapturingOutput(t, false, "", "status")
	if err != nil || !strings.Contains(stdout, "mode:    Hisar app ("+app.socketPath+")") || !strings.Contains(stdout, "account: alice@example.com on https://hisar.example.com") || !strings.Contains(stdout, "asks for approval") {
		t.Errorf("app status: %q %v", stdout, err)
	}
	if allType := app.listAllRequestType(); len(allType) != 1 || allType[0] != cliprotocol.TypeHello {
		t.Errorf("status needed more than hello: %v", allType)
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
	app.helloError = cliprotocol.CreateError(cliprotocol.ErrorIntegrationDisabled, "off")
	app.mutex.Unlock()
	if _, _, err := runCapturingOutput(t, false, "", "whoami"); err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Errorf("whoami with the switch off: %v", err)
	}
	if allType := app.listAllRequestType(); len(allType) != 1 || allType[0] != cliprotocol.TypeHello {
		t.Errorf("standalone sent %v", allType)
	}
}

func TestParseGlobalOption(t *testing.T) {
	option, allArg, err := parseGlobalOption([]string{"--standalone", "get", "github", "-f", "password"})
	if err != nil || !option.isStandalone || strings.Join(allArg, " ") != "get github -f password" {
		t.Errorf("%+v %v %v", option, allArg, err)
	}
	if option, _, _ := parseGlobalOption([]string{"--version"}); !option.isVersion {
		t.Error("--version not read")
	}
	if _, _, err := parseGlobalOption([]string{"--other", "ls"}); err == nil {
		t.Error("unknown option accepted")
	}
}
