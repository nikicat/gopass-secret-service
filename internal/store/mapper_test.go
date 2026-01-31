package store

import (
	"testing"
)

func TestMapper(t *testing.T) {
	m := NewMapper("secret-service")

	t.Run("CollectionPath", func(t *testing.T) {
		path := m.CollectionPath("default")
		expected := "secret-service/default"
		if path != expected {
			t.Errorf("Expected %s, got %s", expected, path)
		}
	})

	t.Run("ItemPath", func(t *testing.T) {
		path := m.ItemPath("default", "123-456")
		expected := "secret-service/default/123-456"
		if path != expected {
			t.Errorf("Expected %s, got %s", expected, path)
		}
	})

	t.Run("AliasesPath", func(t *testing.T) {
		path := m.AliasesPath()
		expected := "secret-service/_aliases"
		if path != expected {
			t.Errorf("Expected %s, got %s", expected, path)
		}
	})

	t.Run("CollectionMetaPath", func(t *testing.T) {
		path := m.CollectionMetaPath("login")
		expected := "secret-service/login/_meta"
		if path != expected {
			t.Errorf("Expected %s, got %s", expected, path)
		}
	})

	t.Run("ParsePath collection only", func(t *testing.T) {
		coll, item, err := m.ParsePath("secret-service/default")
		if err != nil {
			t.Fatalf("ParsePath failed: %v", err)
		}
		if coll != "default" {
			t.Errorf("Expected collection 'default', got %s", coll)
		}
		if item != "" {
			t.Errorf("Expected empty item, got %s", item)
		}
	})

	t.Run("ParsePath with item", func(t *testing.T) {
		coll, item, err := m.ParsePath("secret-service/default/abc-123")
		if err != nil {
			t.Fatalf("ParsePath failed: %v", err)
		}
		if coll != "default" {
			t.Errorf("Expected collection 'default', got %s", coll)
		}
		if item != "abc-123" {
			t.Errorf("Expected item 'abc-123', got %s", item)
		}
	})

	t.Run("ParsePath invalid prefix", func(t *testing.T) {
		_, _, err := m.ParsePath("other/default")
		if err == nil {
			t.Error("Expected error for invalid prefix")
		}
	})

	t.Run("IsCollectionMeta", func(t *testing.T) {
		if !m.IsCollectionMeta("secret-service/default/_meta") {
			t.Error("Expected IsCollectionMeta to return true")
		}
		if m.IsCollectionMeta("secret-service/default/item") {
			t.Error("Expected IsCollectionMeta to return false")
		}
	})

	t.Run("IsAliases", func(t *testing.T) {
		if !m.IsAliases("secret-service/_aliases") {
			t.Error("Expected IsAliases to return true")
		}
		if m.IsAliases("secret-service/default") {
			t.Error("Expected IsAliases to return false")
		}
	})
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with space", "with_space"},
		{"with/slash", "with_slash"},
		{"with\\backslash", "with_backslash"},
		{"multiple spaces here", "multiple_spaces_here"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := SanitizeName(tc.input)
			if result != tc.expected {
				t.Errorf("SanitizeName(%q) = %q, expected %q", tc.input, result, tc.expected)
			}
		})
	}
}
