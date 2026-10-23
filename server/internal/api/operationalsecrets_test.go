package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/mail/mailtest"
	"github.com/tryhisar/hisar/server/internal/store"
)

func TestSecretsMissingRefusesCreationForInitializedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.json")
	if _, err := LoadOrCreateSecrets(path, false); err == nil || !strings.Contains(err.Error(), "restore the matching database") {
		t.Fatalf("missing secrets %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("replacement file exists %v", err)
	}
}

func TestInvalidPresentSecretsAreNotReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.json")
	invalid, err := json.Marshal(&Secrets{OpaqueKeyMaterial: []byte{1}, HMACKey: make([]byte, 32)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, invalid, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateSecrets(path, true); err == nil {
		t.Fatal("invalid OPAQUE secrets accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, invalid) {
		t.Fatal("invalid present secrets replaced")
	}
}

func TestConcurrentSecretCreationUsesOneCompleteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.json")
	var wg sync.WaitGroup
	results := make(chan *Secrets, 12)
	errors := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			secrets, err := LoadOrCreateSecrets(path, true)
			results <- secrets
			errors <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first []byte
	for secrets := range results {
		if first == nil {
			first = secrets.fingerprint()
		}
		if !bytes.Equal(first, secrets.fingerprint()) {
			t.Fatal("concurrent starts chose different secrets")
		}
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files remain %v %v", entries, err)
	}
}

func TestWrongPresentSecretsRefusedBeforeSettingsLoad(t *testing.T) {
	_, _, databaseURL := setupServer(t)
	database, err := store.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	for _, other := range []*Secrets{
		{OpaqueKeyMaterial: generateSecrets().OpaqueKeyMaterial, HMACKey: testServerSecrets.HMACKey},
		{OpaqueKeyMaterial: testServerSecrets.OpaqueKeyMaterial, HMACKey: generateSecrets().HMACKey},
	} {
		if _, err := NewServer(database, other, Config{}); err == nil || !strings.Contains(err.Error(), "do not match") {
			t.Fatalf("wrong-present secrets %v", err)
		}
	}
}

func TestSMTPEnvironmentDoesNotHideCorruptStoredPassword(t *testing.T) {
	_, _, databaseURL := setupServer(t)
	ctx := context.Background()
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)
	if _, err := connection.Exec(ctx, `INSERT INTO instance_settings (id,settings) VALUES (1,'{"smtp":{"password":{"version":1,"ciphertext":"AQID"}}}')`); err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	environment := map[string]string{"HISAR_SMTP_HOST": "localhost", "HISAR_SMTP_FROM": "noreply@example.com"}
	if _, err := NewServer(database, testServerSecrets, Config{Environment: func(name string) string { return environment[name] }}); err == nil || !strings.Contains(err.Error(), "cannot decrypt SMTP password") {
		t.Fatalf("corrupt SMTP hidden by environment: %v", err)
	}
}

func TestEncryptedSMTPPasswordSurvivesRestartAndDelivers(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	admin := signUpTestAccount(t, context.Background(), server, baseURL, "smtp-admin@example.com", apitypes.RoleAdmin)
	sink, err := mailtest.StartSink("127.0.0.1:0", mailtest.SinkConfig{ExpectedUsername: "a10", ExpectedPassword: " a10 smtp secret "})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sink.Close() })
	host, port, _ := net.SplitHostPort(sink.Address())
	portNumber, _ := strconv.Atoi(port)
	status, body := saveSettings(t, baseURL, admin.Client.Token, map[string]any{
		"smtp.host": host, "smtp.port": portNumber, "smtp.username": "a10", "smtp.password": " a10 smtp secret ", "smtp.from": "Hisar <noreply@example.com>",
	})
	if status != http.StatusOK || strings.Contains(body, "a10 smtp secret") {
		t.Fatalf("save %d %s", status, body)
	}
	stored := queryValue[string](t, databaseURL, `SELECT settings::text FROM instance_settings`)
	if strings.Contains(stored, "a10 smtp secret") || !strings.Contains(stored, `"ciphertext"`) {
		t.Fatalf("database disclosure %s", stored)
	}
	restarted, _ := startServerOnDatabase(t, databaseURL, nil)
	status, body = sendRawRequest(t, restarted, admin.Client.Token, "POST", settingsPath+"/test-email", apitypes.AdminTestEmailRequest{To: "recipient@example.com"})
	if status != http.StatusNoContent {
		t.Fatalf("delivery %d %s", status, body)
	}
	messages := sink.ListMessages()
	if len(messages) != 1 || messages[0].Username != "a10" || !strings.Contains(messages[0].Data, "Test email from Hisar") {
		t.Fatalf("SMTP delivery %+v", messages)
	}
	settings := readSettings(t, restarted, admin.Client.Token)
	if string(settings.Settings["smtp.password"].Value) != `{"set":true}` {
		t.Fatal("password API not redacted")
	}
	if encoded, err := json.Marshal(settings); err != nil || strings.Contains(string(encoded), "a10 smtp secret") {
		t.Fatal("settings disclose password")
	}
}
