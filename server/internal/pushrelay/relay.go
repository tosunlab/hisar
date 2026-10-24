package pushrelay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/ratelimit"
)

// ErrUnregistered means the push service no longer knows the token: the app was removed or the token replaced.
var ErrUnregistered = errors.New("pushrelay: the push service no longer knows this token")

const (
	maximumBodyBytes = 8 << 10
	// pushInterval is the least time between two pushes of one kind to one phone; more in that time say nothing new.
	pushInterval = 30 * time.Second
)

// getTimeToLive is how long the push service keeps trying to deliver a push of the kind: an approval request ends
// within minutes, the others stay worth seeing for a day.
func getTimeToLive(kind apitypes.PushKind) time.Duration {
	if kind == apitypes.PushKindApproval {
		return 3 * time.Minute
	}
	return 24 * time.Hour
}

// Sender delivers a push of a kind to one push service token.
type Sender interface {
	Send(ctx context.Context, platform apitypes.PushPlatform, token string, kind apitypes.PushKind) error
}

type Relay struct {
	sealer          *Sealer
	allSender       map[apitypes.PushPlatform]Sender
	pushLimiter     *ratelimit.Limiter
	registerLimiter *ratelimit.Limiter
}

// NewRelay serves the platforms it has a sender for; a phone of another platform cannot register.
func NewRelay(sealer *Sealer, allSender map[apitypes.PushPlatform]Sender) *Relay {
	return &Relay{
		sealer:          sealer,
		allSender:       allSender,
		pushLimiter:     ratelimit.New(rate.Every(pushInterval), 1),
		registerLimiter: ratelimit.New(rate.Every(2*time.Second), 30),
	}
}

// Maintain lets the limiters forget keys that have been quiet; the command calls it every minute.
func (relay *Relay) Maintain() {
	relay.pushLimiter.Maintain()
	relay.registerLimiter.Maintain()
}

func (relay *Relay) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/register", relay.handleRegister)
	mux.HandleFunc("POST /v1/push", relay.handlePush)
	mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusNoContent) })
	return mux
}

func readJSON(request *http.Request, value any) error {
	decoder := json.NewDecoder(io.LimitReader(request.Body, maximumBodyBytes))
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}

func getClientIP(request *http.Request) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		return request.RemoteAddr
	}
	return host
}

func (relay *Relay) handleRegister(writer http.ResponseWriter, request *http.Request) {
	if !relay.registerLimiter.Allow(getClientIP(request)) {
		http.Error(writer, "too many requests", http.StatusTooManyRequests)
		return
	}
	var body apitypes.RelayRegisterRequest
	if err := readJSON(request, &body); err != nil {
		http.Error(writer, "invalid request", http.StatusBadRequest)
		return
	}
	if _, ok := relay.allSender[body.Platform]; !ok {
		http.Error(writer, "this relay does not serve that platform", http.StatusBadRequest)
		return
	}
	handle, err := relay.sealer.Seal(body.Platform, body.Token)
	if err != nil {
		http.Error(writer, "invalid token", http.StatusBadRequest)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(apitypes.RelayRegisterResponse{Handle: handle})
}

func (relay *Relay) handlePush(writer http.ResponseWriter, request *http.Request) {
	var body apitypes.RelayPushRequest
	if err := readJSON(request, &body); err != nil || !apitypes.IsValidPushKind(body.Kind) {
		http.Error(writer, "invalid request", http.StatusBadRequest)
		return
	}
	platform, token, err := relay.sealer.Open(body.Handle)
	if err != nil {
		http.Error(writer, "invalid handle", http.StatusBadRequest)
		return
	}
	sender, ok := relay.allSender[platform]
	if !ok {
		http.Error(writer, "this relay does not serve that platform", http.StatusGone)
		return
	}
	handleHash := sha256.Sum256([]byte(token))
	if !relay.pushLimiter.Allow(hex.EncodeToString(handleHash[:16]) + "/" + string(body.Kind)) {
		writer.WriteHeader(http.StatusAccepted)
		return
	}
	switch err := sender.Send(request.Context(), platform, token, body.Kind); {
	case errors.Is(err, ErrUnregistered):
		http.Error(writer, "unregistered", http.StatusGone)
	case err != nil:
		// The log names the platform and kind only: never the token or the handle.
		log.Printf("push relay: %s %s: %v", platform, body.Kind, err)
		http.Error(writer, "the push service did not take the push", http.StatusBadGateway)
	default:
		writer.WriteHeader(http.StatusNoContent)
	}
}
