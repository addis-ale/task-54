package unit_tests

import (
	"crypto/rand"
	"encoding/base64"
	"testing"

	"clinic-admin-suite/internal/service"
)

func TestFieldCipherRoundTrip(t *testing.T) {
	// Generate a valid 32-byte AES-256-GCM key
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}
	keyBase64 := base64.StdEncoding.EncodeToString(key)

	cipher, err := service.NewFieldCipherFromBase64(keyBase64)
	if err != nil {
		t.Fatalf("create cipher: %v", err)
	}

	plaintext := "SSN:123-45-6789"
	encrypted, err := cipher.Encrypt([]byte(plaintext))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	if encrypted == plaintext {
		t.Fatal("encrypted text should not equal plaintext")
	}
	if encrypted == "" {
		t.Fatal("encrypted text should not be empty")
	}

	decrypted, err := cipher.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if string(decrypted) != plaintext {
		t.Fatalf("round-trip failed: expected %q, got %q", plaintext, string(decrypted))
	}
}

func TestFieldCipherRoundTripVariousInputs(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}
	keyBase64 := base64.StdEncoding.EncodeToString(key)

	cipher, err := service.NewFieldCipherFromBase64(keyBase64)
	if err != nil {
		t.Fatalf("create cipher: %v", err)
	}

	inputs := []string{
		"",
		"a",
		"short",
		"a longer string with special characters !@#$%^&*()",
		string(make([]byte, 1024)), // 1KB of null bytes
	}

	for _, input := range inputs {
		encrypted, err := cipher.Encrypt([]byte(input))
		if err != nil {
			t.Fatalf("encrypt %q: %v", input, err)
		}
		decrypted, err := cipher.Decrypt(encrypted)
		if err != nil {
			t.Fatalf("decrypt %q: %v", input, err)
		}
		if string(decrypted) != input {
			t.Fatalf("round-trip mismatch for input length %d", len(input))
		}
	}
}

func TestFieldCipherInvalidKey(t *testing.T) {
	// 16-byte key (too short for AES-256)
	shortKey := make([]byte, 16)
	if _, err := rand.Read(shortKey); err != nil {
		t.Fatalf("generate key: %v", err)
	}
	_, err := service.NewFieldCipherFromBase64(base64.StdEncoding.EncodeToString(shortKey))
	if err == nil {
		t.Fatal("expected error for 16-byte key (AES-256 requires 32)")
	}
}

func TestFieldCipherInvalidBase64Key(t *testing.T) {
	_, err := service.NewFieldCipherFromBase64("not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64 key")
	}
}

func TestFieldCipherCorruptedCiphertext(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}
	keyBase64 := base64.StdEncoding.EncodeToString(key)

	cipher, err := service.NewFieldCipherFromBase64(keyBase64)
	if err != nil {
		t.Fatalf("create cipher: %v", err)
	}

	encrypted, err := cipher.Encrypt([]byte("sensitive data"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Corrupt the ciphertext by modifying base64
	corrupted := encrypted[:len(encrypted)-4] + "XXXX"
	_, err = cipher.Decrypt(corrupted)
	if err == nil {
		t.Fatal("expected error for corrupted ciphertext")
	}
	if err != service.ErrInvalidCiphertext {
		t.Fatalf("expected ErrInvalidCiphertext, got: %v", err)
	}
}

func TestFieldCipherDecryptWithWrongKey(t *testing.T) {
	// Create two ciphers with different keys
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	if _, err := rand.Read(key1); err != nil {
		t.Fatalf("generate key1: %v", err)
	}
	if _, err := rand.Read(key2); err != nil {
		t.Fatalf("generate key2: %v", err)
	}

	cipher1, err := service.NewFieldCipherFromBase64(base64.StdEncoding.EncodeToString(key1))
	if err != nil {
		t.Fatalf("create cipher1: %v", err)
	}
	cipher2, err := service.NewFieldCipherFromBase64(base64.StdEncoding.EncodeToString(key2))
	if err != nil {
		t.Fatalf("create cipher2: %v", err)
	}

	encrypted, err := cipher1.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatalf("encrypt with cipher1: %v", err)
	}

	_, err = cipher2.Decrypt(encrypted)
	if err == nil {
		t.Fatal("expected error decrypting with wrong key")
	}
	if err != service.ErrInvalidCiphertext {
		t.Fatalf("expected ErrInvalidCiphertext, got: %v", err)
	}
}

func TestFieldCipherDecryptTruncatedPayload(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}
	cipher, err := service.NewFieldCipherFromBase64(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatalf("create cipher: %v", err)
	}

	// Truncated base64 that decodes to fewer bytes than nonce size
	_, err = cipher.Decrypt(base64.StdEncoding.EncodeToString([]byte("short")))
	if err == nil {
		t.Fatal("expected error for truncated ciphertext")
	}
	if err != service.ErrInvalidCiphertext {
		t.Fatalf("expected ErrInvalidCiphertext, got: %v", err)
	}
}
