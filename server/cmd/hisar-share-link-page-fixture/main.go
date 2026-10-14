// Command hisar-share-link-page-fixture runs the API server and creates share links with the Go client, for the recipient page's browser test.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/core/sharelink"
	"github.com/tryhisar/hisar/server/internal/api"
	"github.com/tryhisar/hisar/server/internal/service"
	"github.com/tryhisar/hisar/server/internal/store"
)

const (
	fixturePassword   = "k7#Vq2!mZp9@Lx4$Rt8w"
	fixturePassphrase = "correct horse battery staple"
)

type fixture struct {
	BaseURL    string            `json:"baseURL"`
	Password   string            `json:"password"`
	Passphrase string            `json:"passphrase"`
	Links      map[string]string `json:"links"`
}

func main() {
	address := flag.String("addr", "127.0.0.1:0", "loopback address to listen on")
	flag.Parse()
	databaseURL := os.Getenv("HISAR_DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("HISAR_DATABASE_URL is required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	database, err := store.Open(ctx, databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Migrate(ctx); err != nil {
		log.Fatal(err)
	}
	secretDirectory, err := os.MkdirTemp("", "hisar-share-link-page-fixture")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(secretDirectory)
	secrets, err := api.LoadOrCreateSecrets(filepath.Join(secretDirectory, "secrets.json"))
	if err != nil {
		log.Fatal(err)
	}
	listener, err := net.Listen("tcp", *address)
	if err != nil {
		log.Fatal(err)
	}
	baseURL := "http://" + listener.Addr().String()
	server, err := api.NewServer(database, secrets, api.Config{Config: service.Config{PublicURL: baseURL}, AuthBurst: 1000})
	if err != nil {
		log.Fatal(err)
	}
	app := server.CreateApp()
	go app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true, GracefulContext: ctx})

	created, err := createShareLinks(ctx, server, baseURL)
	if err != nil {
		log.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(created); err != nil {
		log.Fatal(err)
	}
	<-ctx.Done()
}

func createShareLinks(ctx context.Context, server *api.Server, baseURL string) (*fixture, error) {
	suffix := make([]byte, 4)
	rand.Read(suffix)
	email := "page-" + hex.EncodeToString(suffix) + "@example.com"
	invite, err := server.CreateInvite(ctx, email, apitypes.RoleMember, nil)
	if err != nil {
		return nil, err
	}
	owner, _, err := session.SignUp(ctx, apiclient.New(baseURL), invite.Token, email, "Page fixture", []byte("fixture master password"), apitypes.DeviceInfo{Name: "fixture", Platform: "go"})
	if err != nil {
		return nil, err
	}
	defer owner.Lock()
	vault, err := owner.FindVault(ctx, "Personal")
	if err != nil {
		return nil, err
	}
	vaultItem := &item.Item{
		Overview: item.Overview{Title: "GitHub", Username: "octocat", URLs: []string{"https://github.com/login"}},
		Details: item.Details{
			Password:        fixturePassword,
			Notes:           "Recovery codes are in the team runbook.\nRotate after offboarding.",
			Fields:          []item.Field{{Name: "Recovery code", Value: "a1b2-c3d4-e5f6", Concealed: true}},
			OneTimePassword: "otpauth://totp/GitHub:octocat?secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ",
		},
	}
	if _, err := owner.SaveItem(ctx, vault, vaultItem); err != nil {
		return nil, err
	}
	fieldKeys := sharelink.ListAvailableFieldKeys(vaultItem)
	result := &fixture{BaseURL: baseURL, Password: fixturePassword, Passphrase: fixturePassphrase, Links: map[string]string{}}
	for name, share := range map[string]func() (*session.ShareLink, error){
		"item": func() (*session.ShareLink, error) {
			return owner.ShareItem(ctx, vault, vaultItem, fieldKeys, session.ShareLinkOptions{Expiry: sharelink.ExpiryWeek, MaxViews: 3})
		},
		"passphrase": func() (*session.ShareLink, error) {
			return owner.ShareItem(ctx, vault, vaultItem, []string{sharelink.FieldUsername, sharelink.FieldPassword}, session.ShareLinkOptions{Expiry: sharelink.ExpiryDay, MaxViews: 2, Passphrase: []byte(fixturePassphrase)})
		},
		"oneView": func() (*session.ShareLink, error) {
			return owner.ShareItem(ctx, vault, vaultItem, []string{sharelink.FieldPassword}, session.ShareLinkOptions{Expiry: sharelink.ExpiryHour, MaxViews: 1})
		},
		"text": func() (*session.ShareLink, error) {
			return owner.ShareText(ctx, "Staging database URL", "postgres://deploy:s3cr3t@db.internal:5432/app", session.ShareLinkOptions{Expiry: sharelink.ExpiryMonth})
		},
		"screenshot": func() (*session.ShareLink, error) {
			return owner.ShareItem(ctx, vault, vaultItem, fieldKeys, session.ShareLinkOptions{Expiry: sharelink.ExpiryWeek, MaxViews: 5})
		},
		"screenshotPassphrase": func() (*session.ShareLink, error) {
			return owner.ShareItem(ctx, vault, vaultItem, []string{sharelink.FieldPassword}, session.ShareLinkOptions{Expiry: sharelink.ExpiryDay, MaxViews: 2, Passphrase: []byte(fixturePassphrase)})
		},
	} {
		created, err := share()
		if err != nil {
			return nil, err
		}
		result.Links[name] = created.URL
	}
	return result, nil
}
