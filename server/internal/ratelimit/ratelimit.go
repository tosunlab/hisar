// Package ratelimit limits requests per key without discarding outstanding rate-limit debt.
package ratelimit

import (
	"github.com/tryhisar/hisar/server/internal/boundedstate"
	"golang.org/x/time/rate"
	"sync"
	"time"
)

const (
	// DefaultKeyCapacity bounds each map; DefaultTotalCapacity bounds all process maps combined.
	DefaultKeyCapacity   = 65_536
	DefaultTotalCapacity = 262_144
	maximumKeyCount      = DefaultKeyCapacity
	idleKeyLifetime      = 10 * time.Minute
	maintenanceBatch     = 1024
)

var processBudget = boundedstate.NewBudget(DefaultTotalCapacity)

type Limiter struct {
	mutex   sync.Mutex
	every   rate.Limit
	burst   int
	buckets *boundedstate.Table[*rate.Limiter]
	now     func() time.Time
}

func New(every rate.Limit, burst int) *Limiter {
	return NewWithBudget(every, burst, DefaultKeyCapacity, processBudget)
}

// NewWithBudget shares a key budget across a group of limiters. Unknown keys are refused at capacity.
// Known keys keep their debt and remain usable. No overflow request gets a fresh bucket.
func NewWithBudget(every rate.Limit, burst, capacity int, budget *boundedstate.Budget) *Limiter {
	return &Limiter{every: every, burst: burst, buckets: boundedstate.New[*rate.Limiter](capacity, budget), now: time.Now}
}
func NewDaily(count int) *Limiter { return New(rate.Every(24*time.Hour/time.Duration(count)), count) }
func (limiter *Limiter) expiry(now time.Time, bucket *rate.Limiter) time.Time {
	if limiter.every == 0 && bucket.TokensAt(now) < float64(limiter.burst) {
		return time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
	}
	expiry := now.Add(idleKeyLifetime)
	if limiter.every > 0 && limiter.every != rate.Inf {
		refill := time.Duration((float64(limiter.burst) - bucket.TokensAt(now)) / float64(limiter.every) * float64(time.Second))
		expiry = maxTime(expiry, now.Add(refill+time.Nanosecond))
	}
	return expiry
}
func maxTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return b
	}
	return a
}
func (limiter *Limiter) Allow(key string) bool {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	now := limiter.now()
	limiter.buckets.Expire(now, 32, nil)
	hash := boundedstate.Key(key)
	entry := limiter.buckets.Get(hash)
	if entry == nil {
		bucket := rate.NewLimiter(limiter.every, limiter.burst)
		if !limiter.buckets.Insert(hash, bucket, now.Add(idleKeyLifetime)) {
			return false
		}
		entry = limiter.buckets.Get(hash)
	}
	allowed := entry.Value.AllowN(now, 1)
	limiter.buckets.Update(entry, limiter.expiry(now, entry.Value))
	return allowed
}

// Maintain expires only fully refilled, idle keys. It also reclaims empty table backing storage.
func (limiter *Limiter) Maintain() {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	limiter.buckets.Expire(limiter.now(), maintenanceBatch, nil)
}
func (limiter *Limiter) SetBurst(burst int) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	limiter.burst = burst
	now := limiter.now()
	limiter.buckets.Visit(func(entry *boundedstate.Entry[*rate.Limiter]) {
		entry.Value.SetBurstAt(now, burst)
		limiter.buckets.Update(entry, limiter.expiry(now, entry.Value))
	})
}
