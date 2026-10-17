package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/tryhisar/hisar/server/internal/api"
	"github.com/tryhisar/hisar/server/internal/desktopupdate"
	"github.com/tryhisar/hisar/server/internal/service"
	"github.com/tryhisar/hisar/server/internal/settings"
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
	publicURL := getEnv("HISAR_PUBLIC_URL", "https://api.tryhisar.com")
	desktopUpdates, err := createDesktopUpdateSource(publicURL)
	if err != nil {
		database.Close()
		return nil, nil, err
	}
	server, err := api.NewServer(database, secrets, api.Config{
		Config:         service.Config{PublicURL: publicURL},
		Environment:    os.Getenv,
		TrustProxy:     getEnv("HISAR_TRUST_PROXY", "false") == "true",
		DesktopUpdates: desktopUpdates,
	})
	if err != nil {
		database.Close()
		return nil, nil, err
	}
	logSettings(server.Settings())
	return database, server, nil
}

func logSettings(effective settings.Settings) {
	if effective.SMTP.IsSet() {
		log.Printf("email through %s port %d", effective.SMTP.Host, effective.SMTP.Port)
	}
	if minimumVersions := effective.MinimumVersions; minimumVersions != (settings.MinimumVersions{}) {
		log.Printf("minimum app versions: macOS %q, iOS %q, CLI %q, extension %q", minimumVersions.DesktopMacOS, minimumVersions.IOS, minimumVersions.CLI, minimumVersions.Extension)
	}
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
