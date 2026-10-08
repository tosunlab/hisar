package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/server/internal/api"
	"github.com/tryhisar/hisar/server/internal/store"
)

const historyRetention = 90 * 24 * time.Hour

func usage() {
	fmt.Fprint(os.Stderr, `usage: hisar-server <command>

commands:
  serve    run the API server (default)
  invite   create an invite link

environment:
  HISAR_DATABASE_URL   Postgres connection string (required)
  HISAR_ADDR           listen address (default :8080)
  HISAR_PUBLIC_URL     public base URL (default https://tryhisar.com)
  HISAR_SECRETS_FILE   server secrets file, created on first start (default hisar-secrets.json)
  HISAR_TRUST_PROXY    trust X-Forwarded-For from the reverse proxy (default false)
`)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	cmd, args := "serve", os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch cmd {
	case "serve":
		err = serve(ctx, args)
	case "invite":
		err = invite(ctx, args)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func setup(ctx context.Context) (*store.Store, *api.Server, error) {
	dbURL := os.Getenv("HISAR_DATABASE_URL")
	if dbURL == "" {
		return nil, nil, errors.New("HISAR_DATABASE_URL is required")
	}
	st, err := store.Open(ctx, dbURL)
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}
	if err := st.Migrate(ctx); err != nil {
		st.Close()
		return nil, nil, fmt.Errorf("migrate: %w", err)
	}
	sec, err := api.LoadOrCreateSecrets(env("HISAR_SECRETS_FILE", "hisar-secrets.json"))
	if err != nil {
		st.Close()
		return nil, nil, fmt.Errorf("secrets: %w", err)
	}
	srv, err := api.New(st, sec, api.Config{
		PublicURL:  env("HISAR_PUBLIC_URL", "https://tryhisar.com"),
		TrustProxy: env("HISAR_TRUST_PROXY", "false") == "true",
	})
	if err != nil {
		st.Close()
		return nil, nil, err
	}
	return st, srv, nil
}

func serve(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", env("HISAR_ADDR", ":8080"), "listen address")
	fs.Parse(args)

	st, srv, err := setup(ctx)
	if err != nil {
		return err
	}
	defer st.Close()

	go pruneHistory(ctx, st)

	hs := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		hs.Shutdown(shutdown)
	}()

	log.Printf("hisar-server listening on %s", *addr)
	if err := hs.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func pruneHistory(ctx context.Context, st *store.Store) {
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		if n, err := st.PruneHistory(ctx, historyRetention); err != nil {
			log.Printf("prune history: %v", err)
		} else if n > 0 {
			log.Printf("pruned %d history entries", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func invite(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("invite", flag.ExitOnError)
	email := fs.String("email", "", "email address of the invitee")
	role := fs.String("role", "", "admin or member")
	fs.Parse(args)

	st, srv, err := setup(ctx)
	if err != nil {
		return err
	}
	defer st.Close()

	accts, err := st.ListAccounts(ctx)
	if err != nil {
		return err
	}
	defaultRole := proto.RoleMember
	if len(accts) == 0 {
		defaultRole = proto.RoleAdmin
	}

	in := bufio.NewReader(os.Stdin)
	if *email == "" {
		*email = prompt(in, "Email", "")
	}
	if *role == "" {
		*role = prompt(in, "Role (admin/member)", defaultRole)
	}
	*email = strings.ToLower(strings.TrimSpace(*email))
	if !strings.Contains(*email, "@") {
		return fmt.Errorf("invalid email %q", *email)
	}
	if *role != proto.RoleAdmin && *role != proto.RoleMember {
		return fmt.Errorf("invalid role %q", *role)
	}

	resp, err := srv.CreateInvite(ctx, *email, *role, nil)
	if err != nil {
		return err
	}
	fmt.Printf("Invite for %s (%s), valid for 7 days:\n%s\n", *email, *role, resp.URL)
	return nil
}

func prompt(in *bufio.Reader, label, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", label, def)
	} else {
		fmt.Printf("%s: ", label)
	}
	line, _ := in.ReadString('\n')
	if line = strings.TrimSpace(line); line == "" {
		return def
	}
	return line
}
