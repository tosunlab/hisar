package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/server/internal/api"
	"github.com/tryhisar/hisar/server/internal/store"
)

const historyRetention = 90 * 24 * time.Hour

func printUsage() {
	fmt.Fprint(os.Stderr, `usage: hisar-server <command>

commands:
  serve    run the API server (default)
  invite   create an invite link
  migrate  up | down | status: manage database migrations

environment:
  HISAR_DATABASE_URL   Postgres connection string (required)
  HISAR_ADDR           listen address (default :8080)
  HISAR_PUBLIC_URL     public base URL (default https://tryhisar.com)
  HISAR_SECRETS_FILE   server secrets file, created on first start (default hisar-secrets.json)
  HISAR_TRUST_PROXY    trust X-Forwarded-For from the reverse proxy (default false)
  HISAR_AUTO_MIGRATE   apply pending migrations on start (default true)
  HISAR_AUTH_BURST     auth requests per IP before limiting to one per 6 seconds (default 10)
`)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	command, allArg := "serve", os.Args[1:]
	if len(allArg) > 0 && !strings.HasPrefix(allArg[0], "-") {
		command, allArg = allArg[0], allArg[1:]
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch command {
	case "serve":
		err = serve(ctx, allArg)
	case "invite":
		err = invite(ctx, allArg)
	case "migrate":
		err = migrate(ctx, allArg)
	default:
		printUsage()
		os.Exit(2)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func setup(ctx context.Context) (*store.Store, *api.Server, error) {
	databaseURL := os.Getenv("HISAR_DATABASE_URL")
	if databaseURL == "" {
		return nil, nil, errors.New("HISAR_DATABASE_URL is required")
	}
	database, err := store.Open(ctx, databaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}
	if getEnv("HISAR_AUTO_MIGRATE", "true") == "true" {
		allResult, err := database.Migrate(ctx)
		if err != nil {
			database.Close()
			return nil, nil, fmt.Errorf("migrate: %w", err)
		}
		for _, result := range allResult {
			log.Printf("applied migration %s", result.Source.Path)
		}
	}
	secrets, err := api.LoadOrCreateAllSecret(getEnv("HISAR_SECRETS_FILE", "hisar-secrets.json"))
	if err != nil {
		database.Close()
		return nil, nil, fmt.Errorf("secrets: %w", err)
	}
	authBurst, err := strconv.Atoi(getEnv("HISAR_AUTH_BURST", "10"))
	if err != nil || authBurst < 1 {
		database.Close()
		return nil, nil, fmt.Errorf("HISAR_AUTH_BURST must be a positive number")
	}
	server, err := api.CreateServer(database, secrets, api.Config{
		PublicURL:  getEnv("HISAR_PUBLIC_URL", "https://tryhisar.com"),
		TrustProxy: getEnv("HISAR_TRUST_PROXY", "false") == "true",
		AuthBurst:  authBurst,
	})
	if err != nil {
		database.Close()
		return nil, nil, err
	}
	return database, server, nil
}

func serve(ctx context.Context, allArg []string) error {
	flagSet := flag.NewFlagSet("serve", flag.ExitOnError)
	address := flagSet.String("addr", getEnv("HISAR_ADDR", ":8080"), "listen address")
	flagSet.Parse(allArg)

	database, server, err := setup(ctx)
	if err != nil {
		return err
	}
	defer database.Close()

	go pruneHistory(ctx, database)

	app := server.CreateApp()
	log.Printf("hisar-server listening on %s", *address)
	return app.Listen(*address, fiber.ListenConfig{
		DisableStartupMessage: true,
		GracefulContext:       ctx,
		ShutdownTimeout:       10 * time.Second,
	})
}

func pruneHistory(ctx context.Context, database *store.Store) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		if pruned, err := database.PruneHistory(ctx, historyRetention); err != nil {
			log.Printf("prune history: %v", err)
		} else if pruned > 0 {
			log.Printf("pruned %d history entries", pruned)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func invite(ctx context.Context, allArg []string) error {
	flagSet := flag.NewFlagSet("invite", flag.ExitOnError)
	email := flagSet.String("email", "", "email address of the invitee")
	role := flagSet.String("role", "", "admin or member")
	flagSet.Parse(allArg)

	database, server, err := setup(ctx)
	if err != nil {
		return err
	}
	defer database.Close()

	allAccount, err := database.ListAllAccount(ctx)
	if err != nil {
		return err
	}
	defaultRole := proto.RoleMember
	if len(allAccount) == 0 {
		defaultRole = proto.RoleAdmin
	}

	input := bufio.NewReader(os.Stdin)
	if *email == "" {
		*email = prompt(input, "Email", "")
	}
	if *role == "" {
		*role = prompt(input, "Role (admin/member)", defaultRole)
	}
	*email = strings.ToLower(strings.TrimSpace(*email))
	if !strings.Contains(*email, "@") {
		return fmt.Errorf("invalid email %q", *email)
	}
	if *role != proto.RoleAdmin && *role != proto.RoleMember {
		return fmt.Errorf("invalid role %q", *role)
	}

	response, err := server.CreateInvite(ctx, *email, *role, nil)
	if err != nil {
		return err
	}
	fmt.Printf("Invite for %s (%s), valid for 7 days:\n%s\n", *email, *role, response.URL)
	return nil
}

func migrate(ctx context.Context, allArg []string) error {
	if len(allArg) != 1 {
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

	switch allArg[0] {
	case "up":
		allResult, err := database.Migrate(ctx)
		if err != nil {
			return err
		}
		if len(allResult) == 0 {
			fmt.Println("no pending migrations")
		}
		for _, result := range allResult {
			fmt.Printf("applied %s (%s)\n", result.Source.Path, result.Duration)
		}
	case "down":
		result, err := database.RollbackMigration(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("rolled back %s\n", result.Source.Path)
	case "status":
		allStatus, err := database.GetAllMigrationStatus(ctx)
		if err != nil {
			return err
		}
		for _, status := range allStatus {
			appliedAt := "pending"
			if !status.AppliedAt.IsZero() {
				appliedAt = status.AppliedAt.Format(time.RFC3339)
			}
			fmt.Printf("%-24s %s\n", status.Source.Path, appliedAt)
		}
	default:
		return fmt.Errorf("unknown migrate command %q", allArg[0])
	}
	return nil
}

func prompt(input *bufio.Reader, label, defaultValue string) string {
	if defaultValue != "" {
		fmt.Printf("%s [%s]: ", label, defaultValue)
	} else {
		fmt.Printf("%s: ", label)
	}
	line, _ := input.ReadString('\n')
	if line = strings.TrimSpace(line); line == "" {
		return defaultValue
	}
	return line
}
