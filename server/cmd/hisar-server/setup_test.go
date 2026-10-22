package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestSetupRequiresMatchingDatabaseAndSecrets(t *testing.T) {
	databaseURL := os.Getenv("HISAR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HISAR_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)
	if _, err := connection.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HISAR_DATABASE_URL", databaseURL)
	t.Setenv("HISAR_AUTO_MIGRATE", "true")
	t.Setenv("HISAR_DESKTOP_UPDATE_GITHUB_REPOSITORY", "off")
	path := filepath.Join(t.TempDir(), "secrets.json")
	t.Setenv("HISAR_SECRETS_FILE", path)
	database, _, err := setup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	database.Close()
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `INSERT INTO accounts (id,email,name,role,argon2_params,salt,opaque_record,enc_keyset,kem_pub,sign_pub,kem_sig)
		VALUES ('SMOKE1','startup@example.test','Startup','admin','{}','x','x','x','x','x','x')`); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := setup(ctx); err == nil || !strings.Contains(err.Error(), "restore the matching database") {
		t.Fatalf("missing secrets %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("replacement keys created %v", err)
	}
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	database, _, err = setup(ctx)
	if err != nil {
		t.Fatalf("paired recovery %v", err)
	}
	database.Close()
}
