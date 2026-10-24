package api

import (
	"context"
	"testing"

	"github.com/tryhisar/hisar/core/session"
)

func reviewSessionDeletion(t *testing.T, current *session.Session, ctx context.Context) string {
	t.Helper()
	preview, err := current.PreviewAccountDeletion(ctx)
	if err != nil {
		t.Fatalf("review account deletion: %v", err)
	}
	if preview.PlanVersion == "" {
		t.Fatal("deletion review omitted its plan identity")
	}
	return preview.PlanVersion
}
