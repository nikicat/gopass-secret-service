package schema

import (
	"testing"
)

func TestValidate_AllRequiredPresent(t *testing.T) {
	st, ok := Get("api-key")
	if !ok {
		t.Fatal("api-key type not found")
	}
	err := Validate(st, map[string]string{"url": "https://example.com"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_MissingRequired(t *testing.T) {
	st, ok := Get("token")
	if !ok {
		t.Fatal("token type not found")
	}
	err := Validate(st, map[string]string{"username": "bob"})
	if err == nil {
		t.Fatal("expected error for missing required attribute 'url'")
	}
}

func TestValidate_ExtraAttrsOK(t *testing.T) {
	st, ok := Get("password")
	if !ok {
		t.Fatal("password type not found")
	}
	err := Validate(st, map[string]string{
		"url":      "https://example.com",
		"username": "alice",
		"extra":    "ignored",
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_GenericNoRequired(t *testing.T) {
	st, ok := Get("generic")
	if !ok {
		t.Fatal("generic type not found")
	}
	err := Validate(st, map[string]string{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGet_Unknown(t *testing.T) {
	_, ok := Get("nonexistent")
	if ok {
		t.Fatal("expected ok=false for unknown type")
	}
}

func TestTypeNames(t *testing.T) {
	names := TypeNames()
	if len(names) != len(Types) {
		t.Fatalf("expected %d names, got %d", len(Types), len(names))
	}
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Fatalf("names not sorted: %v", names)
		}
	}
}
