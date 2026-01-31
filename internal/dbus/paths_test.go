package dbus

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestCollectionPath(t *testing.T) {
	path := CollectionPath("default")
	expected := dbus.ObjectPath("/org/freedesktop/secrets/collection/default")
	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

func TestItemPath(t *testing.T) {
	path := ItemPath("default", "abc-123")
	expected := dbus.ObjectPath("/org/freedesktop/secrets/collection/default/abc-123")
	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

func TestSessionPath(t *testing.T) {
	path := SessionPath("session-123")
	expected := dbus.ObjectPath("/org/freedesktop/secrets/session/session-123")
	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

func TestPromptPath(t *testing.T) {
	path := PromptPath("prompt-456")
	expected := dbus.ObjectPath("/org/freedesktop/secrets/prompt/prompt-456")
	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

func TestParseCollectionPath(t *testing.T) {
	tests := []struct {
		path     dbus.ObjectPath
		expected string
		hasError bool
	}{
		{"/org/freedesktop/secrets/collection/default", "default", false},
		{"/org/freedesktop/secrets/collection/login", "login", false},
		{"/org/freedesktop/secrets/collection/my-collection", "my-collection", false},
		{"/org/freedesktop/secrets/session/123", "", true},
		{"/invalid/path", "", true},
	}

	for _, tc := range tests {
		t.Run(string(tc.path), func(t *testing.T) {
			result, err := ParseCollectionPath(tc.path)
			if tc.hasError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result != tc.expected {
					t.Errorf("Expected %s, got %s", tc.expected, result)
				}
			}
		})
	}
}

func TestParseItemPath(t *testing.T) {
	tests := []struct {
		path       dbus.ObjectPath
		collection string
		itemID     string
		hasError   bool
	}{
		{"/org/freedesktop/secrets/collection/default/abc-123", "default", "abc-123", false},
		{"/org/freedesktop/secrets/collection/login/item-456", "login", "item-456", false},
		{"/org/freedesktop/secrets/collection/default", "", "", true},
		{"/invalid/path", "", "", true},
	}

	for _, tc := range tests {
		t.Run(string(tc.path), func(t *testing.T) {
			coll, item, err := ParseItemPath(tc.path)
			if tc.hasError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if coll != tc.collection {
					t.Errorf("Expected collection %s, got %s", tc.collection, coll)
				}
				if item != tc.itemID {
					t.Errorf("Expected itemID %s, got %s", tc.itemID, item)
				}
			}
		})
	}
}

func TestParseSessionPath(t *testing.T) {
	tests := []struct {
		path     dbus.ObjectPath
		expected string
		hasError bool
	}{
		{"/org/freedesktop/secrets/session/abc-123", "abc-123", false},
		{"/org/freedesktop/secrets/session/my-session", "my-session", false},
		{"/org/freedesktop/secrets/collection/default", "", true},
		{"/invalid/path", "", true},
	}

	for _, tc := range tests {
		t.Run(string(tc.path), func(t *testing.T) {
			result, err := ParseSessionPath(tc.path)
			if tc.hasError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result != tc.expected {
					t.Errorf("Expected %s, got %s", tc.expected, result)
				}
			}
		})
	}
}

func TestParsePromptPath(t *testing.T) {
	tests := []struct {
		path     dbus.ObjectPath
		expected string
		hasError bool
	}{
		{"/org/freedesktop/secrets/prompt/abc-123", "abc-123", false},
		{"/org/freedesktop/secrets/prompt/my-prompt", "my-prompt", false},
		{"/org/freedesktop/secrets/collection/default", "", true},
		{"/invalid/path", "", true},
	}

	for _, tc := range tests {
		t.Run(string(tc.path), func(t *testing.T) {
			result, err := ParsePromptPath(tc.path)
			if tc.hasError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result != tc.expected {
					t.Errorf("Expected %s, got %s", tc.expected, result)
				}
			}
		})
	}
}

func TestIsCollectionPath(t *testing.T) {
	tests := []struct {
		path     dbus.ObjectPath
		expected bool
	}{
		{"/org/freedesktop/secrets/collection/default", true},
		{"/org/freedesktop/secrets/collection/login", true},
		{"/org/freedesktop/secrets/collection/default/item", false},
		{"/org/freedesktop/secrets/session/123", false},
		{"/org/freedesktop/secrets", false},
	}

	for _, tc := range tests {
		t.Run(string(tc.path), func(t *testing.T) {
			result := IsCollectionPath(tc.path)
			if result != tc.expected {
				t.Errorf("IsCollectionPath(%s) = %v, expected %v", tc.path, result, tc.expected)
			}
		})
	}
}

func TestIsItemPath(t *testing.T) {
	tests := []struct {
		path     dbus.ObjectPath
		expected bool
	}{
		{"/org/freedesktop/secrets/collection/default/item", true},
		{"/org/freedesktop/secrets/collection/login/abc-123", true},
		{"/org/freedesktop/secrets/collection/default", false},
		{"/org/freedesktop/secrets/session/123", false},
		{"/org/freedesktop/secrets", false},
	}

	for _, tc := range tests {
		t.Run(string(tc.path), func(t *testing.T) {
			result := IsItemPath(tc.path)
			if result != tc.expected {
				t.Errorf("IsItemPath(%s) = %v, expected %v", tc.path, result, tc.expected)
			}
		})
	}
}
