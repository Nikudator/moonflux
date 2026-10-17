package httpserver

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestResolveClientIPIgnoresForwardedHeaderFromUntrustedRemote(t *testing.T) {
	got, source := resolveClientIP("198.51.100.7", "203.0.113.9", parseTrusted([]string{"10.0.0.0/8"}))
	if got != "198.51.100.7" || source != "remote_addr" {
		t.Fatalf("got (%q, %q)", got, source)
	}
}

func TestResolveClientIPUsesForwardedChainFromTrustedProxy(t *testing.T) {
	trusted := parseTrusted([]string{"10.0.0.0/8"})
	got, source := resolveClientIP("10.0.0.2", "203.0.113.9, 10.0.0.1", trusted)
	if got != "203.0.113.9" || source != "trusted_proxy" {
		t.Fatalf("got (%q, %q)", got, source)
	}
}

func TestResolveClientIPRejectsMalformedForwardedChain(t *testing.T) {
	got, source := resolveClientIP("10.0.0.2", "not-an-ip", parseTrusted([]string{"10.0.0.0/8"}))
	if got != "10.0.0.2" || source != "invalid_forwarded_chain" {
		t.Fatalf("got (%q, %q)", got, source)
	}
}
func TestMiddlewareLogsRequestDetails(t *testing.T) {
	var logs bytes.Buffer

	log := slog.New(slog.NewJSONHandler(&logs, nil))

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	})

	handler := Middleware(log, nil, "X-Request-ID", true, next)

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.RemoteAddr = "198.51.100.7:12345"
	req.Header.Set("User-Agent", "Moonflux-Test/1.0")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("expected X-Request-ID response header")
	}

	output := logs.String()

	for _, want := range []string{
		`"event":"http.request.completed"`,
		`"request_id":"` + rec.Header().Get("X-Request-ID") + `"`,
		`"client_ip":"198.51.100.7"`,
		`"http_method":"GET"`,
		`"http_path":"/missing"`,
		`"http_status":404`,
		`"user_agent":"Moonflux-Test/1.0"`,
	} {
		if !strings.Contains(output, want) {
			t.Errorf("log does not contain %s\nactual log: %s", want, output)
		}
	}
}
func TestMiddlewareRecoversFromPanic(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	handler := Middleware(log, nil, "X-Request-ID", true, next)

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	req.RemoteAddr = "198.51.100.7:12345"

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	if !strings.Contains(logs.String(), `"event":"http.handler.panic"`) {
		t.Errorf("expected panic event in logs, got: %s", logs.String())
	}

	if !strings.Contains(logs.String(), `"http_status":500`) {
		t.Errorf("expected HTTP status 500 in access log, got: %s", logs.String())
	}
}
func TestMiddlewareDoesNotReplaceStatusAfterResponseStarted(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial response"))
		panic("test panic after response started")
	})

	handler := Middleware(log, nil, "X-Request-ID", true, next)

	req := httptest.NewRequest(http.MethodGet, "/panic-after-write", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	if rec.Body.String() != "partial response" {
		t.Fatalf("body = %q, want %q", rec.Body.String(), "partial response")
	}

	if !strings.Contains(logs.String(), `"event":"http.handler.panic"`) {
		t.Errorf("expected panic event in logs, got: %s", logs.String())
	}

	if !strings.Contains(logs.String(), `"http_status":200`) {
		t.Errorf("expected HTTP status 200 in access log, got: %s", logs.String())
	}
}
func TestResolveClientIPStopsAtUntrustedProxy(t *testing.T) {
	trusted := []string{"10.0.0.0/8"}

	got, source := resolveClientIP(
		"10.0.0.2",
		"198.51.100.77, 192.0.2.10, 10.0.0.1",
		parseTrusted(trusted),
	)

	if got != "192.0.2.10" {
		t.Fatalf("client IP = %q, want %q", got, "192.0.2.10")
	}

	if source != "trusted_proxy" {
		t.Fatalf("source = %q, want %q", source, "trusted_proxy")
	}
}
func TestResolveClientIPWithIPv6ProxyChain(t *testing.T) {
	trusted := []string{"2001:db8:ffff::/48"}

	got, source := resolveClientIP(
		"2001:db8:ffff::2",
		"2001:db8:abcd::9, 2001:db8:ffff::1",
		parseTrusted(trusted),
	)

	if got != "2001:db8:abcd::9" {
		t.Fatalf("client IP = %q, want %q", got, "2001:db8:abcd::9")
	}

	if source != "trusted_proxy" {
		t.Fatalf("source = %q, want %q", source, "trusted_proxy")
	}
}
func TestResolveClientIPUnmapsIPv4MappedIPv6(t *testing.T) {
	got, source := resolveClientIP(
		"10.0.0.2",
		"::ffff:192.0.2.10",
		parseTrusted([]string{"10.0.0.0/8"}),
	)

	if got != "192.0.2.10" {
		t.Fatalf("client IP = %q, want %q", got, "192.0.2.10")
	}

	if source != "trusted_proxy" {
		t.Fatalf("source = %q, want %q", source, "trusted_proxy")
	}
}
func TestResolveClientIPIgnoresForwardedChainFromUntrustedRemote(t *testing.T) {
	got, source := resolveClientIP(
		"203.0.113.50",
		"192.0.2.10, 10.0.0.1",
		parseTrusted([]string{"10.0.0.0/8"}),
	)

	if got != "203.0.113.50" {
		t.Fatalf("client IP = %q, want %q", got, "203.0.113.50")
	}

	if source != "remote_addr" {
		t.Fatalf("source = %q, want %q", source, "remote_addr")
	}
}
func TestResolveClientIPRejectsInvalidIPv6InForwardedChain(t *testing.T) {
	got, source := resolveClientIP(
		"10.0.0.2",
		"2001:db8::zz, 10.0.0.1",
		parseTrusted([]string{"10.0.0.0/8"}),
	)

	if got != "10.0.0.2" {
		t.Fatalf("client IP = %q, want %q", got, "10.0.0.2")
	}

	if source != "invalid_forwarded_chain" {
		t.Fatalf(
			"source = %q, want %q",
			source,
			"invalid_forwarded_chain",
		)
	}
}
func TestParseTrustedMasksCIDR(t *testing.T) {
	trusted := parseTrusted([]string{"10.0.0.7/8"})

	if len(trusted) != 1 {
		t.Fatalf("trusted prefixes count = %d, want 1", len(trusted))
	}

	want := "10.0.0.0/8"
	if got := trusted[0].String(); got != want {
		t.Fatalf("trusted prefix = %q, want %q", got, want)
	}
}
func TestLoginRateLimiterIgnoresSpoofedForwardedIP(t *testing.T) {
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	limiter := NewIPRateLimiter(1, time.Minute)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := Middleware(
		log,
		nil, // Нет доверенных прокси.
		"X-Request-ID",
		false,
		limiter.Middleware(next),
	)

	sendRequest := func(forwardedIP string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
		req.RemoteAddr = "198.51.100.7:12345"
		req.Header.Set("X-Forwarded-For", forwardedIP)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	first := sendRequest("203.0.113.10")
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", first.Code)
	}

	second := sendRequest("203.0.113.20")
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429", second.Code)
	}

	if second.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}
}
