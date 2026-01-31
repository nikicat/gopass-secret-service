package crypto

import (
	"bytes"
	"testing"
)

func TestPlainSession(t *testing.T) {
	session, output, err := NewSession("plain", nil)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer session.Close()

	if len(output) != 0 {
		t.Errorf("Expected empty output, got %v", output)
	}

	if session.Algorithm() != "plain" {
		t.Errorf("Expected algorithm 'plain', got %s", session.Algorithm())
	}
}

func TestPlainEncryptDecrypt(t *testing.T) {
	session, _, err := NewSession("plain", nil)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer session.Close()

	plaintext := []byte("test secret value")

	params, ciphertext, err := session.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if len(params) != 0 {
		t.Errorf("Expected empty params, got %v", params)
	}

	if !bytes.Equal(ciphertext, plaintext) {
		t.Errorf("Expected ciphertext to equal plaintext for plain algorithm")
	}

	decrypted, err := session.Decrypt(params, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("Expected decrypted to equal plaintext")
	}
}

func TestUnsupportedAlgorithm(t *testing.T) {
	_, _, err := NewSession("unsupported", nil)
	if err == nil {
		t.Error("Expected error for unsupported algorithm")
	}
}

func TestSupportedAlgorithms(t *testing.T) {
	algorithms := SupportedAlgorithms()
	if len(algorithms) == 0 {
		t.Error("Expected at least one supported algorithm")
	}

	found := false
	for _, alg := range algorithms {
		if alg == "plain" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected 'plain' to be in supported algorithms")
	}
}
