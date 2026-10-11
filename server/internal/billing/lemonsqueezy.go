// Package billing talks to Lemon Squeezy, the merchant of record: checkouts, the customer portal and webhooks.
package billing

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/server/internal/plan"
)

const (
	DefaultAPIURL         = "https://api.lemonsqueezy.com"
	AccountReferenceField = "account_reference"
	jsonAPIContentType    = "application/vnd.api+json"
	maxResponseSize       = 1 << 20
)

var ErrUnknownVariant = errors.New("billing: the subscription is not for a Hisar Pro variant")

type Config struct {
	APIURL           string
	APIKey           string
	StoreID          string
	WebhookSecret    string
	MonthlyVariantID string
	YearlyVariantID  string
	// RedirectURL is where the checkout sends the buyer afterwards; empty keeps Lemon Squeezy's receipt page.
	RedirectURL string
	HTTP        *http.Client
}

type LemonSqueezy struct {
	config Config
}

func CreateLemonSqueezy(config Config) (*LemonSqueezy, error) {
	if config.APIKey == "" || config.StoreID == "" || config.WebhookSecret == "" || config.MonthlyVariantID == "" || config.YearlyVariantID == "" {
		return nil, errors.New("billing needs an API key, a store ID, a webhook signing secret and both variant IDs")
	}
	if config.MonthlyVariantID == config.YearlyVariantID {
		return nil, errors.New("billing needs different monthly and yearly variant IDs")
	}
	if config.APIURL == "" {
		config.APIURL = DefaultAPIURL
	}
	if _, err := url.Parse(config.APIURL); err != nil {
		return nil, fmt.Errorf("billing API URL: %w", err)
	}
	config.APIURL = strings.TrimRight(config.APIURL, "/")
	if config.HTTP == nil {
		config.HTTP = &http.Client{Timeout: 20 * time.Second}
	}
	return &LemonSqueezy{config: config}, nil
}

// VerifySignature checks X-Signature, the hex HMAC-SHA256 of the raw body with the webhook's signing secret.
func (l *LemonSqueezy) VerifySignature(body []byte, signature string) bool {
	given, err := hex.DecodeString(strings.TrimSpace(signature))
	if err != nil || len(given) != sha256.Size {
		return false
	}
	return hmac.Equal(given, ComputeSignature(l.config.WebhookSecret, body))
}

func ComputeSignature(secret string, body []byte) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return mac.Sum(nil)
}

func (l *LemonSqueezy) GetVariantID(interval string) (string, bool) {
	switch interval {
	case proto.BillingIntervalMonthly:
		return l.config.MonthlyVariantID, true
	case proto.BillingIntervalYearly:
		return l.config.YearlyVariantID, true
	}
	return "", false
}

func (l *LemonSqueezy) GetInterval(variantID string) (string, bool) {
	switch variantID {
	case l.config.MonthlyVariantID:
		return proto.BillingIntervalMonthly, true
	case l.config.YearlyVariantID:
		return proto.BillingIntervalYearly, true
	}
	return "", false
}

type CheckoutInput struct {
	Interval         string
	Email            string
	AccountReference string
}

// CreateCheckout returns a checkout URL with the chosen interval selected and the other one offered.
func (l *LemonSqueezy) CreateCheckout(ctx context.Context, input CheckoutInput) (string, error) {
	variantID, ok := l.GetVariantID(input.Interval)
	if !ok {
		return "", fmt.Errorf("billing: unknown interval %q", input.Interval)
	}
	allEnabledVariant := []int64{}
	for _, enabledVariantID := range []string{l.config.YearlyVariantID, l.config.MonthlyVariantID} {
		if number, err := strconv.ParseInt(enabledVariantID, 10, 64); err == nil {
			allEnabledVariant = append(allEnabledVariant, number)
		}
	}
	productOptions := map[string]any{"enabled_variants": allEnabledVariant}
	if l.config.RedirectURL != "" {
		productOptions["redirect_url"] = l.config.RedirectURL
	}
	body := map[string]any{
		"data": map[string]any{
			"type": "checkouts",
			"attributes": map[string]any{
				"checkout_data": map[string]any{
					"email":  input.Email,
					"custom": map[string]string{AccountReferenceField: input.AccountReference},
				},
				"product_options": productOptions,
			},
			"relationships": map[string]any{
				"store":   map[string]any{"data": map[string]string{"type": "stores", "id": l.config.StoreID}},
				"variant": map[string]any{"data": map[string]string{"type": "variants", "id": variantID}},
			},
		},
	}
	var response struct {
		Data struct {
			Attributes struct {
				URL string `json:"url"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := l.sendRequest(ctx, http.MethodPost, "/v1/checkouts", body, &response); err != nil {
		return "", err
	}
	if !strings.HasPrefix(response.Data.Attributes.URL, "https://") {
		return "", errors.New("billing: the checkout has no https URL")
	}
	return response.Data.Attributes.URL, nil
}

// GetCustomerPortalURL returns the subscription's signed customer portal link, valid for a day.
func (l *LemonSqueezy) GetCustomerPortalURL(ctx context.Context, subscriptionID string) (string, error) {
	var response struct {
		Data struct {
			Attributes struct {
				URLs struct {
					CustomerPortal string `json:"customer_portal"`
				} `json:"urls"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := l.sendRequest(ctx, http.MethodGet, "/v1/subscriptions/"+url.PathEscape(subscriptionID), nil, &response); err != nil {
		return "", err
	}
	if !strings.HasPrefix(response.Data.Attributes.URLs.CustomerPortal, "https://") {
		return "", errors.New("billing: the subscription has no customer portal URL")
	}
	return response.Data.Attributes.URLs.CustomerPortal, nil
}

func (l *LemonSqueezy) sendRequest(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, l.config.APIURL+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", jsonAPIContentType)
	if input != nil {
		request.Header.Set("Content-Type", jsonAPIContentType)
	}
	request.Header.Set("Authorization", "Bearer "+l.config.APIKey)
	response, err := l.config.HTTP.Do(request)
	if err != nil {
		return fmt.Errorf("billing: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize))
	if err != nil {
		return err
	}
	if response.StatusCode >= 300 {
		return fmt.Errorf("billing: %s %s answered %d", method, path, response.StatusCode)
	}
	return json.Unmarshal(data, output)
}

// WebhookEvent: Subscription is set for events about a subscription object, nil for invoices and anything else.
type WebhookEvent struct {
	EventName        string
	AccountReference string
	StoreID          string
	Subscription     *plan.Subscription
}

type webhookPayload struct {
	Meta struct {
		EventName  string         `json:"event_name"`
		CustomData map[string]any `json:"custom_data"`
	} `json:"meta"`
	Data struct {
		Type       string          `json:"type"`
		ID         string          `json:"id"`
		Attributes json.RawMessage `json:"attributes"`
	} `json:"data"`
}

type subscriptionAttributes struct {
	StoreID     json.Number `json:"store_id"`
	CustomerID  json.Number `json:"customer_id"`
	VariantID   json.Number `json:"variant_id"`
	Status      string      `json:"status"`
	RenewsAt    *time.Time  `json:"renews_at"`
	EndsAt      *time.Time  `json:"ends_at"`
	TrialEndsAt *time.Time  `json:"trial_ends_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// ParseWebhook reads a verified webhook body; only the fields Hisar stores are kept.
func (l *LemonSqueezy) ParseWebhook(body []byte) (*WebhookEvent, error) {
	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("billing: webhook: %w", err)
	}
	if payload.Meta.EventName == "" {
		return nil, errors.New("billing: webhook without an event name")
	}
	event := &WebhookEvent{EventName: payload.Meta.EventName}
	if reference, ok := payload.Meta.CustomData[AccountReferenceField].(string); ok {
		event.AccountReference = reference
	}
	if payload.Data.Type != "subscriptions" {
		return event, nil
	}
	var attributes subscriptionAttributes
	if err := json.Unmarshal(payload.Data.Attributes, &attributes); err != nil {
		return nil, fmt.Errorf("billing: webhook subscription: %w", err)
	}
	if payload.Data.ID == "" || attributes.UpdatedAt.IsZero() || !plan.IsKnownSubscriptionStatus(attributes.Status) {
		return nil, errors.New("billing: webhook subscription is incomplete")
	}
	event.StoreID = attributes.StoreID.String()
	interval, ok := l.GetInterval(attributes.VariantID.String())
	if !ok {
		return event, ErrUnknownVariant
	}
	event.Subscription = &plan.Subscription{
		ID: payload.Data.ID, CustomerID: attributes.CustomerID.String(), VariantID: attributes.VariantID.String(), Interval: interval,
		Status: attributes.Status, RenewsAt: attributes.RenewsAt, EndsAt: attributes.EndsAt, TrialEndsAt: attributes.TrialEndsAt,
		ProviderUpdatedAt: attributes.UpdatedAt,
	}
	return event, nil
}

func (l *LemonSqueezy) IsOwnStore(storeID string) bool { return storeID == l.config.StoreID }
