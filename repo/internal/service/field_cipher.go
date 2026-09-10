package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

var ErrInvalidCiphertext = errors.New("invalid encrypted payload")

type FieldCipher struct {
	aead cipher.AEAD
}

func NewFieldCipherFromBase64(masterKeyBase64 string) (*FieldCipher, error) {
	rawKey, err := base64.StdEncoding.DecodeString(masterKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("decode master key: %w", err)
	}
	if len(rawKey) != 32 {
		return nil, errors.New("master key must be 32 bytes for AES-256-GCM")
	}

	block, err := aes.NewCipher(rawKey)
	if err != nil {
		return nil, fmt.Errorf("create cipher block: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	return &FieldCipher{aead: aead}, nil
}

func (f *FieldCipher) Encrypt(plaintext []byte) (string, error) {
	nonce := make([]byte, f.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("read nonce: %w", err)
	}

	ciphertext := f.aead.Seal(nil, nonce, plaintext, nil)
	payload := append(nonce, ciphertext...)

	return base64.StdEncoding.EncodeToString(payload), nil
}

func (f *FieldCipher) Decrypt(payload string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, ErrInvalidCiphertext
	}

	nonceSize := f.aead.NonceSize()
	if len(raw) < nonceSize {
		return nil, ErrInvalidCiphertext
	}

	nonce := raw[:nonceSize]
	ciphertext := raw[nonceSize:]

	plaintext, err := f.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrInvalidCiphertext
	}

	return plaintext, nil
}
