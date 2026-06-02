package webhooks

import (
	"net"
	"testing"
)

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1",
		"127.127.127.127",
		"10.0.0.1",
		"10.255.255.255",
		"172.16.0.1",
		"172.31.255.255",
		"192.168.0.1",
		"192.168.255.255",
		"169.254.0.1",       // link-local / AWS metadata subnet
		"169.254.169.254",   // AWS instance metadata endpoint
		"100.64.0.1",        // CGNAT
		"0.0.0.1",
		"::1",               // IPv6 loopback
		"fe80::1",           // IPv6 link-local
		"fc00::1",           // IPv6 ULA
		"fd00::1",           // IPv6 ULA
		"::ffff:10.0.0.1",   // IPv4-mapped private
	}
	for _, addr := range blocked {
		ip := net.ParseIP(addr)
		if ip == nil {
			t.Fatalf("could not parse IP %q", addr)
		}
		if !isBlockedIP(ip) {
			t.Errorf("expected %s to be blocked", addr)
		}
	}

	allowed := []string{
		"8.8.8.8",
		"1.1.1.1",
		"93.184.216.34", // example.com
		"2001:4860:4860::8888",
	}
	for _, addr := range allowed {
		ip := net.ParseIP(addr)
		if ip == nil {
			t.Fatalf("could not parse IP %q", addr)
		}
		if isBlockedIP(ip) {
			t.Errorf("expected %s to be allowed", addr)
		}
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		allowHTTP bool
		wantErr   bool
	}{
		{"https ok", "https://example.com/hook", false, false},
		{"https with path", "https://example.com/hook?token=abc", false, false},
		{"http rejected in prod", "http://example.com/hook", false, true},
		{"http ok in dev", "http://example.com/hook", true, false},
		{"ftp rejected", "ftp://example.com/hook", false, true},
		{"no scheme", "example.com/hook", false, true},
		{"empty host", "https:///hook", false, true},
		{"loopback literal", "https://127.0.0.1/hook", false, true},
		{"private literal", "https://192.168.1.1/hook", false, true},
		{"metadata literal", "https://169.254.169.254/hook", false, true},
		{"public literal", "https://8.8.8.8/hook", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.url, tt.allowHTTP)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateURL(%q, %v) = %v; wantErr=%v", tt.url, tt.allowHTTP, err, tt.wantErr)
			}
		})
	}
}
