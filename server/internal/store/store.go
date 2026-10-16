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
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

// createMigrator uses a Postgres advisory lock so concurrent server starts migrate only once.
func (s *Store) createMigrator() (*goose.Provider, error) {
	migrationDirectory, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, err
	}
	return goose.NewProvider(goose.DialectPostgres, stdlib.OpenDBFromPool(s.pool), migrationDirectory, goose.WithSessionLocker(locker))
}

func (s *Store) Migrate(ctx context.Context) ([]*goose.MigrationResult, error) {
	migrator, err := s.createMigrator()
	if err != nil {
		return nil, err
	}
	return migrator.Up(ctx)
}

func (s *Store) RollbackMigration(ctx context.Context) (*goose.MigrationResult, error) {
	migrator, err := s.createMigrator()
	if err != nil {
		return nil, err
	}
	return migrator.Down(ctx)
}

func (s *Store) GetMigrationStatuses(ctx context.Context) ([]*goose.MigrationStatus, error) {
	migrator, err := s.createMigrator()
	if err != nil {
		return nil, err
	}
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
