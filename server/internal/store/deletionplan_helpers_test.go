package store

import (
	"context"
	"errors"
	"testing"
)

func reviewStoreDeletion(t *testing.T, database *Store, ctx context.Context, accountID string) string {
	t.Helper()
	preview, err := database.PreviewAccountDeletion(ctx, accountID)
	if errors.Is(err, ErrNotFound) {
		return "1:already-deleted"
	}
	if err != nil {
		t.Fatalf("review account deletion: %v", err)
	}
	if preview.PlanVersion == "" {
		t.Fatal("deletion review omitted its plan identity")
	}
	return preview.PlanVersion
}
