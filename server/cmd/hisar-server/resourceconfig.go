package main

import (
	"fmt"
	"github.com/tryhisar/hisar/server/internal/service"
	"os"
	"strconv"
)

func readResourceConfig() (service.Config, error) {
	config := service.Config{PublicURL: getEnv("HISAR_PUBLIC_URL", "https://eu.tryhisar.com")}
	for _, entry := range []struct {
		name   string
		target *int
	}{
		{"HISAR_AUTH_PENDING_CAPACITY", &config.PendingLoginCapacity},
		{"HISAR_LIMITER_KEY_CAPACITY", &config.LimiterKeyCapacity},
		{"HISAR_LIMITER_TOTAL_CAPACITY", &config.LimiterTotalCapacity},
	} {
		value := os.Getenv(entry.name)
		if value == "" {
			continue
		}
		capacity, err := strconv.Atoi(value)
		if err != nil || capacity <= 0 {
			return service.Config{}, fmt.Errorf("%s must be a positive integer", entry.name)
		}
		*entry.target = capacity
	}
	return config, nil
}
