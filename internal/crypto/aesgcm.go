// Package crypto implementa AES-256-GCM para cifragem de segredos em repouso.
//
// Os tokens do Chatwoot e evolution-go são armazenados cifrados no Postgres
// (bytea) com nonce prefixado (12 bytes) + ciphertext + tag (16 bytes).
// A chave mestra vem de CONNECT_MASTER_KEY (32 bytes em base64).
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// KeySize em bytes (AES-256).
const KeySize = 32

// NonceSize em bytes (recomendação GCM).
const NonceSize = 12

// ErrKeySize é retornado quando a chave não tem 32 bytes.
var ErrKeySize = errors.New("crypto: chave deve ter 32 bytes (AES-256)")

// ErrCiphertextTooShort é retornado quando o ciphertext é menor que nonce+tag.
var ErrCiphertextTooShort = errors.New("crypto: ciphertext muito curto")

// Cipher encapsula uma chave AES-256-GCM.
type Cipher struct {
	gcm cipher.AEAD
}

// New cria um cipher a partir de uma chave base64 (32 bytes).
func New(masterKeyB64 string) (*Cipher, error) {
	key, err := base64.StdEncoding.DecodeString(masterKeyB64)
	if err != nil {
		return nil, fmt.Errorf("crypto: decode key: %w", err)
	}
	if len(key) != KeySize {
		return nil, ErrKeySize
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}
	return &Cipher{gcm: gcm}, nil
}

// Encrypt cifra o plaintext. Saída: nonce || ciphertext || tag.
func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto: read nonce: %w", err)
	}
	// Seal appends ciphertext+tag to nonce; result: nonce || ct || tag
	ct := c.gcm.Seal(nonce, nonce, plaintext, nil)
	return ct, nil
}

// Decrypt decifra um blob produzido por Encrypt.
func (c *Cipher) Decrypt(blob []byte) ([]byte, error) {
	if len(blob) < NonceSize+c.gcm.Overhead() {
		return nil, ErrCiphertextTooShort
	}
	nonce, ct := blob[:NonceSize], blob[NonceSize:]
	pt, err := c.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}
	return pt, nil
}

// EncryptString é um helper que cifra uma string UTF-8.
func (c *Cipher) EncryptString(s string) ([]byte, error) {
	return c.Encrypt([]byte(s))
}

// DecryptString é um helper que decifra para string UTF-8.
func (c *Cipher) DecryptString(blob []byte) (string, error) {
	pt, err := c.Decrypt(blob)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
