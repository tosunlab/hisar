// Package fakelemonsqueezy is a local stand-in for the Lemon Squeezy API in tests: checkouts, subscriptions and signed webhooks.
package fakelemonsqueezy

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/tryhisar/hisar/server/internal/billing"
)

const (
	APIKey           = "test-api-key"
	StoreID          = "4242"
	WebhookSecret    = "test-signing-secret"
	MonthlyVariantID = "111"
	YearlyVariantID  = "222"
)

// Checkout is what a create checkout request asked for.
type Checkout struct {
	StoreID           string
	VariantID         string
	Email             string
	AccountReference  string
	AllEnabledVariant []int64
	RedirectURL       string
}

type Server struct {
	URL string

	mutex         sync.Mutex
	httpServer    *httptest.Server
	allCheckout   []Checkout
	isFailing     bool
	portalBaseURL string
}

func Start() *Server {
	fake := &Server{portalBaseURL: "https://hisar-test.lemonsqueezy.com/billing"}
	fake.httpServer = httptest.NewServer(http.HandlerFunc(fake.handle))
	fake.URL = fake.httpServer.URL
	return fake
}

func (fake *Server) Close() { fake.httpServer.Close() }

func (fake *Server) CreateConfig() billing.Config {
	return billing.Config{
		APIURL: fake.URL, APIKey: APIKey, StoreID: StoreID, WebhookSecret: WebhookSecret,
		MonthlyVariantID: MonthlyVariantID, YearlyVariantID: YearlyVariantID,
	}
}

// SetFailing makes every API request answer 500.
func (fake *Server) SetFailing(isFailing bool) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.isFailing = isFailing
}

func (fake *Server) ListAllCheckout() []Checkout {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return append([]Checkout{}, fake.allCheckout...)
}

func (fake *Server) handle(writer http.ResponseWriter, request *http.Request) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	if request.Header.Get("Authorization") != "Bearer "+APIKey || request.Header.Get("Accept") != "application/vnd.api+json" {
		http.Error(writer, `{"errors":[{"status":"401"}]}`, http.StatusUnauthorized)
		return
	}
	if fake.isFailing {
		http.Error(writer, `{"errors":[{"status":"500"}]}`, http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/vnd.api+json")
	switch {
	case request.Method == http.MethodPost && request.URL.Path == "/v1/checkouts":
		fake.createCheckout(writer, request)
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/v1/subscriptions/"):
		subscriptionID := strings.TrimPrefix(request.URL.Path, "/v1/subscriptions/")
		json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
			"type": "subscriptions", "id": subscriptionID,
			"attributes": map[string]any{"urls": map[string]string{"customer_portal": fake.portalBaseURL + "?subscription=" + subscriptionID}},
		}})
	default:
		http.NotFound(writer, request)
	}
}

func (fake *Server) createCheckout(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Data struct {
			Type       string `json:"type"`
			Attributes struct {
				CheckoutData struct {
					Email  string            `json:"email"`
					Custom map[string]string `json:"custom"`
				} `json:"checkout_data"`
				ProductOptions struct {
					EnabledVariants []int64 `json:"enabled_variants"`
					RedirectURL     string  `json:"redirect_url"`
				} `json:"product_options"`
			} `json:"attributes"`
			Relationships struct {
				Store   struct{ Data struct{ ID string } } `json:"store"`
				Variant struct{ Data struct{ ID string } } `json:"variant"`
			} `json:"relationships"`
		} `json:"data"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body.Data.Type != "checkouts" {
		http.Error(writer, `{"errors":[{"status":"422"}]}`, http.StatusUnprocessableEntity)
		return
	}
	checkout := Checkout{
		StoreID: body.Data.Relationships.Store.Data.ID, VariantID: body.Data.Relationships.Variant.Data.ID,
		Email: body.Data.Attributes.CheckoutData.Email, AccountReference: body.Data.Attributes.CheckoutData.Custom[billing.AccountReferenceField],
		AllEnabledVariant: body.Data.Attributes.ProductOptions.EnabledVariants, RedirectURL: body.Data.Attributes.ProductOptions.RedirectURL,
	}
	fake.allCheckout = append(fake.allCheckout, checkout)
	checkoutURL := fmt.Sprintf("https://hisar-test.lemonsqueezy.com/checkout/custom/%d", len(fake.allCheckout))
	writer.WriteHeader(http.StatusCreated)
	json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
		"type": "checkouts", "id": fmt.Sprint(len(fake.allCheckout)), "attributes": map[string]any{"url": checkoutURL},
	}})
}

// SubscriptionEvent describes a subscription webhook; zero times are left out as null.
type SubscriptionEvent struct {
	EventName        string
	AccountReference string
	SubscriptionID   string
	StoreID          string
	CustomerID       string
	VariantID        string
	Status           string
	RenewsAt         time.Time
	EndsAt           time.Time
	TrialEndsAt      time.Time
	UpdatedAt        time.Time
}

func formatOptionalTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

// BuildSubscriptionWebhook returns the body Lemon Squeezy sends for a subscription event.
func BuildSubscriptionWebhook(event SubscriptionEvent) []byte {
	if event.StoreID == "" {
		event.StoreID = StoreID
	}
	if event.CustomerID == "" {
		event.CustomerID = "9001"
	}
	if event.VariantID == "" {
		event.VariantID = YearlyVariantID
	}
	meta := map[string]any{"event_name": event.EventName, "test_mode": true}
	if event.AccountReference != "" {
		meta["custom_data"] = map[string]string{billing.AccountReferenceField: event.AccountReference}
	}
	body, _ := json.Marshal(map[string]any{
		"meta": meta,
		"data": map[string]any{
			"type": "subscriptions", "id": event.SubscriptionID,
			"attributes": map[string]any{
				"store_id": json.Number(event.StoreID), "customer_id": json.Number(event.CustomerID), "variant_id": json.Number(event.VariantID),
				"order_id": 1, "product_id": 1, "status": event.Status, "cancelled": event.Status == "cancelled",
				"user_email": "never-used@example.com", "card_brand": "visa", "card_last_four": "4242",
				"renews_at": formatOptionalTime(event.RenewsAt), "ends_at": formatOptionalTime(event.EndsAt), "trial_ends_at": formatOptionalTime(event.TrialEndsAt),
				"created_at": event.UpdatedAt.UTC().Format(time.RFC3339Nano), "updated_at": event.UpdatedAt.UTC().Format(time.RFC3339Nano),
				"test_mode": true,
			},
		},
	})
	return body
}

// BuildInvoiceWebhook returns the body of a subscription payment event, which carries an invoice, not a subscription.
func BuildInvoiceWebhook(eventName, accountReference, subscriptionID string) []byte {
	body, _ := json.Marshal(map[string]any{
		"meta": map[string]any{"event_name": eventName, "custom_data": map[string]string{billing.AccountReferenceField: accountReference}},
		"data": map[string]any{
			"type": "subscription-invoices", "id": "77",
			"attributes": map[string]any{"store_id": json.Number(StoreID), "subscription_id": json.Number(subscriptionID), "status": "paid"},
		},
	})
	return body
}

func Sign(body []byte) string {
	return hex.EncodeToString(billing.ComputeSignature(WebhookSecret, body))
}

// SendWebhook posts body to the server's webhook endpoint with the signature Lemon Squeezy would send.
func SendWebhook(baseURL string, body []byte, signature string) (int, error) {
	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/billing/lemon-squeezy/webhook", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Event-Name", "subscription")
	request.Header.Set("X-Signature", signature)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, err
	}
	response.Body.Close()
	return response.StatusCode, nil
}
