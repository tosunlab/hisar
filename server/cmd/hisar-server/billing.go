package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/tryhisar/hisar/server/internal/billing"
)

func createLemonSqueezy() (*billing.LemonSqueezy, error) {
	allRequiredName := []string{
		"HISAR_LEMON_SQUEEZY_API_KEY", "HISAR_LEMON_SQUEEZY_STORE_ID", "HISAR_LEMON_SQUEEZY_WEBHOOK_SECRET",
		"HISAR_LEMON_SQUEEZY_MONTHLY_VARIANT_ID", "HISAR_LEMON_SQUEEZY_YEARLY_VARIANT_ID",
	}
	setCount := 0
	for _, name := range allRequiredName {
		if os.Getenv(name) != "" {
			setCount++
		}
	}
	if setCount == 0 {
		return nil, nil
	}
	if setCount != len(allRequiredName) {
		return nil, errors.New("plans need HISAR_LEMON_SQUEEZY_API_KEY, HISAR_LEMON_SQUEEZY_STORE_ID, HISAR_LEMON_SQUEEZY_WEBHOOK_SECRET, HISAR_LEMON_SQUEEZY_MONTHLY_VARIANT_ID and HISAR_LEMON_SQUEEZY_YEARLY_VARIANT_ID together")
	}
	lemonSqueezy, err := billing.CreateLemonSqueezy(billing.Config{
		APIURL:           os.Getenv("HISAR_LEMON_SQUEEZY_API_URL"),
		APIKey:           os.Getenv("HISAR_LEMON_SQUEEZY_API_KEY"),
		StoreID:          os.Getenv("HISAR_LEMON_SQUEEZY_STORE_ID"),
		WebhookSecret:    os.Getenv("HISAR_LEMON_SQUEEZY_WEBHOOK_SECRET"),
		MonthlyVariantID: os.Getenv("HISAR_LEMON_SQUEEZY_MONTHLY_VARIANT_ID"),
		YearlyVariantID:  os.Getenv("HISAR_LEMON_SQUEEZY_YEARLY_VARIANT_ID"),
		RedirectURL:      os.Getenv("HISAR_LEMON_SQUEEZY_REDIRECT_URL"),
	})
	if err != nil {
		return nil, fmt.Errorf("plans: %w", err)
	}
	log.Printf("plans sold through Lemon Squeezy store %s", os.Getenv("HISAR_LEMON_SQUEEZY_STORE_ID"))
	return lemonSqueezy, nil
}
