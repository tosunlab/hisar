package main

import (
	"context"
	"log"
	"time"

	"github.com/tryhisar/hisar/server/internal/store"
)

func cleanVaultTransitions(ctx context.Context, database *store.Store) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		if err := database.CleanupVaultTransitions(ctx); err != nil && ctx.Err() == nil {
			log.Printf("clean vault transitions: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
