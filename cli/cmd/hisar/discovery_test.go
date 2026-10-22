package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

func TestA09CLIProcess(t *testing.T) {
	encoded := os.Getenv("HISAR_A09_CLI_ARGS")
	if encoded == "" {
		return
	}
	var args []string
	if err := json.Unmarshal([]byte(encoded), &args); err != nil {
		t.Fatal(err)
	}
	os.Args = append([]string{"hisar"}, args...)
	main()
	os.Exit(0)
}

func TestCLIDuplicateDiscoveryAndIDSelection(t *testing.T) {
	app := startFakeApp(t)
	app.mutex.Lock()
	app.answer = func(request cliprotocol.Request) (any, *cliprotocol.ResponseError) {
		switch request.Type {
		case cliprotocol.TypeListVault:
			return cliprotocol.VaultListResult{Vaults: []cliprotocol.Vault{{ID: "private-id", Name: "Work", Type: "personal", Role: "owner"}, {ID: "shared-id", Name: "WORK", Type: "shared", Role: "editor"}}}, nil
		case cliprotocol.TypeListItem:
			return cliprotocol.ItemListResult{Items: []cliprotocol.Item{{ID: "deploy-a", Title: "Deploy", Username: "alice", Tags: []string{"deploy"}}, {ID: "deploy-b", Title: "DEPLOY", Username: "bob", Tags: []string{"deploy"}}, {ID: "unique-id", Title: "Unique"}}}, nil
		case cliprotocol.TypeGetItem:
			return cliprotocol.ItemResult{Item: cliprotocol.Item{ID: request.ItemID, Title: request.ItemID, Password: "synthetic-never-in-metadata"}}, nil
		}
		return answerFakeVaultRequest(request)
	}
	app.mutex.Unlock()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	runCLI := func(wantSuccess bool, args ...string) string {
		t.Helper()
		encoded, _ := json.Marshal(args)
		command := exec.Command(binary, "-test.run=^TestA09CLIProcess$")
		command.Env = append(os.Environ(), "HISAR_A09_CLI_ARGS="+string(encoded))
		var stderr bytes.Buffer
		command.Stderr = &stderr
		output, err := command.Output()
		if !wantSuccess {
			output = append(output, stderr.Bytes()...)
		}
		if (err == nil) != wantSuccess {
			t.Fatalf("hisar %v: %v\n%s", args, err, output)
		}
		if strings.Contains(string(output), "synthetic-never-in-metadata") {
			t.Fatal("metadata operation revealed a password")
		}
		t.Logf("hisar %s\n%s", strings.Join(args, " "), output)
		return string(output)
	}
	listing := runCLI(true, "vaults")
	if !strings.Contains(listing, "private-id") || !strings.Contains(listing, "shared-id") {
		t.Fatal("vault IDs undiscoverable")
	}
	ambiguous := runCLI(false, "ls", "-v", "work")
	for _, want := range []string{"private-id", "shared-id", "personal", "shared", "use an ID"} {
		if !strings.Contains(ambiguous, want) {
			t.Fatalf("candidate metadata lacks %q: %s", want, ambiguous)
		}
	}
	metadata := runCLI(true, "ls", "-v", "private-id", "--tag", "deploy")
	if !strings.Contains(metadata, "deploy-a") || !strings.Contains(metadata, "deploy-b") || strings.Contains(metadata, "unique-id") {
		t.Fatal("metadata filter did not discover intended IDs")
	}
	refusal := runCLI(false, "get", "-v", "private-id", "deploy", "-f", "title")
	if !strings.Contains(refusal, "deploy-a") || !strings.Contains(refusal, "deploy-b") || !strings.Contains(refusal, "private-id") {
		t.Fatal("duplicate item selection lacked candidates and vault metadata")
	}
	if gets := app.findRequest(cliprotocol.TypeGetItem); len(gets) != 0 {
		t.Fatalf("ambiguous selection fetched a secret: %+v", gets)
	}
	if selected := runCLI(true, "get", "-v", "private-id", "deploy-b", "-f", "title"); strings.TrimSpace(selected) != "deploy-b" {
		t.Fatalf("ID selected wrong object: %q", selected)
	}
	runCLI(true, "get", "-v", "private-id", "unique", "-f", "title")
}
