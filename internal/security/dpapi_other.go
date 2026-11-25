//go:build !windows

package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
	"os"
)

// nonWindowsSealKey derives a machine/user-local test key on non-Windows CI hosts.
func nonWindowsSealKey() []byte {
	host, _ := os.Hostname()
	sum := sha256.Sum256([]byte("cloud-manager-nonwin-fallback:" + host + ":" + os.Getenv("USER")))
	return sum[:]
}

// ProtectSecret seals bytes with AES-256-GCM on non-Windows environments.
func ProtectSecret(plain []byte) ([]byte, error) {
	if len(plain) == 0 {
		return []byte{}, nil
	}
	block, err := aes.NewCipher(nonWindowsSealKey())
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

// UnprotectSecret opens bytes sealed by ProtectSecret on non-Windows environments.
func UnprotectSecret(sealed []byte) ([]byte, error) {
	if len(sealed) == 0 {
		return []byte{}, nil
	}
	block, err := aes.NewCipher(nonWindowsSealKey())
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(sealed) < gcm.NonceSize() {
		return nil, errors.New("corrupt sealed payload")
	}
	return gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], nil)
}
