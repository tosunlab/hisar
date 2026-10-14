package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/tryhisar/hisar/server/internal/api"
	"github.com/tryhisar/hisar/server/internal/desktopupdate"
	"github.com/tryhisar/hisar/server/internal/mail"
	"github.com/tryhisar/hisar/server/internal/service"
	"github.com/tryhisar/hisar/server/internal/store"
)

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
		results, err := database.Migrate(ctx)
		if err != nil {
			database.Close()
			return nil, nil, fmt.Errorf("migrate: %w", err)
		}
		for _, result := range results {
			log.Printf("applied migration %s", result.Source.Path)
		}
	}
	secrets, err := api.LoadOrCreateSecrets(getEnv("HISAR_SECRETS_FILE", "hisar-secrets.json"))
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
	minimumVersions := readMinimumVersions()
	if err := minimumVersions.Validate(); err != nil {
		database.Close()
		return nil, nil, err
	}
	server, err := api.NewServer(database, secrets, api.Config{
		Config:          service.Config{PublicURL: publicURL, Mailer: mailer},
		TrustProxy:      getEnv("HISAR_TRUST_PROXY", "false") == "true",
		AuthBurst:       authBurst,
		DesktopUpdates:  desktopUpdates,
		MinimumVersions: minimumVersions,
	})
	if err != nil {
		database.Close()
		return nil, nil, err
	}
	return database, server, nil
}

func readMinimumVersions() api.MinimumVersions {
	minimumVersions := api.MinimumVersions{
		DesktopMacOS: strings.TrimSpace(os.Getenv("HISAR_MINIMUM_VERSION_DESKTOP_MACOS")),
		IOS:          strings.TrimSpace(os.Getenv("HISAR_MINIMUM_VERSION_IOS")),
		CLI:          strings.TrimSpace(os.Getenv("HISAR_MINIMUM_VERSION_CLI")),
		Extension:    strings.TrimSpace(os.Getenv("HISAR_MINIMUM_VERSION_EXTENSION")),
	}
	if minimumVersions != (api.MinimumVersions{}) {
		log.Printf("minimum app versions: macOS %q, iOS %q, CLI %q, extension %q", minimumVersions.DesktopMacOS, minimumVersions.IOS, minimumVersions.CLI, minimumVersions.Extension)
	}
	return minimumVersions
}

func createDesktopUpdateSource(publicURL string) (*desktopupdate.Source, error) {
	repository := strings.TrimSpace(getEnv("HISAR_DESKTOP_UPDATE_GITHUB_REPOSITORY", desktopupdate.DefaultGitHubRepository))
	if repository == "off" {
		log.Printf("desktop updates are off")
		return nil, nil
	}
	token := strings.TrimSpace(os.Getenv("HISAR_DESKTOP_UPDATE_GITHUB_TOKEN"))
	gitHub, err := desktopupdate.NewGitHub(desktopupdate.GitHubConfig{
		APIURL:     getEnv("HISAR_DESKTOP_UPDATE_GITHUB_API_URL", desktopupdate.DefaultGitHubAPIURL),
		Repository: repository,
		Token:      token,
	})
	if err != nil {
		return nil, fmt.Errorf("desktop updates: %w", err)
	}
	tokenNote := "without a token"
	if token != "" {
		tokenNote = "with a token"
	}
	log.Printf("desktop updates from the GitHub Releases of %s, %s", repository, tokenNote)
	return desktopupdate.NewSource(gitHub, desktopupdate.SourceConfig{PublicURL: publicURL}), nil
}

func createMailer() (mail.Mailer, error) {
	names := []string{"HISAR_SMTP_HOST", "HISAR_SMTP_PORT", "HISAR_SMTP_USERNAME", "HISAR_SMTP_PASSWORD", "HISAR_SMTP_FROM"}
	setCount := 0
	for _, name := range names {
		if os.Getenv(name) != "" {
			setCount++
		}
	}
	if setCount == 0 {
		return nil, nil
	}
	if setCount != len(names) {
		return nil, errors.New("email needs HISAR_SMTP_HOST, HISAR_SMTP_PORT, HISAR_SMTP_USERNAME, HISAR_SMTP_PASSWORD and HISAR_SMTP_FROM together")
	}
	port, err := strconv.Atoi(os.Getenv("HISAR_SMTP_PORT"))
	if err != nil {
		return nil, fmt.Errorf("HISAR_SMTP_PORT must be a port number")
	}
	mailer, err := mail.NewSMTPMailer(mail.SMTPConfig{
		Host: os.Getenv("HISAR_SMTP_HOST"), Port: port, Username: os.Getenv("HISAR_SMTP_USERNAME"), Password: os.Getenv("HISAR_SMTP_PASSWORD"), From: os.Getenv("HISAR_SMTP_FROM"),
	})
	if err != nil {
		return nil, err
	}
	log.Printf("email through %s port %d", os.Getenv("HISAR_SMTP_HOST"), port)
	return mailer, nil
}
