package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/tryhisar/hisar/server/internal/store"
)

func migrate(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: hisar-server migrate up|down|status")
	}
	databaseURL := os.Getenv("HISAR_DATABASE_URL")
	if databaseURL == "" {
		return errors.New("HISAR_DATABASE_URL is required")
	}
	database, err := store.Open(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	switch args[0] {
	case "up":
		results, err := database.Migrate(ctx)
		if err != nil {
			return err
		}
		if len(results) == 0 {
			fmt.Println("no pending migrations")
		}
		for _, result := range results {
			fmt.Printf("applied %s (%s)\n", result.Source.Path, result.Duration)
		}
	case "down":
		result, err := database.RollbackMigration(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("rolled back %s\n", result.Source.Path)
	case "status":
		statuses, err := database.GetMigrationStatuses(ctx)
		if err != nil {
			return err
		}
		for _, status := range statuses {
			appliedAt := "pending"
			if !status.AppliedAt.IsZero() {
				appliedAt = status.AppliedAt.Format(time.RFC3339)
			}
			fmt.Printf("%-24s %s\n", status.Source.Path, appliedAt)
		}
	default:
		return fmt.Errorf("unknown migrate command %q", args[0])
	}
	return nil
}
