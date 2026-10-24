package main

import "testing"

func TestResourceCapacityConfiguration(t *testing.T) {
	for _, name := range []string{"HISAR_AUTH_PENDING_CAPACITY", "HISAR_LIMITER_KEY_CAPACITY", "HISAR_LIMITER_TOTAL_CAPACITY"} {
		t.Setenv(name, "")
	}
	config, err := readResourceConfig()
	if err != nil || config.PendingLoginCapacity != 0 || config.LimiterKeyCapacity != 0 || config.LimiterTotalCapacity != 0 {
		t.Fatalf("defaults: %+v %v", config, err)
	}
	t.Setenv("HISAR_AUTH_PENDING_CAPACITY", "32")
	t.Setenv("HISAR_LIMITER_KEY_CAPACITY", "64")
	t.Setenv("HISAR_LIMITER_TOTAL_CAPACITY", "128")
	config, err = readResourceConfig()
	if err != nil || config.PendingLoginCapacity != 32 || config.LimiterKeyCapacity != 64 || config.LimiterTotalCapacity != 128 {
		t.Fatalf("configured: %+v %v", config, err)
	}
	for _, value := range []string{"0", "-1", "invalid"} {
		t.Setenv("HISAR_AUTH_PENDING_CAPACITY", value)
		if _, err := readResourceConfig(); err == nil {
			t.Fatalf("invalid capacity %q accepted", value)
		}
	}
}
