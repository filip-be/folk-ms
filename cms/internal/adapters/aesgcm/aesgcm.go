// Package aesgcm implements ports.Cipher with AES-256-GCM.
package aesgcm

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

// Cipher encrypts with a 32-byte key. Ciphertexts are nonce || sealed.
type Cipher struct{ aead cipher.AEAD }

// New builds a Cipher from a base64-encoded 32-byte key.
func New(keyB64 string) (*Cipher, error) {
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("aesgcm: key is not base64: %w", err)
	}
	if len(key) != 32 {
		return nil, errors.New("aesgcm: key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt seals plaintext bound to aad.
func (c *Cipher) Encrypt(plaintext, aad []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return c.aead.Seal(nonce, nonce, plaintext, aad), nil
}

// Decrypt opens a ciphertext made by Encrypt with the same aad.
func (c *Cipher) Decrypt(ciphertext, aad []byte) ([]byte, error) {
	n := c.aead.NonceSize()
	if len(ciphertext) < n {
		return nil, errors.New("aesgcm: ciphertext too short")
	}
	return c.aead.Open(nil, ciphertext[:n], ciphertext[n:], aad)
}
