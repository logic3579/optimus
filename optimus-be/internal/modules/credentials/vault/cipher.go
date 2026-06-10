package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

// Cipher wraps an AES-256-GCM AEAD instance over the master key.
// Stored ciphertext layout: [nonce (12 bytes)][ciphertext][tag (16 bytes)].
type Cipher struct {
	aead      cipher.AEAD
	nonceSize int
}

// NewCipher returns a Cipher using AES-256-GCM. key must be exactly 32 bytes.
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != KeyLen {
		return nil, fmt.Errorf("vault: cipher key must be %d bytes, got %d", KeyLen, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("vault: NewCipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("vault: NewGCM: %w", err)
	}
	return &Cipher{aead: aead, nonceSize: aead.NonceSize()}, nil
}

// Seal encrypts plaintext. Returns nonce||ciphertext||tag as a single slice.
func (c *Cipher) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("vault: nonce: %w", err)
	}
	// aead.Seal appends to nonce, producing nonce||ciphertext||tag.
	return c.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// ErrInvalidCiphertext is returned by Open when the input is shorter than a
// single nonce.
var ErrInvalidCiphertext = errors.New("vault: ciphertext too short")

// Open decrypts the layout produced by Seal. Returns ErrInvalidCiphertext on
// length-too-short, and the underlying GCM error on auth failure or tamper.
func (c *Cipher) Open(data []byte) ([]byte, error) {
	if len(data) < c.nonceSize {
		return nil, ErrInvalidCiphertext
	}
	nonce, ct := data[:c.nonceSize], data[c.nonceSize:]
	return c.aead.Open(nil, nonce, ct, nil)
}
