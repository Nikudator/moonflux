package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestIPRateLimiterLimitAndWindowReset(t *testing.T) {
	now := time.Now()
	limiter := NewIPRateLimiter(2, time.Minute)
	limiter.now = func() time.Time {
		return now
	}

	for i := 0; i < 2; i++ {
		allowed, _ := limiter.Allow("192.0.2.1")
		if !allowed {
			t.Fatalf("request %d unexpectedly rejected", i+1)
		}
	}

	allowed, retryAfter := limiter.Allow("192.0.2.1")
	if allowed {
		t.Fatal("request above limit unexpectedly allowed")
	}
	if retryAfter <= 0 || retryAfter > time.Minute {
		t.Fatalf("retryAfter = %v, want a duration in (0, 1m]", retryAfter)
	}

	allowed, _ = limiter.Allow("192.0.2.2")
	if !allowed {
		t.Fatal("request from a different IP unexpectedly rejected")
	}

	now = now.Add(2 * time.Minute)

	allowed, _ = limiter.Allow("192.0.2.1")
	if !allowed {
		t.Fatal("request after window expiration unexpectedly rejected")
	}
}

func TestIPRateLimiterMiddlewareReturns429(t *testing.T) {
	limiter := NewIPRateLimiter(1, time.Minute)
	var calls atomic.Int32

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	})

	handler := limiter.Middleware(next)

	newRequest := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
		ctx := req.Context()
		ctx = contextWithClientIP(ctx, "192.0.2.10")
		return req.WithContext(ctx)
	}

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, newRequest())
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", first.Code)
	}

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, newRequest())
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429", second.Code)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}
	if calls.Load() != 1 {
		t.Fatalf("next handler calls = %d, want 1", calls.Load())
	}
}

func TestIPRateLimiterConcurrentRequests(t *testing.T) {
	const (
		limit    = 10
		requests = 100
	)

	limiter := NewIPRateLimiter(limit, time.Minute)
	var allowedCount atomic.Int32

	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed, _ := limiter.Allow("192.0.2.20")
			if allowed {
				allowedCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := allowedCount.Load(); got != limit {
		t.Fatalf("allowed requests = %d, want %d", got, limit)
	}
}

func TestIPRateLimiterRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		limit  int
		window time.Duration
	}{
		{limit: 0, window: time.Minute},
		{limit: -1, window: time.Minute},
		{limit: 1, window: 0},
		{limit: 1, window: -time.Second},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("limit_%s_window_%s",
			strconv.Itoa(tt.limit), tt.window), func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected panic for invalid configuration")
				}
			}()
			NewIPRateLimiter(tt.limit, tt.window)
		})
	}
}
func contextWithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPKey, ip)
}

func TestRegistrationRateLimiterReturns429(t *testing.T) {
	limiter := NewIPRateLimiter(1, time.Minute)
	var calls atomic.Int32

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusCreated)
	})

	handler := limiter.Middleware(next)

	newRequest := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/users", nil)
		ctx := contextWithClientIP(req.Context(), "192.0.2.30")
		return req.WithContext(ctx)
	}

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, newRequest())
	if first.Code != http.StatusCreated {
		t.Fatalf("first status = %d, want 201", first.Code)
	}

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, newRequest())
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429", second.Code)
	}

	if second.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("next handler calls = %d, want 1", got)
	}
}
