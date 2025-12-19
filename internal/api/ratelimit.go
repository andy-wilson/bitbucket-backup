// Package api provides a Bitbucket Cloud API client with rate limiting.
package api

import (
	"math"
	"math/rand"
	"sync"
	"time"
)

// RateLimiter implements a token bucket rate limiter with support for
// exponential backoff when rate limits are hit.
type RateLimiter struct {
	mu sync.Mutex

	// Token bucket
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time

	// Backoff settings
	maxRetries        int
	baseBackoff       time.Duration
	backoffMultiplier float64
	maxBackoff        time.Duration

	// Current backoff state
	consecutiveFailures int
}

// RateLimiterConfig holds configuration for the rate limiter.
type RateLimiterConfig struct {
	RequestsPerHour        int
	BurstSize              int
	MaxRetries             int
	RetryBackoffSeconds    int
	RetryBackoffMultiplier float64
	MaxBackoffSeconds      int
}

// NewRateLimiter creates a new rate limiter with the given configuration.
func NewRateLimiter(cfg RateLimiterConfig) *RateLimiter {
	refillRate := float64(cfg.RequestsPerHour) / 3600.0 // tokens per second

	return &RateLimiter{
		tokens:            float64(cfg.BurstSize),
		maxTokens:         float64(cfg.BurstSize),
		refillRate:        refillRate,
		lastRefill:        time.Now(),
		maxRetries:        cfg.MaxRetries,
		baseBackoff:       time.Duration(cfg.RetryBackoffSeconds) * time.Second,
		backoffMultiplier: cfg.RetryBackoffMultiplier,
		maxBackoff:        time.Duration(cfg.MaxBackoffSeconds) * time.Second,
	}
}

// Wait blocks until a token is available, then consumes one token.
// Returns an error if the context is cancelled.
func (r *RateLimiter) Wait() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.refill()

	// If we have tokens, use one immediately
	if r.tokens >= 1 {
		r.tokens--
		return
	}

	// Calculate how long until we have a token
	deficit := 1 - r.tokens
	waitTime := time.Duration(deficit/r.refillRate*1000) * time.Millisecond

	r.mu.Unlock()
	time.Sleep(waitTime)
	r.mu.Lock()

	r.refill()
	r.tokens--
}

// refill adds tokens based on time elapsed since last refill.
// Must be called with mutex held.
func (r *RateLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(r.lastRefill).Seconds()
	r.tokens = math.Min(r.maxTokens, r.tokens+elapsed*r.refillRate)
	r.lastRefill = now
}

// OnSuccess should be called after a successful request.
// It resets the consecutive failure counter.
func (r *RateLimiter) OnSuccess() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.consecutiveFailures = 0
}

// OnRateLimited should be called when a 429 response is received.
// It returns the duration to wait before retrying, and whether
// more retries are allowed.
func (r *RateLimiter) OnRateLimited() (time.Duration, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.consecutiveFailures++

	if r.consecutiveFailures > r.maxRetries {
		return 0, false
	}

	// Calculate exponential backoff with jitter
	backoff := r.calculateBackoff()
	return backoff, true
}

// calculateBackoff computes the backoff duration with jitter.
// Must be called with mutex held.
func (r *RateLimiter) calculateBackoff() time.Duration {
	// Exponential backoff: base * multiplier^(failures-1)
	multiplier := math.Pow(r.backoffMultiplier, float64(r.consecutiveFailures-1))
	backoff := time.Duration(float64(r.baseBackoff) * multiplier)

	// Cap at max backoff
	if backoff > r.maxBackoff {
		backoff = r.maxBackoff
	}

	// Add jitter (±25%)
	jitter := 0.75 + rand.Float64()*0.5
	backoff = time.Duration(float64(backoff) * jitter)

	return backoff
}

// GetRetryCount returns the current consecutive failure count.
func (r *RateLimiter) GetRetryCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.consecutiveFailures
}

// MaxRetries returns the maximum number of retries configured.
func (r *RateLimiter) MaxRetries() int {
	return r.maxRetries
}
