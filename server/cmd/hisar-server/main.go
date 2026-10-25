package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/tryhisar/hisar/server/internal/desktopupdate"
)

func printUsage() {
	fmt.Fprintf(os.Stderr, `usage: hisar-server <command>

commands:
  serve    run the API server (default)
  invite   create an invite link (printed, not emailed)
  migrate  up | down | status: manage database migrations
  push-relay  run Hisar's push relay for the phone apps' notifications (no database)

environment:
  HISAR_DATABASE_URL   Postgres connection string (required)
  HISAR_ADDR           listen address (default :8080)
  HISAR_PUBLIC_URL     public base URL (default https://api.tryhisar.com)
  HISAR_SECRETS_FILE   server secrets file, created only for an empty uninitialized database (default hisar-secrets.json)
  HISAR_TRUST_PROXY    trust X-Forwarded-For from the reverse proxy (default false)
  HISAR_AUTO_MIGRATE   apply pending migrations on start (default true)

instance settings, usually edited in the admin dashboard at /admin; a value here overrides it and locks the field:
  HISAR_AUTH_BURST     auth requests per IP before limiting to one per 6 seconds (default 10)

email for invites and share invitations (optional; any of these sets all of Email and locks it in the dashboard;
dashboard passwords are encrypted using the separately stored server secrets; with a host, a sender is needed):
  HISAR_SMTP_HOST      SMTP server, e.g. smtp.example.com
  HISAR_SMTP_PORT      465 for implicit TLS; any other port must offer STARTTLS (default 587)
  HISAR_SMTP_USERNAME  SMTP user name
  HISAR_SMTP_PASSWORD  SMTP password
  HISAR_SMTP_FROM      sender, e.g. "Hisar <noreply@tryhisar.com>"

desktop updates (the Mac app's Sparkle feed) and the newest server release for the admin dashboard, read from GitHub Releases:
  HISAR_DESKTOP_UPDATE_GITHUB_REPOSITORY  owner/name with the desktop-<version> and server-<version> releases (default %s); off disables both
  HISAR_DESKTOP_UPDATE_GITHUB_TOKEN       optional read-only token (Contents: read), needed while the repository is private
  HISAR_DESKTOP_UPDATE_GITHUB_API_URL     GitHub API base URL (default https://api.github.com)

push relay (push-relay only; it needs the sealing key and the APNs key):
  HISAR_PUSH_RELAY_ADDR              listen address (default :8090)
  HISAR_PUSH_RELAY_SEALING_KEY_FILE  32 random bytes in base64, which seal phones' push tokens into handles (default push-relay-sealing-key)
  HISAR_PUSH_RELAY_APNS_KEY_FILE     the APNs Auth Key (.p8)
  HISAR_PUSH_RELAY_APNS_KEY_ID       its key ID
  HISAR_PUSH_RELAY_APNS_TEAM_ID      the Apple developer team ID
  HISAR_PUSH_RELAY_APNS_TOPIC        the iPhone app's bundle ID (default com.tryhisar.hisar)

minimum app versions (optional; empty leaves the dashboard's value, older apps get 426 Upgrade Required):
  HISAR_MINIMUM_VERSION_DESKTOP_MACOS  e.g. 0.1.6
  HISAR_MINIMUM_VERSION_IOS
  HISAR_MINIMUM_VERSION_ANDROID
  HISAR_MINIMUM_VERSION_CLI
  HISAR_MINIMUM_VERSION_EXTENSION
`, desktopupdate.DefaultGitHubRepository)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	command, args := "serve", os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command, args = args[0], args[1:]
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch command {
	case "serve":
		err = serve(ctx, args)
	case "invite":
		err = invite(ctx, args)
	case "migrate":
		err = migrate(ctx, args)
	case "push-relay":
		err = pushRelay(ctx, args)
	default:
		printUsage()
		os.Exit(2)
	}
	if err != nil {
		log.Fatal(err)
	}
}
