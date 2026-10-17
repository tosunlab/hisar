// Package ratelimit limits requests per key, an IP address or an account ID, with a token bucket for each key.
package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Keys not seen for this long are forgotten once the limiter holds more than maximumKeyCount of them.
const (
	maximumKeyCount = 10000
	idleKeyLifetime = 10 * time.Minute
)

// Limiter allows a burst of requests per key, refilled at a steady rate.
type Limiter struct {
	mutex   sync.Mutex
	every   rate.Limit
	burst   int
	buckets map[string]*bucket
}

type bucket struct {
	limiter *rate.Limiter
	seen    time.Time
}

func New(every rate.Limit, burst int) *Limiter {
	return &Limiter{every: every, burst: burst, buckets: map[string]*bucket{}}
}

// NewDaily allows count requests per key a day, all of them at once if the key has been quiet.
func NewDaily(count int) *Limiter {
	return New(rate.Every(24*time.Hour/time.Duration(count)), count)
}

func (limiter *Limiter) Allow(key string) bool {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	now := time.Now()
	if len(limiter.buckets) > maximumKeyCount {
		for bucketKey, entry := range limiter.buckets {
			if now.Sub(entry.seen) > idleKeyLifetime {
				delete(limiter.buckets, bucketKey)
			}
		}
	}
	entry, isFound := limiter.buckets[key]
	if !isFound {
		entry = &bucket{limiter: rate.NewLimiter(limiter.every, limiter.burst)}
		limiter.buckets[key] = entry
	}
	entry.seen = now
	return entry.limiter.Allow()
}

// SetBurst changes the burst of every key, the ones seen so far included, without forgetting what they have used.
func (limiter *Limiter) SetBurst(burst int) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	limiter.burst = burst
	for _, entry := range limiter.buckets {
		entry.limiter.SetBurst(burst)
	}
}
