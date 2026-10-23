// Package fixturedatabase opens the database of a browser-test fixture with the server secrets it is enrolled with.
// Each database keeps one secrets file in the temporary directory, named after a hash of its URL, so a fixture restarted
// on the same database serves its earlier accounts and settings.
package fixturedatabase

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"

	"github.com/tryhisar/hisar/server/internal/api"
	"github.com/tryhisar/hisar/server/internal/store"
)

// Open migrates the database at databaseURL and loads its secrets file. With fresh, or when the file is missing (a
// cleared temporary directory), it first drops the public schema and starts over with new secrets: a database enrolled
// with secrets nobody has refuses to start.
func Open(ctx context.Context, databaseURL string, fresh bool) (*store.Store, *api.Secrets, error) {
	sum := sha256.Sum256([]byte(databaseURL))
	secretsPath := filepath.Join(os.TempDir(), fmt.Sprintf("hisar-fixture-secrets-%x.json", sum[:16]))
	if fresh {
		if err := os.Remove(secretsPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, nil, err
		}
	}
	if _, err := os.Stat(secretsPath); errors.Is(err, fs.ErrNotExist) {
		if err := emptyDatabase(ctx, databaseURL); err != nil {
			return nil, nil, err
		}
	} else if err != nil {
		return nil, nil, err
	}
	database, err := store.Open(ctx, databaseURL)
	if err != nil {
		return nil, nil, err
	}
	if _, err := database.Migrate(ctx); err != nil {
		database.Close()
		return nil, nil, err
	}
	secrets, err := api.LoadOrCreateSecrets(secretsPath, true)
	if err != nil {
		database.Close()
		return nil, nil, err
	}
	return database, secrets, nil
}

func emptyDatabase(ctx context.Context, databaseURL string) error {
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer connection.Close(ctx)
	_, err = connection.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
	return err
}
