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
	"github.com/tryhisar/hisar/server/internal/desktopupdate"
	"github.com/tryhisar/hisar/server/internal/mail"
	"github.com/tryhisar/hisar/server/internal/store"
)

const (
	historyRetention       = 90 * 24 * time.Hour
	auditRetention         = 365 * 24 * time.Hour
	expiredInvitationAge   = 30 * 24 * time.Hour
	shareLinkSweepInterval = 10 * time.Minute
	webhookEventRetention  = 30 * 24 * time.Hour
)

func printUsage() {
	fmt.Fprint(os.Stderr, `usage: hisar-server <command>

commands:
  serve    run the API server (default)
  invite   create an invite link (printed, not emailed)
  migrate  up | down | status: manage database migrations
  operator show-plan | grant-founding | revoke-founding: one account's plan (see hisar-server operator)

environment:
  HISAR_DATABASE_URL   Postgres connection string (required)
  HISAR_ADDR           listen address (default :8080)
  HISAR_PUBLIC_URL     public base URL (default https://api.tryhisar.com)
  HISAR_SECRETS_FILE   server secrets file, created on first start (default hisar-secrets.json)
  HISAR_TRUST_PROXY    trust X-Forwarded-For from the reverse proxy (default false)
  HISAR_AUTO_MIGRATE   apply pending migrations on start (default true)
  HISAR_AUTH_BURST     auth requests per IP before limiting to one per 6 seconds (default 10)

email for invites and share invitations (optional; all five or none):
  HISAR_SMTP_HOST      SMTP server, e.g. smtp.example.com
  HISAR_SMTP_PORT      465 for implicit TLS; any other port must offer STARTTLS
  HISAR_SMTP_USERNAME  SMTP user name
  HISAR_SMTP_PASSWORD  SMTP password
  HISAR_SMTP_FROM      sender, e.g. "Hisar <noreply@tryhisar.com>"

desktop updates (optional; all three or none):
  HISAR_UPDATE_GITLAB_URL         GitLab holding the hisar-desktop generic package, e.g. https://git.tryhisar.com
  HISAR_UPDATE_GITLAB_PROJECT_ID  numeric ID of that GitLab project
  HISAR_UPDATE_GITLAB_TOKEN       project access token, Reporter role, read_api scope

plans sold through Lemon Squeezy (optional; the first five together, or none):
  HISAR_LEMON_SQUEEZY_API_KEY            API key (test mode or live)
  HISAR_LEMON_SQUEEZY_STORE_ID           numeric store ID
  HISAR_LEMON_SQUEEZY_WEBHOOK_SECRET     signing secret of the webhook
  HISAR_LEMON_SQUEEZY_MONTHLY_VARIANT_ID variant ID of Pro, monthly
  HISAR_LEMON_SQUEEZY_YEARLY_VARIANT_ID  variant ID of Pro, yearly
  HISAR_LEMON_SQUEEZY_REDIRECT_URL       where the checkout sends the buyer afterwards (optional)
  HISAR_LEMON_SQUEEZY_API_URL            API base URL (default https://api.lemonsqueezy.com)
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
	case "operator":
		err = runOperatorCommand(ctx, allArg)
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
	publicURL := getEnv("HISAR_PUBLIC_URL", "https://api.tryhisar.com")
	desktopUpdates, err := createDesktopUpdateSource(publicURL)
	if err != nil {
		database.Close()
		return nil, nil, err
	}
	mailer, err := createMailer()
	if err != nil {
		database.Close()
		return nil, nil, err
	}
	lemonSqueezy, err := createLemonSqueezy()
	if err != nil {
		database.Close()
		return nil, nil, err
	}
	server, err := api.CreateServer(database, secrets, api.Config{
		PublicURL:      publicURL,
		TrustProxy:     getEnv("HISAR_TRUST_PROXY", "false") == "true",
		AuthBurst:      authBurst,
		DesktopUpdates: desktopUpdates,
		Mailer:         mailer,
		Billing:        lemonSqueezy,
	})
	if err != nil {
		database.Close()
		return nil, nil, err
	}
	return database, server, nil
}

func createDesktopUpdateSource(publicURL string) (*desktopupdate.Source, error) {
	gitLabURL, projectID, token := os.Getenv("HISAR_UPDATE_GITLAB_URL"), os.Getenv("HISAR_UPDATE_GITLAB_PROJECT_ID"), os.Getenv("HISAR_UPDATE_GITLAB_TOKEN")
	if gitLabURL == "" && projectID == "" && token == "" {
		return nil, nil
	}
	if gitLabURL == "" || projectID == "" || token == "" {
		return nil, errors.New("desktop updates need HISAR_UPDATE_GITLAB_URL, HISAR_UPDATE_GITLAB_PROJECT_ID and HISAR_UPDATE_GITLAB_TOKEN together")
	}
	gitLab, err := desktopupdate.CreateGitLab(desktopupdate.GitLabConfig{BaseURL: gitLabURL, ProjectID: projectID, Token: token})
	if err != nil {
		return nil, fmt.Errorf("desktop updates: %w", err)
	}
	log.Printf("desktop updates from %s project %s", gitLabURL, projectID)
	return desktopupdate.CreateSource(gitLab, desktopupdate.SourceConfig{PublicURL: publicURL}), nil
}

func createMailer() (mail.Mailer, error) {
	allName := []string{"HISAR_SMTP_HOST", "HISAR_SMTP_PORT", "HISAR_SMTP_USERNAME", "HISAR_SMTP_PASSWORD", "HISAR_SMTP_FROM"}
	setCount := 0
	for _, name := range allName {
		if os.Getenv(name) != "" {
			setCount++
		}
	}
	if setCount == 0 {
		return nil, nil
	}
	if setCount != len(allName) {
		return nil, errors.New("email needs HISAR_SMTP_HOST, HISAR_SMTP_PORT, HISAR_SMTP_USERNAME, HISAR_SMTP_PASSWORD and HISAR_SMTP_FROM together")
	}
	port, err := strconv.Atoi(os.Getenv("HISAR_SMTP_PORT"))
	if err != nil {
		return nil, fmt.Errorf("HISAR_SMTP_PORT must be a port number")
	}
	mailer, err := mail.CreateSMTPMailer(mail.SMTPConfig{
		Host: os.Getenv("HISAR_SMTP_HOST"), Port: port, Username: os.Getenv("HISAR_SMTP_USERNAME"), Password: os.Getenv("HISAR_SMTP_PASSWORD"), From: os.Getenv("HISAR_SMTP_FROM"),
	})
	if err != nil {
		return nil, err
	}
	log.Printf("email through %s port %d", os.Getenv("HISAR_SMTP_HOST"), port)
	return mailer, nil
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

	go pruneAllOldRecord(ctx, database)
	go deleteAllExpiredShareLink(ctx, database, shareLinkSweepInterval)

	app := server.CreateApp()
	log.Printf("hisar-server listening on %s", *address)
	return app.Listen(*address, fiber.ListenConfig{
		DisableStartupMessage: true,
		GracefulContext:       ctx,
		ShutdownTimeout:       10 * time.Second,
	})
}

func pruneAllOldRecord(ctx context.Context, database *store.Store) {
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
		if _, err := database.PruneWebhookEvent(ctx, webhookEventRetention); err != nil {
			log.Printf("prune billing webhook events: %v", err)
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

func deleteAllExpiredShareLink(ctx context.Context, database *store.Store, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if deletedCount, err := database.DeleteAllExpiredShareLink(ctx); err != nil && ctx.Err() == nil {
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
