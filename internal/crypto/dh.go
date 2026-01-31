package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"

	"golang.org/x/crypto/hkdf"
)

// DH-IETF1024-SHA256-AES128-CBC-PKCS7 algorithm constants
const (
	AlgorithmDHAES = "dh-ietf1024-sha256-aes128-cbc-pkcs7"
)

// RFC 2409 MODP group 2 (1024-bit)
var (
	dhPrime = func() *big.Int {
		p, _ := new(big.Int).SetString(
			"FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD1"+
				"29024E088A67CC74020BBEA63B139B22514A08798E3404DD"+
				"EF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245"+
				"E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED"+
				"EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE65381"+
				"FFFFFFFFFFFFFFFF", 16)
		return p
	}()
	dhGenerator = big.NewInt(2)
)

// DHSession implements DH key exchange with AES-128-CBC encryption
type DHSession struct {
	privateKey *big.Int
	publicKey  *big.Int
	aesKey     []byte
}

// NewDHSession creates a new DH session
// clientPublic is the client's DH public key (big-endian bytes)
func NewDHSession(clientPublic []byte) (*DHSession, []byte, error) {
	// Generate server private key (random 1024 bits)
	privateKey, err := rand.Int(rand.Reader, dhPrime)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Calculate server public key: g^private mod p
	publicKey := new(big.Int).Exp(dhGenerator, privateKey, dhPrime)

	// Calculate shared secret: clientPublic^private mod p
	clientPub := new(big.Int).SetBytes(clientPublic)
	sharedSecret := new(big.Int).Exp(clientPub, privateKey, dhPrime)

	// Pad shared secret to 128 bytes (1024 bits) - spec requires leading zeros
	sharedBytes := sharedSecret.Bytes()
	paddedSecret := make([]byte, 128)
	copy(paddedSecret[128-len(sharedBytes):], sharedBytes)

	// Derive AES key using HKDF-SHA256 with NULL salt and empty info (per spec)
	hkdfReader := hkdf.New(sha256.New, paddedSecret, nil, nil)
	aesKey := make([]byte, 16)
	if _, err := hkdfReader.Read(aesKey); err != nil {
		return nil, nil, fmt.Errorf("HKDF failed: %w", err)
	}

	session := &DHSession{
		privateKey: privateKey,
		publicKey:  publicKey,
		aesKey:     aesKey,
	}

	// Return server's public key as output, padded to 128 bytes
	pubBytes := publicKey.Bytes()
	paddedPub := make([]byte, 128)
	copy(paddedPub[128-len(pubBytes):], pubBytes)

	return session, paddedPub, nil
}

// Algorithm returns the algorithm name
func (s *DHSession) Algorithm() string {
	return AlgorithmDHAES
}

// Encrypt encrypts plaintext using AES-128-CBC with PKCS7 padding
// Returns IV as parameters and ciphertext
func (s *DHSession) Encrypt(plaintext []byte) (parameters, ciphertext []byte, err error) {
	block, err := aes.NewCipher(s.aesKey)
	if err != nil {
		return nil, nil, err
	}

	// PKCS7 padding
	padLen := aes.BlockSize - (len(plaintext) % aes.BlockSize)
	padded := make([]byte, len(plaintext)+padLen)
	copy(padded, plaintext)
	for i := len(plaintext); i < len(padded); i++ {
		padded[i] = byte(padLen)
	}

	// Generate random IV
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return nil, nil, err
	}

	// Encrypt
	ciphertext = make([]byte, len(padded))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, padded)

	return iv, ciphertext, nil
}

// Decrypt decrypts ciphertext using AES-128-CBC with PKCS7 padding
// parameters contains the IV
func (s *DHSession) Decrypt(parameters, ciphertext []byte) (plaintext []byte, err error) {
	if len(parameters) != aes.BlockSize {
		return nil, fmt.Errorf("invalid IV length: %d", len(parameters))
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("invalid ciphertext length: %d", len(ciphertext))
	}

	block, err := aes.NewCipher(s.aesKey)
	if err != nil {
		return nil, err
	}

	// Decrypt
	decrypted := make([]byte, len(ciphertext))
	mode := cipher.NewCBCDecrypter(block, parameters)
	mode.CryptBlocks(decrypted, ciphertext)

	// Remove PKCS7 padding
	if len(decrypted) == 0 {
		return nil, fmt.Errorf("empty decrypted data")
	}
	padLen := int(decrypted[len(decrypted)-1])
	if padLen == 0 || padLen > aes.BlockSize || padLen > len(decrypted) {
		return nil, fmt.Errorf("invalid padding: padLen=%d", padLen)
	}
	// Verify padding
	for i := len(decrypted) - padLen; i < len(decrypted); i++ {
		if decrypted[i] != byte(padLen) {
			return nil, fmt.Errorf("invalid padding")
		}
	}

	return decrypted[:len(decrypted)-padLen], nil
}

// Close is a no-op for DH sessions
func (s *DHSession) Close() error {
	// Zero out the AES key
	for i := range s.aesKey {
		s.aesKey[i] = 0
	}
	return nil
}
