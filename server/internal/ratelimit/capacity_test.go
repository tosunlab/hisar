package ratelimit

import (
	"github.com/tryhisar/hisar/server/internal/boundedstate"
	"golang.org/x/time/rate"
	"runtime"
	"strconv"
	"testing"
	"time"
)

func TestCapacityPreservesDebtAndAggregateBudget(t *testing.T) {
	budget := boundedstate.NewBudget(3)
	a := NewWithBudget(rate.Every(24*time.Hour), 1, 2, budget)
	b := NewWithBudget(rate.Every(24*time.Hour), 1, 2, budget)
	start := time.Now()
	now := start
	a.now = func() time.Time { return now }
	b.now = a.now
	if !a.Allow("spent") || !a.Allow("second") || a.Allow("overflow") || !b.Allow("third") || b.Allow("overflow") {
		t.Fatal("capacity or aggregate limit failed")
	}
	for range 10000 {
		if a.Allow("overflow") || b.Allow("new") {
			t.Fatal("overflow obtained a fresh burst")
		}
	}
	now = start.Add(11 * time.Minute)
	a.Maintain()
	b.Maintain()
	if a.Allow("spent") || budget.Used() != 3 {
		t.Fatal("idle expiry erased daily debt")
	}
	now = start.Add(24*time.Hour + 11*time.Minute)
	a.Maintain()
	b.Maintain()
	if budget.Used() != 0 || !a.Allow("spent") {
		t.Fatal("fully repaid idle state was not reclaimed")
	}
}

func TestBurstIncreaseDoesNotExpireOutstandingDebt(t *testing.T) {
	limiter := NewWithBudget(rate.Every(time.Hour), 1, 2, boundedstate.NewBudget(2))
	now := time.Now()
	limiter.now = func() time.Time { return now }
	if !limiter.Allow("spent") {
		t.Fatal("initial burst refused")
	}
	limiter.SetBurst(3)
	now = now.Add(time.Hour + time.Second)
	limiter.Maintain()
	if !limiter.Allow("spent") || limiter.Allow("spent") {
		t.Fatal("raising burst reset existing debt through expiry")
	}
}
func TestCapacityReleasesBackingAndKeepsOneHeapNode(t *testing.T) {
	limiter := NewWithBudget(rate.Every(time.Second), 2, 1000, boundedstate.NewBudget(1000))
	now := time.Now()
	limiter.now = func() time.Time { return now }
	for index := range 1000 {
		if !limiter.Allow(strconv.Itoa(index)) {
			t.Fatal("premature capacity refusal")
		}
	}
	for range 10000 {
		limiter.Allow("0")
	}
	if limiter.buckets.Len() != 1000 {
		t.Fatal("known-key updates grew expiry state")
	}
	now = now.Add(11 * time.Minute)
	limiter.Maintain()
	if limiter.buckets.Len() != 0 {
		t.Fatal("idle storage remained")
	}
}
func TestMeasuredLimiterCapacityHeap(t *testing.T) {
	// A retained-heap sample, not a throughput or deployment-capacity benchmark.
	runtime.GC()
	var before, full, after runtime.MemStats
	runtime.ReadMemStats(&before)
	limiter := NewWithBudget(rate.Every(time.Hour), 1, DefaultKeyCapacity, boundedstate.NewBudget(DefaultKeyCapacity))
	now := time.Now()
	limiter.now = func() time.Time { return now }
	accepted, refused := 0, 0
	for index := range DefaultKeyCapacity * 2 {
		if limiter.Allow(strconv.Itoa(index)) {
			accepted++
		} else {
			refused++
		}
	}
	runtime.GC()
	runtime.ReadMemStats(&full)
	now = now.Add(11 * time.Hour)
	for range 65 {
		limiter.Maintain()
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(limiter)
	if accepted > DefaultKeyCapacity || refused == 0 || limiter.buckets.Len() != 0 {
		t.Fatalf("accepted %d refused %d retained %d", accepted, refused, limiter.buckets.Len())
	}
	t.Logf("one-run heap sample accepted=%d refused=%d before=%d full=%d reclaimed=%d bytes", accepted, refused, before.HeapAlloc, full.HeapAlloc, after.HeapAlloc)
}
