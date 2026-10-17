package httpserver

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

type rateLimitEntry struct {
	windowStart time.Time
	count       int
}

type IPRateLimiter struct {
	mu          sync.Mutex
	entries     map[string]rateLimitEntry
	limit       int
	window      time.Duration
	nextCleanup time.Time
	now         func() time.Time
}

func NewIPRateLimiter(limit int, window time.Duration) *IPRateLimiter {
	if limit < 1 {
		panic("rate limit must be positive")
	}
	if window <= 0 {
		panic("rate limit window must be positive")
	}

	return &IPRateLimiter{
		entries: make(map[string]rateLimitEntry),
		limit:   limit,
		window:  window,
		now:     time.Now,
	}
}

// Allow reports whether the client IP may make another request.
func (l *IPRateLimiter) Allow(ip string) (bool, time.Duration) {
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	if !now.Before(l.nextCleanup) {
		for key, entry := range l.entries {
			if now.Sub(entry.windowStart) >= l.window {
				delete(l.entries, key)
			}
		}
		l.nextCleanup = now.Add(l.window)
	}

	entry, exists := l.entries[ip]
	if !exists || now.Sub(entry.windowStart) >= l.window {
		entry = rateLimitEntry{windowStart: now}
	}

	if entry.count >= l.limit {
		retryAfter := l.window - now.Sub(entry.windowStart)
		if retryAfter < 0 {
			retryAfter = 0
		}
		return false, retryAfter
	}

	entry.count++
	l.entries[ip] = entry

	return true, 0
}

// Middleware limits requests using the client IP resolved by the outer
// HTTP middleware. It must be placed inside Middleware().
func (l *IPRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ClientInfoFromContext(r.Context()).ClientIP

		allowed, retryAfter := l.Allow(ip)
		if !allowed {
			seconds := int((retryAfter + time.Second - 1) / time.Second)
			if seconds < 1 {
				seconds = 1
			}

			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}
