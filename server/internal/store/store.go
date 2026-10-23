package store

import (
	"context"
	"embed"
	"errors"
	"io/fs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

var (
	ErrNotFound = errors.New("store: not found")
	ErrConflict = errors.New("store: conflict")
	ErrExists   = errors.New("store: already exists")
)

type Store struct {
	pool *pgxpool.Pool
	// liveItemCount is GetInstanceCounts' last count of live items.
	liveItemCount *itemCountCache
	settingsKey   []byte
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool, liveItemCount: newItemCountCache()}, nil
}

func (s *Store) Close() { s.pool.Close() }

// createMigrator takes a Postgres advisory lock when takesLock, so concurrent server starts migrate only once; reading
// the status takes none, so the admin dashboard never waits behind a migration. Closing the migrator leaves the pool open.
func (s *Store) createMigrator(takesLock bool) (*goose.Provider, error) {
	migrationDirectory, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}
	var options []goose.ProviderOption
	if takesLock {
		locker, err := lock.NewPostgresSessionLocker()
		if err != nil {
			return nil, err
		}
		options = append(options, goose.WithSessionLocker(locker))
	}
	return goose.NewProvider(goose.DialectPostgres, stdlib.OpenDBFromPool(s.pool), migrationDirectory, options...)
}

func (s *Store) Migrate(ctx context.Context) ([]*goose.MigrationResult, error) {
	migrator, err := s.createMigrator(true)
	if err != nil {
		return nil, err
	}
	defer migrator.Close()
	return migrator.Up(ctx)
}

func (s *Store) RollbackMigration(ctx context.Context) (*goose.MigrationResult, error) {
	migrator, err := s.createMigrator(true)
	if err != nil {
		return nil, err
	}
	defer migrator.Close()
	return migrator.Down(ctx)
}

func (s *Store) GetMigrationStatuses(ctx context.Context) ([]*goose.MigrationStatus, error) {
	migrator, err := s.createMigrator(false)
	if err != nil {
		return nil, err
	}
	defer migrator.Close()
	return migrator.Status(ctx)
}

func (s *Store) runInTransaction(ctx context.Context, run func(pgx.Tx) error) error {
	transaction, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer transaction.Rollback(ctx)
	if err := run(transaction); err != nil {
		return err
	}
	return transaction.Commit(ctx)
}

// queryer is the pool or a transaction.
type queryer interface {
	Query(ctx context.Context, sql string, arguments ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, arguments ...any) pgx.Row
}

func isUniqueViolation(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505"
}

func convertNoRowsToNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
