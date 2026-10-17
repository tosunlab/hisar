package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestInstanceSettings(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	if document, err := database.GetInstanceSettings(ctx); err != nil || document != nil {
		t.Fatalf("before a save: %s %v", document, err)
	}
	if err := database.UpdateInstanceSettings(ctx, "admin-1", func(document []byte) ([]byte, error) {
		if string(document) != "{}" {
			t.Errorf("first document %s", document)
		}
		return []byte(`{"authBurst": 2}`), nil
	}); err != nil {
		t.Fatal(err)
	}
	refusal := errors.New("refused")
	if err := database.UpdateInstanceSettings(ctx, "admin-2", func([]byte) ([]byte, error) { return []byte(`{"authBurst": 3}`), refusal }); !errors.Is(err, refusal) {
		t.Fatalf("refused update: %v", err)
	}
	document, err := database.GetInstanceSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]int
	if err := json.Unmarshal(document, &stored); err != nil || stored["authBurst"] != 2 {
		t.Errorf("stored %s %v", document, err)
	}
	var updatedBy string
	if err := database.pool.QueryRow(ctx, `SELECT updated_by FROM instance_settings WHERE updated_at IS NOT NULL`).Scan(&updatedBy); err != nil || updatedBy != "admin-1" {
		t.Errorf("updated by %q %v", updatedBy, err)
	}
}
