package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/pushrelay"
)

// pushRelay runs Hisar's push relay, which holds the APNs key for every Hisar server: Android phones get their pushes from the servers directly, through Web Push. It needs no database.
func pushRelay(ctx context.Context, _ []string) error {
	relay, err := newPushRelayFromEnvironment()
	if err != nil {
		return err
	}
	address := getEnv("HISAR_PUSH_RELAY_ADDR", ":8090")
	server := &http.Server{Addr: address, Handler: relay.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 2 * time.Minute}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				server.Shutdown(shutdownContext)
				cancel()
				return
			case <-ticker.C:
				relay.Maintain()
			}
		}
	}()
	log.Printf("hisar-server %s push relay listening on %s", version, address)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func newPushRelayFromEnvironment() (*pushrelay.Relay, error) {
	keyText, err := os.ReadFile(getEnv("HISAR_PUSH_RELAY_SEALING_KEY_FILE", "push-relay-sealing-key"))
	if err != nil {
		return nil, fmt.Errorf("push relay: the sealing key: %w", err)
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(keyText)))
	if err != nil {
		return nil, errors.New("push relay: the sealing key file must hold 32 bytes in base64")
	}
	sealer, err := pushrelay.NewSealer(key)
	if err != nil {
		return nil, err
	}
	path := os.Getenv("HISAR_PUSH_RELAY_APNS_KEY_FILE")
	if path == "" {
		return nil, errors.New("push relay: set HISAR_PUSH_RELAY_APNS_KEY_FILE")
	}
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("push relay: the APNs key: %w", err)
	}
	apnsKey, err := pushrelay.ParseAPNsKey(pemBytes)
	if err != nil {
		return nil, err
	}
	keyID, teamID := os.Getenv("HISAR_PUSH_RELAY_APNS_KEY_ID"), os.Getenv("HISAR_PUSH_RELAY_APNS_TEAM_ID")
	if keyID == "" || teamID == "" {
		return nil, errors.New("push relay: HISAR_PUSH_RELAY_APNS_KEY_ID and HISAR_PUSH_RELAY_APNS_TEAM_ID are needed with the APNs key")
	}
	sender := pushrelay.NewAPNsSender(pushrelay.APNsConfig{KeyID: keyID, TeamID: teamID, Topic: getEnv("HISAR_PUSH_RELAY_APNS_TOPIC", "com.tryhisar.hisar"), Key: apnsKey})
	allSender := map[apitypes.PushPlatform]pushrelay.Sender{apitypes.PushPlatformAPNs: sender, apitypes.PushPlatformAPNsSandbox: sender}
	return pushrelay.NewRelay(sealer, allSender), nil
}
