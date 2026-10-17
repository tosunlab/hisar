// Command hisar-admin-page-fixture runs the API server with the built admin dashboard and an SMTP sink, and signs up one
// admin and two members with the Go client, for the dashboard's browser tests. It prints one JSON line with the server's
// URL, the sink's address and the accounts' credentials and session tokens, then one JSON line per email the sink
// receives. HISAR_* variables override the instance settings as they do for hisar-server.
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
	"strings"
	"sync"
	"syscall"

	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/server/internal/api"
	"github.com/tryhisar/hisar/server/internal/mail/mailtest"
	"github.com/tryhisar/hisar/server/internal/service"
	"github.com/tryhisar/hisar/server/internal/settings"
	"github.com/tryhisar/hisar/server/internal/store"
)

type account struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	SecretKey string `json:"secretKey"`
	Token     string `json:"token"`
}

type fixture struct {
	BaseURL  string `json:"baseURL"`
	SMTPSink string `json:"smtpSink"`
	account
	Members []account `json:"members"`
}

type receivedEmail struct {
	Recipients []string `json:"recipients"`
	Data       string   `json:"data"`
}

func main() {
	address := flag.String("addr", "127.0.0.1:0", "address to listen on")
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
	secretDirectory, err := os.MkdirTemp("", "hisar-admin-page-fixture")
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
	defaultSettings := settings.Defaults()
	defaultSettings.AuthBurst = 1000
	server, err := api.NewServer(database, secrets, api.Config{Config: service.Config{PublicURL: baseURL}, DefaultSettings: &defaultSettings, Environment: os.Getenv})
	if err != nil {
		log.Fatal(err)
	}
	var outputMutex sync.Mutex
	output := json.NewEncoder(os.Stdout)
	sink, err := mailtest.StartSink("127.0.0.1:0", mailtest.SinkConfig{OnMessage: func(received mailtest.ReceivedMessage) {
		outputMutex.Lock()
		defer outputMutex.Unlock()
		output.Encode(map[string]receivedEmail{"receivedEmail": {Recipients: received.Recipients, Data: received.Data}})
	}})
	if err != nil {
		log.Fatal(err)
	}
	defer sink.Close()
	app := server.CreateApp()
	go app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true, GracefulContext: ctx})

	created := fixture{BaseURL: baseURL, SMTPSink: sink.Address()}
	suffix := make([]byte, 4)
	rand.Read(suffix)
	for _, seed := range []struct{ name, role string }{{"Ada Admin", apitypes.RoleAdmin}, {"Mira Member", apitypes.RoleMember}, {"Milo Member", apitypes.RoleMember}} {
		signedUp, err := signUp(ctx, server, baseURL, seed.name, seed.role, hex.EncodeToString(suffix))
		if err != nil {
			log.Fatal(err)
		}
		if seed.role == apitypes.RoleAdmin {
			created.account = signedUp
		} else {
			created.Members = append(created.Members, signedUp)
		}
	}
	outputMutex.Lock()
	err = output.Encode(created)
	outputMutex.Unlock()
	if err != nil {
		log.Fatal(err)
	}
	<-ctx.Done()
}

func signUp(ctx context.Context, server *api.Server, baseURL, name, role, suffix string) (account, error) {
	firstName := strings.ToLower(strings.Fields(name)[0])
	created := account{Name: name, Email: firstName + "-" + suffix + "@example.com", Password: "fixture master password for " + firstName}
	invite, err := server.CreateInvite(ctx, created.Email, role, nil)
	if err != nil {
		return account{}, err
	}
	signedUp, secretKey, err := session.SignUp(ctx, apiclient.New(baseURL), invite.Token, created.Email, name, []byte(created.Password), apitypes.DeviceInfo{Name: "fixture", Platform: "go"})
	if err != nil {
		return account{}, err
	}
	signedUp.Lock()
	created.SecretKey, created.Token = secretKey.Encode(), signedUp.Client.Token
	return created, nil
}
