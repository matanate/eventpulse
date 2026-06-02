package webhooks

import (
	"bytes"
	"encoding/base64"
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
	c1, err := EncryptSecret(plaintext, testKey)
	if err != nil {
		t.Fatalf("EncryptSecret (1): %v", err)
	}
	c2, err := EncryptSecret(plaintext, testKey)
	if err != nil {
		t.Fatalf("EncryptSecret (2): %v", err)
	}
	if c1 == c2 {
		t.Error("EncryptSecret should produce different ciphertexts (random nonce)")
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	plaintext := []byte("secret")
	ct, err := EncryptSecret(plaintext, testKey)
	if err != nil {
		t.Fatalf("EncryptSecret: %v", err)
	}

	wrongKey := bytes.Repeat([]byte("x"), 32)
	if _, err := DecryptSecret(ct, wrongKey); err == nil {
		t.Error("expected error when decrypting with wrong key")
	}
}

// TestDecrypt_Tampered flips a byte inside the raw binary blob (after base64
// decode) so the GCM authentication tag is exercised, not base64 rejection.
func TestDecrypt_Tampered(t *testing.T) {
	plaintext := []byte("secret")
	ct, err := EncryptSecret(plaintext, testKey)
	if err != nil {
		t.Fatalf("EncryptSecret: %v", err)
	}

	blob, err := base64.StdEncoding.DecodeString(ct)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	// Flip a byte in the ciphertext region (past the 12-byte nonce).
	blob[12] ^= 0x01
	tampered := base64.StdEncoding.EncodeToString(blob)

	if _, err := DecryptSecret(tampered, testKey); err == nil {
		t.Error("expected GCM auth error for tampered ciphertext")
	}
}

func TestDecrypt_InvalidBase64(t *testing.T) {
	if _, err := DecryptSecret("not!!base64", testKey); err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestDecrypt_TooShort(t *testing.T) {
	// Single byte decoded — shorter than GCM nonce (12 bytes).
	shortBlob := base64.StdEncoding.EncodeToString([]byte("a"))
	if _, err := DecryptSecret(shortBlob, testKey); err == nil {
		t.Error("expected error for ciphertext shorter than nonce")
	}
}

func TestEncryptDecrypt_InvalidKeyLength(t *testing.T) {
	plaintext := []byte("secret")

	// aes.NewCipher accepts 16, 24, and 32 bytes. Only lengths outside those
	// values are invalid at the crypto layer. Our constructors enforce 32 exactly,
	// but the functions themselves should still reject non-AES key sizes.
	invalidKeys := [][]byte{
		nil,
		{},
		bytes.Repeat([]byte("k"), 7),  // too short
		bytes.Repeat([]byte("k"), 15), // one byte under AES-128
		bytes.Repeat([]byte("k"), 17), // between AES-128 and AES-192
		bytes.Repeat([]byte("k"), 33), // one byte over AES-256
	}
	for _, k := range invalidKeys {
		if _, err := EncryptSecret(plaintext, k); err == nil {
			t.Errorf("EncryptSecret: expected error for key length %d", len(k))
		}
	}

	ct, err := EncryptSecret(plaintext, testKey)
	if err != nil {
		t.Fatalf("EncryptSecret: %v", err)
	}
	for _, k := range invalidKeys {
		if _, err := DecryptSecret(ct, k); err == nil {
			t.Errorf("DecryptSecret: expected error for key length %d", len(k))
		}
	}
}
