package ratelimit

import (
	"testing"

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
