package infrastructure

import (
	"sync"
	"time"
)

type RateLimiter struct {
	requests map[string][]time.Time
	window   time.Duration
	limit    int
	mutex    sync.RWMutex
}

func NewRateLimiter(window time.Duration, limit int) *RateLimiter {
	// Get rate limiting configuration from environment variables
	rateLimitWindow := GetEnvAsDuration("RATE_LIMIT_WINDOW", window)
	rateLimitMaxRequests := GetEnvAsInt("RATE_LIMIT_MAX_REQUESTS", limit)

	rl := &RateLimiter{
		requests: make(map[string][]time.Time),
		window:   rateLimitWindow,
		limit:    rateLimitMaxRequests,
	}

	// Start cleanup goroutine
	go rl.cleanupStaleEntries()
	return rl
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	// Get existing requests for this key
	requests, exists := rl.requests[key]
	if !exists {
		requests = []time.Time{}
	}

	// Remove old requests outside the window
	var validRequests []time.Time
	for _, reqTime := range requests {
		if reqTime.After(windowStart) {
			validRequests = append(validRequests, reqTime)
		}
	}

	// Check if we're under the limit
	if len(validRequests) < rl.limit {
		// Add current request
		validRequests = append(validRequests, now)
		rl.requests[key] = validRequests
		return true
	}

	// Update requests list even if we're over limit
	rl.requests[key] = validRequests
	return false
}

func (rl *RateLimiter) cleanupStaleEntries() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		rl.mutex.Lock()
		now := time.Now()
		cutoff := now.Add(-rl.window)

		for key, requests := range rl.requests {
			var validRequests []time.Time
			for _, reqTime := range requests {
				if reqTime.After(cutoff) {
					validRequests = append(validRequests, reqTime)
				}
			}

			if len(validRequests) == 0 {
				delete(rl.requests, key)
			} else {
				rl.requests[key] = validRequests
			}
		}
		rl.mutex.Unlock()
	}
}
