package app

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/sharelink"
)

// terminalAnswer is what a person types once prompt shows.
type terminalAnswer struct {
	prompt string
	text   string
}

type lockedBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (buffer *lockedBuffer) Write(data []byte) (int, error) {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	return buffer.buffer.Write(data)
}

func (buffer *lockedBuffer) String() string {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	return buffer.buffer.String()
}

// runCommandLineInTerminal runs the built hisar with a terminal, through script(1) as scripts/test-end-to-end.sh does, and
// types each answer once its prompt shows, so hisar's wizards run as they do for a person.
func runCommandLineInTerminal(t *testing.T, binaryPath, socketPath, statePath string, answers []terminalAnswer, args ...string) string {
	t.Helper()
	var command *exec.Cmd
	if runtime.GOOS == "darwin" {
		command = exec.Command("script", append([]string{"-q", "/dev/null", binaryPath}, args...)...)
	} else {
		words := []string{}
		for _, word := range append([]string{binaryPath}, args...) {
			words = append(words, "'"+strings.ReplaceAll(word, "'", `'\''`)+"'")
		}
		command = exec.Command("script", "-qec", strings.Join(words, " "), "/dev/null")
	}
	command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), cliprotocol.SocketEnvironment + "=" + socketPath, "HISAR_STATE=" + statePath, "HISAR_SECRET_KEY=-", "HISAR_NO_LAUNCH=1"}
	output := &lockedBuffer{}
	command.Stdout, command.Stderr = output, output
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	position := 0
	for _, answer := range answers {
		deadline := time.Now().Add(time.Minute)
		for {
			text := output.String()
			if index := strings.Index(text[position:], answer.prompt); index >= 0 {
				position += index + len(answer.prompt)
				break
			}
			if time.Now().After(deadline) {
				command.Process.Kill()
				t.Fatalf("hisar %s never asked %q:\n%s", strings.Join(args, " "), answer.prompt, text)
			}
			time.Sleep(20 * time.Millisecond)
		}
		if _, err := stdin.Write([]byte(answer.text + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err = <-done:
	case <-time.After(time.Minute):
		command.Process.Kill()
		t.Fatalf("hisar %s did not finish:\n%s", strings.Join(args, " "), output.String())
	}
	if err != nil {
		t.Fatalf("hisar %s: %v\n%s", strings.Join(args, " "), err, output.String())
	}
	return output.String()
}

// checkSelectionThroughApp runs the built hisar against the app where a vault name, item titles and an SSH key title
// repeat or equal another object's ID: names that several objects have are refused with their IDs, and an ID always
// selects its own object.
func checkSelectionThroughApp(t *testing.T, service *Service, binaryPath, socketPath, statePath string) {
	t.Helper()
	if output, err := runCommandLineBinary(t, binaryPath, socketPath, statePath, "", "vaults"); err != nil {
		t.Fatalf("vaults, which unlocks Hisar through the approval: %v\n%s", err, output)
	}
	personalID := findPersonalVaultID(t, service)
	shared, err := service.CreateVault("PERSONAL")
	if err != nil {
		t.Fatal(err)
	}
	save := func(vaultID, title, password string) string {
		t.Helper()
		saved, err := service.SaveItem(vaultID, ItemDetail{Title: title, Password: password})
		if err != nil {
			t.Fatal(err)
		}
		return saved.ID
	}
	firstDeploy, secondDeploy := save(personalID, "Deploy", "first-deploy"), save(personalID, "DEPLOY", "second-deploy")
	target := save(personalID, "Target", "target-password")
	save(personalID, target, "shadow-password")
	run := func(isSuccess bool, input string, args ...string) string {
		t.Helper()
		output, err := runCommandLineBinary(t, binaryPath, socketPath, statePath, input, args...)
		if (err == nil) != isSuccess {
			t.Fatalf("hisar %s: %v\n%s", strings.Join(args, " "), err, output)
		}
		return output
	}

	linksBefore, err := service.ListShareLinks()
	if err != nil {
		t.Fatal(err)
	}
	if output := run(false, "", "share-link", "-v", "personal", "target", "--field", "password"); !strings.Contains(output, personalID) || !strings.Contains(output, shared.ID) {
		t.Errorf("share-link in one of two vaults named personal:\n%s", output)
	}
	if output := run(false, "", "share-link", "-v", personalID, "deploy", "--field", "password"); !strings.Contains(output, firstDeploy) || !strings.Contains(output, secondDeploy) {
		t.Errorf("share-link of one of two items titled deploy:\n%s", output)
	}
	if links, err := service.ListShareLinks(); err != nil || len(links) != len(linksBefore) {
		t.Fatalf("an ambiguous share-link created a link: %d before, %d after, %v", len(linksBefore), len(links), err)
	}
	output := run(true, "", "share-link", "-v", personalID, target, "--field", "password")
	link := shareLinkURLPattern.FindString(output)
	fetched, err := sharelink.Fetch(context.Background(), nil, link)
	if err != nil {
		t.Fatalf("share-link by IDs:\n%s\n%v", output, err)
	}
	if payload, err := fetched.Open(nil); err != nil || len(payload.Fields) != 1 || payload.Fields[0].Value != "target-password" {
		t.Errorf("the item ID shared %+v, not the item it names: %v", payload, err)
	}

	key, err := service.GenerateSSHKey(personalID, "Selection key", "ed25519")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.GenerateSSHKey(personalID, key.ID, "ed25519"); err != nil {
		t.Fatal(err)
	}
	run(true, "Selection server\nselection.example.com\n\ndeploy\nserver-password\n\n"+key.ID+"\n", "add", "--type", "server", "-v", personalID)
	items, err := service.ListItems(personalID)
	if err != nil {
		t.Fatal(err)
	}
	for _, summary := range items {
		if summary.Title != "Selection server" {
			continue
		}
		if server, err := service.GetItem(personalID, summary.ID); err != nil || server.Server == nil || server.Server.SSHKeyID != key.ID {
			t.Errorf("the server links %+v, not the key with ID %s: %v", server.Server, key.ID, err)
		}
	}

	envPath := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envPath, []byte("SELECTION_TOKEN=selection-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output = runCommandLineInTerminal(t, binaryPath, socketPath, statePath, []terminalAnswer{
		{"Type numbers", ""}, {"Choose a number, vault ID or name", "personal"}, {"Choose a number, vault ID or name", shared.ID},
		{"Item name", "Selection env"}, {"Continue?", "y"},
	}, "import", envPath)
	for _, want := range []string{"Personal [" + personalID + "] (type personal, role owner)", "PERSONAL [" + shared.ID + "] (type shared, role owner)", `2 vaults are named "personal"`} {
		if !strings.Contains(output, want) {
			t.Errorf("the .env wizard does not show %q:\n%s", want, output)
		}
	}
	if sharedItems, err := service.ListItems(shared.ID); err != nil || len(sharedItems) != 1 || sharedItems[0].Title != "Selection env" {
		t.Errorf("the .env values went to %+v, not the vault chosen by ID: %v", sharedItems, err)
	}

	output = runCommandLineInTerminal(t, binaryPath, socketPath, statePath, []terminalAnswer{
		{"Choose", "1"}, {"(y/N)", "y"},
	}, "vault", "delete", shared.ID)
	for _, want := range []string{"1) Move them to Personal [" + personalID + "] (type personal, role owner)", "and delete PERSONAL?"} {
		if !strings.Contains(output, want) {
			t.Errorf("the delete wizard does not show %q:\n%s", want, output)
		}
	}
	if moved, err := service.ListItems(personalID); err != nil || !slices.ContainsFunc(moved, func(detail ItemDetail) bool { return detail.Title == "Selection env" }) {
		t.Errorf("the item did not move to Personal: %v", err)
	}
}
