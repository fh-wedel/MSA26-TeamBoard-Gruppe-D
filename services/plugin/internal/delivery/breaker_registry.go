package delivery

import (
	"sync"
	"time"

	"github.com/sony/gobreaker"
)

type BreakerRegistry struct {
	mu                  sync.Mutex
	breakers            sync.Map
	consecutiveFailures uint32
	timeout             time.Duration
}

func NewBreakerRegistry(consecutiveFailures uint32, timeout time.Duration) *BreakerRegistry {
	return &BreakerRegistry{
		consecutiveFailures: consecutiveFailures,
		timeout:             timeout,
	}
}

func (r *BreakerRegistry) Get(targetURL string) *gobreaker.CircuitBreaker {
	if b, ok := r.breakers.Load(targetURL); ok {
		return b.(*gobreaker.CircuitBreaker)
	}
	cf := r.consecutiveFailures
	settings := gobreaker.Settings{
		Name:        targetURL,
		MaxRequests: 1,
		Interval:    1 * time.Minute,
		Timeout:     r.timeout,
		ReadyToTrip: func(c gobreaker.Counts) bool {
			return c.ConsecutiveFailures >= cf
		},
	}
	b := gobreaker.NewCircuitBreaker(settings)
	actual, _ := r.breakers.LoadOrStore(targetURL, b)
	return actual.(*gobreaker.CircuitBreaker)
}
