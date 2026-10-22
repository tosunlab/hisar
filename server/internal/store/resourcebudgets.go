package store

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const HistoryByteCapacity int64 = 1 << 30

var ErrResourceBudget = errors.New("store: resource growth budget exceeded")

func IsResourceBudget(err error) bool {
	var postgresError *pgconn.PgError
	return errors.Is(err, ErrResourceBudget) || (errors.As(err, &postgresError) && postgresError.Code == "P0001" && postgresError.Message == "resource growth budget exceeded")
}

// HistoryUsage is sampled before a vault-locked mutation. The fixed charge covers row and index overhead;
// it is an accounting envelope, not a claim about exact Postgres resident memory or compressed disk bytes.
func HistoryUsage(ctx context.Context, tx pgx.Tx, vaultID string) (int64, error) {
	var used int64
	err := tx.QueryRow(ctx, `SELECT coalesce(sum(octet_length(overview)::bigint+octet_length(details)+256),0) FROM item_history WHERE vault_id=$1`, vaultID).Scan(&used)
	return used, err
}

// CheckHistoryGrowth permits legacy over-budget data to shrink or be re-encrypted without growing.
// allowDeletion is reserved for a single live-to-deleted transition, which is still charged to future baselines.
func CheckHistoryGrowth(ctx context.Context, tx pgx.Tx, vaultID string, before int64, allowDeletion bool) error {
	after, err := HistoryUsage(ctx, tx, vaultID)
	if err != nil {
		return err
	}
	if after > HistoryByteCapacity && after > before && !allowDeletion {
		return ErrResourceBudget
	}
	return nil
}
