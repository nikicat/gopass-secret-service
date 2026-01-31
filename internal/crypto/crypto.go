package crypto

import (
	"fmt"

	dbtypes "github.com/nblogist/gopass-secret-service/internal/dbus"
)

// Session represents a crypto session for encrypting/decrypting secrets
type Session interface {
	// Algorithm returns the algorithm name used by this session
	Algorithm() string

	// Encrypt encrypts a secret value, returning parameters and ciphertext
	Encrypt(plaintext []byte) (parameters, ciphertext []byte, err error)

	// Decrypt decrypts a secret value using parameters and ciphertext
	Decrypt(parameters, ciphertext []byte) (plaintext []byte, err error)

	// Close closes the session and releases any resources
	Close() error
}

// NewSession creates a new crypto session for the given algorithm
func NewSession(algorithm string, clientInput []byte) (Session, []byte, error) {
	switch algorithm {
	case dbtypes.AlgorithmPlain:
		return NewPlainSession()
	case AlgorithmDHAES:
		return NewDHSession(clientInput)
	default:
		return nil, nil, fmt.Errorf("unsupported algorithm: %s", algorithm)
	}
}

// SupportedAlgorithms returns the list of supported algorithm names
func SupportedAlgorithms() []string {
	return []string{dbtypes.AlgorithmPlain, AlgorithmDHAES}
}
