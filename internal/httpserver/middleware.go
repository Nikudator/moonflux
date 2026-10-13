package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

type requestContextKey uint8

const (
	requestIDKey requestContextKey = iota
	clientIPKey
	remoteIPKey
	ipSourceKey
)

type ClientInfo struct{ RequestID, ClientIP, RemoteIP, IPSource, UserAgent string }

func ClientInfoFromContext(ctx context.Context) ClientInfo {
	return ClientInfo{RequestID: stringValue(ctx, requestIDKey), ClientIP: stringValue(ctx, clientIPKey), RemoteIP: stringValue(ctx, remoteIPKey), IPSource: stringValue(ctx, ipSourceKey)}
}
func stringValue(ctx context.Context, key requestContextKey) string {
	v, _ := ctx.Value(key).(string)
	return v
}

func Middleware(log *slog.Logger, trustedProxyStrings []string, requestIDHeader string, accessLog bool, next http.Handler) http.Handler {
	trusted := parseTrusted(trustedProxyStrings)
	if strings.TrimSpace(requestIDHeader) == "" {
		requestIDHeader = "X-Request-ID"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		requestID := newRequestID()
		remote := remoteAddrIP(r.RemoteAddr)
		client, source := resolveClientIP(remote, r.Header.Get("X-Forwarded-For"), trusted)
		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		ctx = context.WithValue(ctx, clientIPKey, client)
		ctx = context.WithValue(ctx, remoteIPKey, remote)
		ctx = context.WithValue(ctx, ipSourceKey, source)
		r = r.WithContext(ctx)
		w.Header().Set(requestIDHeader, requestID)
		wrapped := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Error("http.handler.panic", "event", "http.handler.panic", "request_id", requestID, "client_ip", client, "remote_ip", remote, "ip_source", source, "error", fmt.Sprint(recovered))
				if !wrapped.wroteHeader {
					http.Error(wrapped, "internal server error", http.StatusInternalServerError)
				}
			}
			if accessLog {
				attrs := []any{"event", "http.request.completed", "request_id", requestID, "client_ip", client, "remote_ip", remote, "ip_source", source, "http_method", r.Method, "http_path", safePath(r.URL.Path), "http_status", wrapped.status, "duration_ms", time.Since(started).Milliseconds(), "user_agent", truncate(r.UserAgent(), 1024)}
				if wrapped.status >= 500 {
					log.Error("http request completed", attrs...)
				} else if wrapped.status >= 400 {
					log.Warn("http request completed", attrs...)
				} else {
					log.Info("http request completed", attrs...)
				}
			}
		}()
		next.ServeHTTP(wrapped, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.status = code
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(code)
}
func (w *statusWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func parseTrusted(raw []string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if p, err := netip.ParsePrefix(item); err == nil {
			out = append(out, p.Masked())
			continue
		}
		if a, err := netip.ParseAddr(item); err == nil {
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
		}
	}
	return out
}
func isTrusted(addr netip.Addr, trusted []netip.Prefix) bool {
	for _, p := range trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}
func remoteAddrIP(raw string) string {
	host, _, err := net.SplitHostPort(raw)
	if err == nil {
		if a, e := netip.ParseAddr(strings.Trim(host, "[]")); e == nil {
			return a.Unmap().String()
		}
	}
	if a, err := netip.ParseAddr(strings.Trim(raw, "[]")); err == nil {
		return a.Unmap().String()
	}
	return ""
}
func resolveClientIP(remote, forwarded string, trusted []netip.Prefix) (string, string) {
	remoteAddr, err := netip.ParseAddr(remote)
	if err != nil {
		return "", "unresolved"
	}
	if !isTrusted(remoteAddr, trusted) {
		return remote, "remote_addr"
	}
	parts := strings.Split(forwarded, ",")
	if strings.TrimSpace(forwarded) == "" {
		return remote, "remote_addr"
	}
	chain := make([]netip.Addr, 0, len(parts))
	for _, part := range parts {
		a, parseErr := netip.ParseAddr(strings.TrimSpace(part))
		if parseErr != nil {
			return remote, "invalid_forwarded_chain"
		}
		chain = append(chain, a.Unmap())
	}
	current := remoteAddr.Unmap()
	for i := len(chain) - 1; i >= 0; i-- {
		if !isTrusted(current, trusted) {
			break
		}
		current = chain[i]
	}
	return current.String(), "trusted_proxy"
}
func safePath(path string) string {
	if path == "" {
		return "/"
	}
	return truncate(path, 2048)
}
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	// UUID v7 layout: 48-bit Unix milliseconds, version nibble, random payload, RFC 4122 variant.
	ms := uint64(time.Now().UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	b[6] = (b[6] & 0x0f) | 0x70
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
