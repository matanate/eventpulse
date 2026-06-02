package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSign(t *testing.T) {
	secret := []byte("super-secret-key-1234567890")
	body := []byte(`{"event":"page_view","user_id":"u1"}`)

	got := Sign(secret, body)

	// Verify independently using the standard library.
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))

	if got != want {
		t.Errorf("Sign = %q, want %q", got, want)
	}
	// Verify constant-time equality.
	if !hmac.Equal([]byte(got), []byte(want)) {
		t.Error("hmac.Equal returned false for identical signatures")
	}
}

func TestSign_Deterministic(t *testing.T) {
	secret := []byte("key")
	body := []byte("body")
	sig1 := Sign(secret, body)
	sig2 := Sign(secret, body)
	if sig1 != sig2 {
		t.Error("Sign is not deterministic")
	}
}

func TestSign_DifferentSecretProducesDifferentSig(t *testing.T) {
	body := []byte("body")
	if Sign([]byte("key1"), body) == Sign([]byte("key2"), body) {
		t.Error("different secrets should produce different signatures")
	}
}

func TestDeliver_Success(t *testing.T) {
	var gotHeaders http.Header
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	payload := []byte(`{"event":"click"}`)
	secret := "my-secret-key-at-least-16-chars"

	client := NewClientWithTransport(http.DefaultTransport, 5*time.Second)
	result := client.Deliver(context.Background(), DeliveryWithSub{
		Delivery: Delivery{ID: "d1", Payload: payload},
		URL:      srv.URL,
		Secret:   secret,
	})

	if !result.Delivered {
		t.Fatalf("expected delivered=true, got err=%q status=%d", result.Err, result.StatusCode)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("unexpected status %d", result.StatusCode)
	}
	if gotHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotHeaders.Get("Content-Type"))
	}
	wantSig := "sha256=" + Sign([]byte(secret), payload)
	if gotHeaders.Get("X-EventPulse-Signature") != wantSig {
		t.Errorf("X-EventPulse-Signature = %q, want %q", gotHeaders.Get("X-EventPulse-Signature"), wantSig)
	}
	if string(gotBody) != string(payload) {
		t.Errorf("body = %q, want %q", gotBody, payload)
	}
}

func TestDeliver_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewClientWithTransport(http.DefaultTransport, 5*time.Second)
	result := client.Deliver(context.Background(), DeliveryWithSub{
		Delivery: Delivery{ID: "d2", Payload: []byte(`{}`)},
		URL:      srv.URL,
		Secret:   "secret-at-least-16-chars",
	})

	if result.Delivered {
		t.Error("expected delivered=false for 5xx response")
	}
	if result.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", result.StatusCode)
	}
}

func TestDeliver_RedirectNotFollowed(t *testing.T) {
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer redirectTarget.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL, http.StatusFound)
	}))
	defer redirector.Close()

	client := NewClientWithTransport(http.DefaultTransport, 5*time.Second)
	result := client.Deliver(context.Background(), DeliveryWithSub{
		Delivery: Delivery{ID: "d3", Payload: []byte(`{}`)},
		URL:      redirector.URL,
		Secret:   "secret-at-least-16-chars",
	})

	// The redirect response is a 302, not 2xx, so delivered=false.
	if result.Delivered {
		t.Error("client must not follow redirects")
	}
}

func TestDeliver_InvalidURL(t *testing.T) {
	client := NewClient(5*time.Second, false)
	result := client.Deliver(context.Background(), DeliveryWithSub{
		Delivery: Delivery{ID: "d4", Payload: []byte(`{}`)},
		URL:      "http://192.168.1.1/hook", // private IP, https-only client
		Secret:   "secret-at-least-16-chars",
	})
	if result.Delivered {
		t.Error("expected delivered=false for invalid URL")
	}
	if result.Err == "" {
		t.Error("expected non-empty Err for invalid URL")
	}
}
