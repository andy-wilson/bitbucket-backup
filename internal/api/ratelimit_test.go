package api

import (
	"testing"
	"time"
)

func TestNewRateLimiter(t *testing.T) {
	cfg := RateLimiterConfig{
		RequestsPerHour:        3600, // 1 per second
		BurstSize:              10,
		MaxRetries:             5,
		RetryBackoffSeconds:    1,
		RetryBackoffMultiplier: 2.0,
		MaxBackoffSeconds:      60,
	}

	rl := NewRateLimiter(cfg)

	if rl.maxTokens != 10 {
		t.Errorf("expected maxTokens = 10, got %f", rl.maxTokens)
	}
	if rl.tokens != 10 {
		t.Errorf("expected initial tokens = 10, got %f", rl.tokens)
	}
	if rl.refillRate != 1.0 {
		t.Errorf("expected refillRate = 1.0, got %f", rl.refillRate)
	}
	if rl.maxRetries != 5 {
		t.Errorf("expected maxRetries = 5, got %d", rl.maxRetries)
	}
}

func TestRateLimiter_Wait_ImmediateWithTokens(t *testing.T) {
	cfg := RateLimiterConfig{
		RequestsPerHour:        3600,
		BurstSize:              10,
		MaxRetries:             3,
		RetryBackoffSeconds:    1,
		RetryBackoffMultiplier: 2.0,
		MaxBackoffSeconds:      60,
	}

	rl := NewRateLimiter(cfg)

	start := time.Now()
	rl.Wait()
	elapsed := time.Since(start)

	// Should be nearly instant when we have tokens
	if elapsed > 10*time.Millisecond {
		t.Errorf("Wait took too long with available tokens: %v", elapsed)
	}

	// Token should have been consumed
	rl.mu.Lock()
	if rl.tokens >= 10 {
		t.Errorf("expected tokens to be consumed, got %f", rl.tokens)
	}
	rl.mu.Unlock()
}

func TestRateLimiter_Wait_WaitsWhenEmpty(t *testing.T) {
	cfg := RateLimiterConfig{
		RequestsPerHour:        36000, // 10 per second for faster test
		BurstSize:              1,
		MaxRetries:             3,
		RetryBackoffSeconds:    1,
		RetryBackoffMultiplier: 2.0,
		MaxBackoffSeconds:      60,
	}

	rl := NewRateLimiter(cfg)

	// First call should be instant
	rl.Wait()

	// Second call should wait for refill (~100ms at 10/sec)
	start := time.Now()
	rl.Wait()
	elapsed := time.Since(start)

	// Should wait approximately 100ms (with some tolerance)
	if elapsed < 50*time.Millisecond {
		t.Errorf("Wait should have blocked, but took only %v", elapsed)
	}
}

func TestRateLimiter_OnRateLimited_RetriesAllowed(t *testing.T) {
	cfg := RateLimiterConfig{
		RequestsPerHour:        3600,
		BurstSize:              10,
		MaxRetries:             3,
		RetryBackoffSeconds:    1,
		RetryBackoffMultiplier: 2.0,
		MaxBackoffSeconds:      60,
	}

	rl := NewRateLimiter(cfg)

	// First failure
	backoff1, retry1 := rl.OnRateLimited()
	if !retry1 {
		t.Error("expected retry to be allowed on first failure")
	}
	if backoff1 < 500*time.Millisecond || backoff1 > 1500*time.Millisecond {
		t.Errorf("expected backoff around 1s (with jitter), got %v", backoff1)
	}

	// Second failure - should have longer backoff
	backoff2, retry2 := rl.OnRateLimited()
	if !retry2 {
		t.Error("expected retry to be allowed on second failure")
	}
	if backoff2 < 1*time.Second || backoff2 > 3*time.Second {
		t.Errorf("expected backoff around 2s (with jitter), got %v", backoff2)
	}

	// Third failure
	_, retry3 := rl.OnRateLimited()
	if !retry3 {
		t.Error("expected retry to be allowed on third failure")
	}

	// Fourth failure - should exceed max retries
	_, retry4 := rl.OnRateLimited()
	if retry4 {
		t.Error("expected retry to be denied after max retries")
	}
}

func TestRateLimiter_OnSuccess_ResetsFailures(t *testing.T) {
	cfg := RateLimiterConfig{
		RequestsPerHour:        3600,
		BurstSize:              10,
		MaxRetries:             3,
		RetryBackoffSeconds:    1,
		RetryBackoffMultiplier: 2.0,
		MaxBackoffSeconds:      60,
	}

	rl := NewRateLimiter(cfg)

	// Simulate some failures
	rl.OnRateLimited()
	rl.OnRateLimited()

	if rl.GetRetryCount() != 2 {
		t.Errorf("expected retry count = 2, got %d", rl.GetRetryCount())
	}

	// Success should reset
	rl.OnSuccess()

	if rl.GetRetryCount() != 0 {
		t.Errorf("expected retry count = 0 after success, got %d", rl.GetRetryCount())
	}
}

func TestRateLimiter_MaxBackoff(t *testing.T) {
	cfg := RateLimiterConfig{
		RequestsPerHour:        3600,
		BurstSize:              10,
		MaxRetries:             10,
		RetryBackoffSeconds:    10,
		RetryBackoffMultiplier: 10.0, // Aggressive multiplier
		MaxBackoffSeconds:      30,   // But capped at 30s
	}

	rl := NewRateLimiter(cfg)

	// Multiple failures to trigger max backoff
	for i := 0; i < 5; i++ {
		rl.OnRateLimited()
	}

	backoff, _ := rl.OnRateLimited()

	// Should be capped at max backoff (with jitter, so check upper bound)
	maxExpected := time.Duration(float64(30*time.Second) * 1.25) // max + 25% jitter
	if backoff > maxExpected {
		t.Errorf("backoff %v exceeded max expected %v", backoff, maxExpected)
	}
}

func TestRateLimiter_Refill(t *testing.T) {
	cfg := RateLimiterConfig{
		RequestsPerHour:        3600, // 1 per second
		BurstSize:              5,
		MaxRetries:             3,
		RetryBackoffSeconds:    1,
		RetryBackoffMultiplier: 2.0,
		MaxBackoffSeconds:      60,
	}

	rl := NewRateLimiter(cfg)

	// Consume all tokens
	for i := 0; i < 5; i++ {
		rl.Wait()
	}

	rl.mu.Lock()
	if rl.tokens > 0.1 {
		t.Errorf("expected tokens near 0, got %f", rl.tokens)
	}
	rl.mu.Unlock()

	// Wait for refill
	time.Sleep(2 * time.Second)

	rl.mu.Lock()
	rl.refill()
	if rl.tokens < 1.5 {
		t.Errorf("expected tokens to refill, got %f", rl.tokens)
	}
	rl.mu.Unlock()
}
