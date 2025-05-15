package infrastructure
// RateLimiter provides basic rate limiting functionality
import (
	"time"
	"sync"
)

type RateLimiter struct {
    attempts map[string]int
    lastTry  map[string]time.Time
    mutex    sync.Mutex
    window   time.Duration
    maxTries int
}

// NewRateLimiter creates a new rate limiter
// window: time period for rate limiting
// maxTries: maximum number of attempts allowed within window
func NewRateLimiter(window time.Duration, maxTries int) *RateLimiter {
    return &RateLimiter{
        attempts: make(map[string]int),
        lastTry:  make(map[string]time.Time),
        window:   window,
        maxTries: maxTries,
    }
}

// Allow checks if a request from the given identifier should be allowed
// Returns true if the request is allowed, false if it exceeds the rate limit
func (rl *RateLimiter) Allow(identifier string) bool {
    rl.mutex.Lock()
    defer rl.mutex.Unlock()

    now := time.Now()
    lastTry, exists := rl.lastTry[identifier]
    
    // Reset counter if window has passed
    if !exists || now.Sub(lastTry) > rl.window {
        rl.attempts[identifier] = 1
        rl.lastTry[identifier] = now
        return true
    }
    
    // Check if we've exceeded the max attempts
    if rl.attempts[identifier] >= rl.maxTries {
        return false
    }
    
    // Increment the counter and update the timestamp
    rl.attempts[identifier]++
    rl.lastTry[identifier] = now
    return true
}

// GetRemainingAttempts returns the number of remaining attempts for the identifier
func (rl *RateLimiter) GetRemainingAttempts(identifier string) int {
    rl.mutex.Lock()
    defer rl.mutex.Unlock()
    
    now := time.Now()
    lastTry, exists := rl.lastTry[identifier]
    
    // If no attempts or window reset, return max tries
    if !exists || now.Sub(lastTry) > rl.window {
        return rl.maxTries
    }
    
    remaining := rl.maxTries - rl.attempts[identifier]
    if remaining < 0 {
        return 0
    }
    return remaining
}

// GetTimeToReset returns the duration until the rate limit resets for the identifier
func (rl *RateLimiter) GetTimeToReset(identifier string) time.Duration {
    rl.mutex.Lock()
    defer rl.mutex.Unlock()
    
    lastTry, exists := rl.lastTry[identifier]
    if !exists {
        return 0
    }
    
    resetTime := lastTry.Add(rl.window)
    now := time.Now()
    
    if now.After(resetTime) {
        return 0
    }
    
    return resetTime.Sub(now)
}

// CleanupStaleEntries removes entries that have expired from the maps
// Should be called periodically to prevent memory leaks
func (rl *RateLimiter) CleanupStaleEntries() {
    rl.mutex.Lock()
    defer rl.mutex.Unlock()
    
    now := time.Now()
    for id, lastTry := range rl.lastTry {
        if now.Sub(lastTry) > rl.window {
            delete(rl.attempts, id)
            delete(rl.lastTry, id)
        }
    }
}