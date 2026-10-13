package config

import (
	"strings"
	"testing"
)

func TestValidateTrustedProxies(t *testing.T) {
	tests := []struct {
		name    string
		proxies []string
		wantErr bool
	}{
		{
			name:    "IPv4 address",
			proxies: []string{"10.0.0.1"},
		},
		{
			name:    "IPv4 CIDR",
			proxies: []string{"10.0.0.0/8"},
		},
		{
			name:    "IPv6 address",
			proxies: []string{"2001:db8::1"},
		},
		{
			name:    "IPv6 CIDR",
			proxies: []string{"2001:db8::/32"},
		},
		{
			name:    "invalid IP",
			proxies: []string{"10.0.0.999"},
			wantErr: true,
		},
		{
			name:    "invalid CIDR",
			proxies: []string{"10.0.0.0/99"},
			wantErr: true,
		},
		{
			name:    "mixed valid and invalid entries",
			proxies: []string{"10.0.0.1", "not-an-ip"},
			wantErr: true,
		},
		{
			name:    "IPv4 address with surrounding spaces",
			proxies: []string{" 10.0.0.1 "},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{}
			cfg.HTTP.TrustedProxies = tt.proxies

			err := cfg.Validate()

			if tt.wantErr {
				if err == nil {
					t.Fatal("Validate() error = nil, want an error")
				}
				if !strings.Contains(err.Error(), "http.trusted_proxies") {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}

			if err != nil && strings.Contains(err.Error(), "http.trusted_proxies") {
				t.Fatalf("unexpected trusted proxy validation error: %v", err)
			}
		})
	}
}
