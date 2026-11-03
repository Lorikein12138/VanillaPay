package platform

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

var (
	ErrInvalidEncryptionKey = errors.New("invalid encryption key")
	ErrInvalidCiphertext    = errors.New("invalid ciphertext")
	ErrRandomSource         = errors.New("random source failed")
)

// Encrypt protects plaintext with AES-256-GCM. The returned byte slice is
// nonce || ciphertext || authentication tag.
func Encrypt(key, plaintext []byte, random io.Reader) ([]byte, error) {
	if len(key) != 32 {
		return nil, ErrInvalidEncryptionKey
	}
	if random == nil {
		random = rand.Reader
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrInvalidEncryptionKey
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrInvalidEncryptionKey
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(random, nonce); err != nil {
		return nil, ErrRandomSource
	}
	return aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt opens an AES-256-GCM value created by Encrypt.
func Decrypt(key, ciphertext []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, ErrInvalidEncryptionKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrInvalidEncryptionKey
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrInvalidEncryptionKey
	}
	if len(ciphertext) < aead.NonceSize()+aead.Overhead() {
		return nil, ErrInvalidCiphertext
	}
	nonce, sealed := ciphertext[:aead.NonceSize()], ciphertext[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, ErrInvalidCiphertext
	}
	return plaintext, nil
}

// RandomToken returns a URL-safe, unpadded token made from cryptographically
// random bytes.
func RandomToken(random io.Reader, size int) (string, error) {
	if size <= 0 {
		return "", ErrRandomSource
	}
	if random == nil {
		random = rand.Reader
	}
	value := make([]byte, size)
	if _, err := io.ReadFull(random, value); err != nil {
		return "", ErrRandomSource
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
