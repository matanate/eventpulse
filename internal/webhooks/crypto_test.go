package webhooks

import (
	"bytes"
	"testing"
)

var testKey = bytes.Repeat([]byte("k"), 32) // 32-byte AES-256 key for tests

func TestEncryptDecryptRoundTrip(t *testing.T) {
	plaintext := []byte("super-secret-hmac-signing-key")

	ciphertext, err := EncryptSecret(plaintext, testKey)
	if err != nil {
		t.Fatalf("EncryptSecret: %v", err)
	}
	if ciphertext == string(plaintext) {
		t.Error("ciphertext must not equal plaintext")
	}

	got, err := DecryptSecret(ciphertext, testKey)
	if err != nil {
		t.Fatalf("DecryptSecret: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("round-trip mismatch: got %q, want %q", got, plaintext)
	}
}

func TestEncrypt_NonDeterministic(t *testing.T) {
	plaintext := []byte("secret")
	c1, _ := EncryptSecret(plaintext, testKey)
	c2, _ := EncryptSecret(plaintext, testKey)
	if c1 == c2 {
		t.Error("EncryptSecret should produce different ciphertexts (random nonce)")
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	plaintext := []byte("secret")
	ct, _ := EncryptSecret(plaintext, testKey)

	wrongKey := bytes.Repeat([]byte("x"), 32)
	if _, err := DecryptSecret(ct, wrongKey); err == nil {
		t.Error("expected error when decrypting with wrong key")
	}
}

func TestDecrypt_Tampered(t *testing.T) {
	plaintext := []byte("secret")
	ct, _ := EncryptSecret(plaintext, testKey)

	// Flip a byte in the base64 blob.
	b := []byte(ct)
	b[len(b)/2] ^= 0xff
	if _, err := DecryptSecret(string(b), testKey); err == nil {
		t.Error("expected error for tampered ciphertext")
	}
}

func TestDecrypt_InvalidBase64(t *testing.T) {
	if _, err := DecryptSecret("not!!base64", testKey); err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestDecrypt_TooShort(t *testing.T) {
	import64 := "YQ==" // single byte, shorter than GCM nonce
	if _, err := DecryptSecret(import64, testKey); err == nil {
		t.Error("expected error for ciphertext shorter than nonce")
	}
}
