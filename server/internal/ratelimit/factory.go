package ratelimit

import (
	"github.com/tryhisar/hisar/server/internal/boundedstate"
	"golang.org/x/time/rate"
	"time"
)

// Factory owns one process's aggregate limiter budget, shared by API, service and approvals.
type Factory struct {
	keyCapacity int
	budget      *boundedstate.Budget
}

func NewFactory(keyCapacity, totalCapacity int) *Factory {
	if keyCapacity == 0 {
		keyCapacity = DefaultKeyCapacity
	}
	if totalCapacity == 0 {
		totalCapacity = DefaultTotalCapacity
	}
	return &Factory{keyCapacity: keyCapacity, budget: boundedstate.NewBudget(totalCapacity)}
}
func (factory *Factory) New(every rate.Limit, burst int) *Limiter {
	return NewWithBudget(every, burst, factory.keyCapacity, factory.budget)
}
func (factory *Factory) Daily(count int) *Limiter {
	return factory.New(rate.Every(24*time.Hour/time.Duration(count)), count)
}
