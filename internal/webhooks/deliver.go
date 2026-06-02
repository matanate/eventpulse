package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const (
	maxResponseBody = 64 << 10 // 64 KiB
	userAgent       = "EventPulse-Webhook/1"
)

// DeliveryResult holds the outcome of a single HTTP delivery attempt.
type DeliveryResult struct {
	StatusCode int
	Delivered  bool   // true when the endpoint returned 2xx
	Err        string // non-empty on network/SSRF/timeout errors
}

// Client makes signed webhook HTTP POST requests via an SSRF-hardened transport.
type Client struct {
	http        *http.Client
	allowHTTP   bool
	noValidate  bool // set by NewClientWithTransport — skips URL validation for tests
}

// NewClient creates a production Client with an SSRF-guarded transport and no
// redirect following. allowHTTP permits http:// targets (development only).
func NewClient(timeout time.Duration, allowHTTP bool) *Client {
	transport := &http.Transport{
		DialContext: safeDial(&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}),
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
	return &Client{
		http: &http.Client{
			Transport:     transport,
			CheckRedirect: noFollowRedirect,
			Timeout:       timeout,
		},
		allowHTTP: allowHTTP,
	}
}

// NewClientWithTransport creates a Client backed by a custom transport.
// Intended for tests that need to reach loopback httptest servers; skips SSRF
// URL validation because the caller provides the transport.
func NewClientWithTransport(transport http.RoundTripper, timeout time.Duration) *Client {
	return &Client{
		http: &http.Client{
			Transport:     transport,
			CheckRedirect: noFollowRedirect,
			Timeout:       timeout,
		},
		allowHTTP:  true,
		noValidate: true,
	}
}

// Deliver POSTs payload to d.URL with an HMAC-SHA256 signature header.
func (c *Client) Deliver(ctx context.Context, d DeliveryWithSub) DeliveryResult {
	if !c.noValidate {
		if err := ValidateURL(d.URL, c.allowHTTP); err != nil {
			return DeliveryResult{Err: fmt.Sprintf("URL validation: %s", err)}
		}
	}

	sig := Sign([]byte(d.Secret), d.Payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.URL, bytes.NewReader(d.Payload))
	if err != nil {
		return DeliveryResult{Err: fmt.Sprintf("build request: %s", err)}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-EventPulse-Signature", "sha256="+sig)
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return DeliveryResult{Err: err.Error()}
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBody)) //nolint:errcheck

	return DeliveryResult{
		StatusCode: resp.StatusCode,
		Delivered:  resp.StatusCode >= 200 && resp.StatusCode < 300,
	}
}

// Sign returns the hex-encoded HMAC-SHA256 of body keyed by secret.
func Sign(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body) //nolint:errcheck
	return hex.EncodeToString(mac.Sum(nil))
}

func noFollowRedirect(_ *http.Request, _ []*http.Request) error {
	return http.ErrUseLastResponse
}
