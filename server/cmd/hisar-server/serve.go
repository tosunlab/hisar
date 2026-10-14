package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/tryhisar/hisar/server/internal/store"
)

const (
	historyRetention       = 90 * 24 * time.Hour
	auditRetention         = 365 * 24 * time.Hour
	expiredInvitationAge   = 30 * 24 * time.Hour
	shareLinkSweepInterval = 10 * time.Minute
)

func serve(ctx context.Context, args []string) error {
	flagSet := flag.NewFlagSet("serve", flag.ExitOnError)
	address := flagSet.String("addr", getEnv("HISAR_ADDR", ":8080"), "listen address")
	flagSet.Parse(args)

	database, server, err := setup(ctx)
	if err != nil {
		return err
	}
	defer database.Close()

	go pruneOldRecords(ctx, database)
	go deleteExpiredShareLinks(ctx, database, shareLinkSweepInterval)

	app := server.CreateApp()
	log.Printf("hisar-server listening on %s", *address)
	return app.Listen(*address, fiber.ListenConfig{
		DisableStartupMessage: true,
		GracefulContext:       ctx,
		ShutdownTimeout:       10 * time.Second,
	})
}

func pruneOldRecords(ctx context.Context, database *store.Store) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		if pruned, err := database.PruneHistory(ctx, historyRetention); err != nil {
			log.Printf("prune history: %v", err)
		} else if pruned > 0 {
			log.Printf("pruned %d history entries", pruned)
		}
		if pruned, err := database.PruneAudit(ctx, auditRetention); err != nil {
			log.Printf("prune audit log: %v", err)
		} else if pruned > 0 {
			log.Printf("pruned %d audit events", pruned)
		}
		if pruned, err := database.PruneExpiredInvitation(ctx, expiredInvitationAge); err != nil {
			log.Printf("prune expired invitations: %v", err)
		} else if pruned > 0 {
			log.Printf("pruned %d expired invitations", pruned)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func deleteExpiredShareLinks(ctx context.Context, database *store.Store, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if deletedCount, err := database.DeleteExpiredShareLinks(ctx); err != nil && ctx.Err() == nil {
			log.Printf("delete expired share links: %v", err)
		} else if deletedCount > 0 {
			log.Printf("deleted %d expired share links", deletedCount)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
