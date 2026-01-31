package crypto

import (
	dbtypes "github.com/nblogist/gopass-secret-service/internal/dbus"
)

// PlainSession implements the "plain" algorithm (no encryption)
type PlainSession struct{}

// NewPlainSession creates a new plain text session
func NewPlainSession() (*PlainSession, []byte, error) {
	// Plain algorithm returns empty output
	return &PlainSession{}, []byte{}, nil
}

// Algorithm returns "plain"
func (s *PlainSession) Algorithm() string {
	return dbtypes.AlgorithmPlain
}

// Encrypt returns the plaintext as-is (no encryption)
func (s *PlainSession) Encrypt(plaintext []byte) (parameters, ciphertext []byte, err error) {
	return []byte{}, plaintext, nil
}

// Decrypt returns the ciphertext as-is (no decryption)
func (s *PlainSession) Decrypt(parameters, ciphertext []byte) (plaintext []byte, err error) {
	return ciphertext, nil
}

// Close is a no-op for plain sessions
func (s *PlainSession) Close() error {
	return nil
}
