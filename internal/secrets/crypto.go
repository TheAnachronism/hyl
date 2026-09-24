// Package secrets encrypts the provider tokens hyl stores on behalf of a user.
//
// The key is derived from HYL_SECRET_KEY with a fixed context string, so a
// secret rotation invalidates stored tokens instead of silently keeping them
// readable. AES-256-GCM with a random 12-byte nonce prefixed to the ciphertext
// makes each stored blob self-contained and tamper-evident.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
)

// keyContext keeps the token key independent of any other use of the secret.
const keyContext = "hyl-token-v1|"

// ErrInvalidCiphertext is returned when a stored blob cannot be decrypted.
var ErrInvalidCiphertext = errors.New("stored token is not decryptable with this key")

// Cipher encrypts and decrypts provider tokens.
type Cipher struct {
	aead cipher.AEAD
}

// New derives the token key from the application secret.
func New(secretKey string) (*Cipher, error) {
	if len(secretKey) < 32 {
		return nil, fmt.Errorf("secret key must be at least 32 bytes (got %d)", len(secretKey))
	}
	key := sha256.Sum256([]byte(keyContext + secretKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt seals a token, prefixing the nonce.
func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return c.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt opens a sealed token.
func (c *Cipher) Decrypt(ciphertext []byte) ([]byte, error) {
	size := c.aead.NonceSize()
	if len(ciphertext) < size+1 {
		return nil, ErrInvalidCiphertext
	}
	plaintext, err := c.aead.Open(nil, ciphertext[:size], ciphertext[size:], nil)
	if err != nil {
		return nil, ErrInvalidCiphertext
	}
	return plaintext, nil
}

// EncryptString is the convenience wrapper used by the connection handlers.
func (c *Cipher) EncryptString(plaintext string) ([]byte, error) {
	return c.Encrypt([]byte(plaintext))
}

// DecryptString is the convenience wrapper used by the worker.
func (c *Cipher) DecryptString(ciphertext []byte) (string, error) {
	if len(ciphertext) == 0 {
		return "", nil
	}
	plaintext, err := c.Decrypt(ciphertext)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
