package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cache"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/export"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/session"
)

func r08CaptureCLI(t *testing.T, input string, args ...string) (string, string, error) {
	t.Helper()
	stdout, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	stderr, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer stderr.Close()
	oldOut, oldErr, oldIn := os.Stdout, os.Stderr, stdinReader
	os.Stdout, os.Stderr, stdinReader = stdout, stderr, bufio.NewReader(strings.NewReader(input))
	defer func() { os.Stdout, os.Stderr, stdinReader = oldOut, oldErr, oldIn }()
	selected, ok := findCommand(args[0])
	if !ok {
		t.Fatalf("unknown command %q", args[0])
	}
	err = runCommand(context.Background(), selected, false, flag.NewFlagSet(args[0], flag.ContinueOnError), args[1:])
	out, _ := os.ReadFile(stdout.Name())
	diagnostic, _ := os.ReadFile(stderr.Name())
	return string(out), string(diagnostic), err
}

func TestR08RestartOfflineCLIArtifactRoundTrip(t *testing.T) {
	ctx := context.Background()
	accountID := crypt.GenerateAccountID()
	secret, err := crypt.GenerateSecretKey(accountID)
	if err != nil {
		t.Fatal(err)
	}
	keyset, err := crypt.GenerateKeyset()
	if err != nil {
		t.Fatal(err)
	}
	defer keyset.Wipe()
	password := []byte("synthetic-review-password")
	salt := crypt.GenerateSalt()
	derived, err := crypt.DeriveKeys(password, salt, crypt.MinArgon2Params(), secret)
	if err != nil {
		t.Fatal(err)
	}
	defer derived.Wipe()
	sealedKeyset, err := keyset.Encrypt(derived.AccountUnlockKey, accountID)
	if err != nil {
		t.Fatal(err)
	}
	account := apitypes.Account{AccountID: accountID, Email: "review@example.test", Salt: salt, Argon2: crypt.MinArgon2Params(), EncKeyset: sealedKeyset, PublicKeys: keyset.GetPublicKeys(accountID)}
	vaultID, itemID, revision := "rescue-vault", "rescue-item", "rescue-revision"
	vaultKey := crypt.GenerateKey()
	defer clear(vaultKey)
	wrapped, err := crypt.WrapVaultKey(keyset.GetKEMPublicKey(), vaultID, accountID, vaultKey)
	if err != nil {
		t.Fatal(err)
	}
	encName, err := crypt.SealVaultName(vaultKey, vaultID, []byte("Personal"))
	if err != nil {
		t.Fatal(err)
	}
	vaultRecord := apitypes.Vault{ID: vaultID, Type: apitypes.VaultPersonal, Role: apitypes.VaultOwner, EncName: encName, WrappedKey: wrapped, KeyGeneration: 1}
	plainOverview, _ := json.Marshal(item.Overview{Title: "Raw secret", Username: "review"})
	const rawSecret = "synthetic-raw-\x1b[31m-password"
	plainDetails, _ := json.Marshal(item.Details{Password: rawSecret})
	overview, err := crypt.SealItemPart(vaultKey, vaultID, itemID, revision, crypt.Overview, plainOverview)
	if err != nil {
		t.Fatal(err)
	}
	details, err := crypt.SealItemPart(vaultKey, vaultID, itemID, revision, crypt.Details, plainDetails)
	if err != nil {
		t.Fatal(err)
	}
	records := []apitypes.Item{{ID: itemID, Revision: revision, Seq: 1, Overview: overview, Details: details}, {ID: "bad", Revision: "bad-revision", Seq: 2, Overview: []byte("bad"), Details: []byte("bad")}}
	var offline atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if offline.Load() {
			w.WriteHeader(502)
			return
		}
		switch r.URL.Path {
		case "/api/v1/account":
			json.NewEncoder(w).Encode(account)
		case "/api/v1/vaults":
			json.NewEncoder(w).Encode([]apitypes.Vault{vaultRecord})
		case "/api/v1/vaults/" + vaultID + "/changes":
			json.NewEncoder(w).Encode(apitypes.ChangesResponse{Items: records, NextSeq: 2, KeyGeneration: 1})
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "cache.sqlite")
	c, err := cache.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	client := apiclient.New(server.URL)
	client.Token = "synthetic-token"
	unlocked, err := session.Unlock(ctx, client, c, password, secret)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := unlocked.ReadVaults(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := unlocked.ReadItems(ctx, &inventory.Vaults[0])
	if err != nil || len(result.Items) != 1 || len(result.Issues) != 1 {
		t.Fatalf("HTTP acquisition failed: %+v %v", result, err)
	}
	unlocked.Lock()
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	offline.Store(true)
	c, err = cache.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	unlocked, err = session.Unlock(ctx, client, c, password, secret)
	if err != nil {
		t.Fatal(err)
	}
	defer unlocked.Lock()
	access := &standaloneBackend{currentState: &localstate.State{Email: account.Email, Server: server.URL}, unlockedSession: unlocked}
	socket := startFakeApp(t)
	socket.answer = func(request cliprotocol.Request) (any, *cliprotocol.ResponseError) {
		if request.Type != cliprotocol.TypeExport {
			return answerFakeVaultRequest(request)
		}
		result, err := access.writeExport(ctx, *request.Export)
		if err != nil {
			return nil, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, err.Error())
		}
		return result, nil
	}
	for _, format := range []string{"plain", "encrypted"} {
		artifactPath := filepath.Join(t.TempDir(), "recovery-"+format)
		input := ""
		if format == "encrypted" {
			input = "synthetic-export-passphrase\n"
		}
		stdout, stderr, err := r08CaptureCLI(t, input, "export", "--format", format, "-o", artifactPath, "-y")
		if err != nil || stdout != "" || !strings.Contains(stderr, "Cached copy.") || !strings.Contains(stderr, "bad-revision") {
			t.Fatalf("CLI recovery warnings or stdout wrong: out=%q err=%v stderr=%s", stdout, err, stderr)
		}
		content, err := os.ReadFile(artifactPath)
		if err != nil {
			t.Fatal(err)
		}
		passphrase := []byte(nil)
		if format == "encrypted" {
			passphrase = []byte("synthetic-export-passphrase")
		}
		doc, err := export.Decode(content, passphrase)
		if err != nil || doc.Version != 1 || doc.CountItem() != 1 || doc.Vaults[0].Items[0].Details.Password != rawSecret || doc.Provenance == nil || !doc.Provenance.IsCachedCopy() || !doc.Provenance.IsPartial() {
			t.Fatalf("actual artifact lost data or status: %+v %v", doc, err)
		}
		restoreWrites := 0
		socket.mutex.Lock()
		socket.answer = func(request cliprotocol.Request) (any, *cliprotocol.ResponseError) {
			if request.Type == cliprotocol.TypeImportItem {
				before, err := os.ReadFile(os.Stderr.Name())
				if err != nil || !strings.Contains(string(before), "Cached copy.") || !strings.Contains(string(before), "Keep the source file.") {
					return nil, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, fmt.Sprintf("restore wrote before warning: %s %v", before, err))
				}
				restoreWrites++
			}
			return answerVaultDataRequest(request)
		}
		socket.mutex.Unlock()
		restoreInput := ""
		if format == "encrypted" {
			restoreInput = "synthetic-export-passphrase\n"
		}
		_, stderr, err = r08CaptureCLI(t, restoreInput, "import", artifactPath, "-y", "--include-duplicates")
		if err != nil || restoreWrites != 1 {
			t.Fatalf("restore failed: writes=%d err=%v stderr=%s", restoreWrites, err, stderr)
		}
		if _, err := os.Stat(artifactPath); err != nil || strings.Contains(stderr, "Delete it and any other copies") {
			t.Fatalf("restore advised deletion of incomplete source: %v %s", err, stderr)
		}
		socket.mutex.Lock()
		socket.answer = func(request cliprotocol.Request) (any, *cliprotocol.ResponseError) {
			if request.Type == cliprotocol.TypeExport {
				result, err := access.writeExport(ctx, *request.Export)
				if err != nil {
					return nil, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, err.Error())
				}
				return result, nil
			}
			return answerFakeVaultRequest(request)
		}
		socket.mutex.Unlock()
	}
}

func TestR08CLIRawValueAndOldAppRefusal(t *testing.T) {
	app := startFakeApp(t)
	stdout, _, err := r08CaptureCLI(t, "", "get", "github-id", "-f", "password")
	if err != nil || stdout != "github-password\n" {
		t.Fatalf("raw output changed: %q %v", stdout, err)
	}
	app.mutex.Lock()
	app.hello.Features = nil
	app.mutex.Unlock()
	path := filepath.Join(t.TempDir(), "must-not-exist.json")
	_, stderr, err := r08CaptureCLI(t, "", "export", "--format", "plain", "-y", "-o", path)
	if err == nil || !strings.Contains(err.Error(), "update the Hisar app") {
		t.Fatalf("old app silently downgraded: %v %s", err, stderr)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("old app refusal created artifact: %v", err)
	}
}
