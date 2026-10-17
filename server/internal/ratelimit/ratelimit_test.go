package ratelimit

import (
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestLimiterAllowsABurstPerKey(t *testing.T) {
	limiter := NewDaily(2)
	if !limiter.Allow("a") || !limiter.Allow("a") || limiter.Allow("a") {
		t.Error("the daily limit of 2 was not kept")
	}
	if !limiter.Allow("b") {
		t.Error("another key shares the first key's bucket")
	}
	if unlimited := New(rate.Inf, 1); !unlimited.Allow("a") || !unlimited.Allow("a") {
		t.Error("rate.Inf limited")
	}
}

func TestSetBurstAppliesToSeenKeys(t *testing.T) {
	limiter := New(rate.Every(time.Hour), 1)
	if !limiter.Allow("seen") || limiter.Allow("seen") {
		t.Fatal("a burst of 1 was not kept")
	}
	limiter.SetBurst(3)
	if !limiter.Allow("new") || !limiter.Allow("new") || !limiter.Allow("new") || limiter.Allow("new") {
		t.Error("a new key did not get the new burst of 3")
	}
	limiter.SetBurst(1)
	if !limiter.Allow("later") || limiter.Allow("later") {
		t.Error("a lowered burst was not kept")
	}
	if limiter.Allow("seen") {
		t.Error("raising the burst refilled a key that had used its burst")
	}
}
