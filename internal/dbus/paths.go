package dbus

import (
	"fmt"
	"strings"

	"github.com/godbus/dbus/v5"
)

// CollectionPath returns the D-Bus object path for a collection
func CollectionPath(name string) dbus.ObjectPath {
	return dbus.ObjectPath(fmt.Sprintf("%s/%s", CollectionBasePath, name))
}

// ItemPath returns the D-Bus object path for an item
func ItemPath(collection, itemID string) dbus.ObjectPath {
	return dbus.ObjectPath(fmt.Sprintf("%s/%s/%s", CollectionBasePath, collection, itemID))
}

// SessionPath returns the D-Bus object path for a session
func SessionPath(id string) dbus.ObjectPath {
	return dbus.ObjectPath(fmt.Sprintf("%s/%s", SessionBasePath, id))
}

// PromptPath returns the D-Bus object path for a prompt
func PromptPath(id string) dbus.ObjectPath {
	return dbus.ObjectPath(fmt.Sprintf("%s/%s", PromptBasePath, id))
}

// ParseCollectionPath extracts the collection name from a D-Bus path
func ParseCollectionPath(path dbus.ObjectPath) (string, error) {
	prefix := CollectionBasePath + "/"
	if !strings.HasPrefix(string(path), prefix) {
		return "", fmt.Errorf("invalid collection path: %s", path)
	}
	rest := strings.TrimPrefix(string(path), prefix)
	// Collection name is the first segment
	parts := strings.SplitN(rest, "/", 2)
	return parts[0], nil
}

// ParseItemPath extracts the collection name and item ID from a D-Bus path
func ParseItemPath(path dbus.ObjectPath) (collection, itemID string, err error) {
	prefix := CollectionBasePath + "/"
	if !strings.HasPrefix(string(path), prefix) {
		return "", "", fmt.Errorf("invalid item path: %s", path)
	}
	rest := strings.TrimPrefix(string(path), prefix)
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid item path: %s", path)
	}
	return parts[0], parts[1], nil
}

// ParseSessionPath extracts the session ID from a D-Bus path
func ParseSessionPath(path dbus.ObjectPath) (string, error) {
	prefix := SessionBasePath + "/"
	if !strings.HasPrefix(string(path), prefix) {
		return "", fmt.Errorf("invalid session path: %s", path)
	}
	return strings.TrimPrefix(string(path), prefix), nil
}

// ParsePromptPath extracts the prompt ID from a D-Bus path
func ParsePromptPath(path dbus.ObjectPath) (string, error) {
	prefix := PromptBasePath + "/"
	if !strings.HasPrefix(string(path), prefix) {
		return "", fmt.Errorf("invalid prompt path: %s", path)
	}
	return strings.TrimPrefix(string(path), prefix), nil
}

// IsCollectionPath checks if the path is a valid collection path
func IsCollectionPath(path dbus.ObjectPath) bool {
	prefix := CollectionBasePath + "/"
	if !strings.HasPrefix(string(path), prefix) {
		return false
	}
	rest := strings.TrimPrefix(string(path), prefix)
	// Should be exactly one segment (no slashes)
	return !strings.Contains(rest, "/")
}

// IsItemPath checks if the path is a valid item path
func IsItemPath(path dbus.ObjectPath) bool {
	prefix := CollectionBasePath + "/"
	if !strings.HasPrefix(string(path), prefix) {
		return false
	}
	rest := strings.TrimPrefix(string(path), prefix)
	// Should be exactly two segments
	parts := strings.Split(rest, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != ""
}

// AliasPath returns the D-Bus object path for a collection alias
func AliasPath(alias string) dbus.ObjectPath {
	return dbus.ObjectPath(fmt.Sprintf("%s/%s", AliasBasePath, alias))
}

// ParseAliasPath extracts the alias name from a D-Bus path
func ParseAliasPath(path dbus.ObjectPath) (string, error) {
	prefix := AliasBasePath + "/"
	if !strings.HasPrefix(string(path), prefix) {
		return "", fmt.Errorf("invalid alias path: %s", path)
	}
	return strings.TrimPrefix(string(path), prefix), nil
}

// IsAliasPath checks if the path is a valid alias path
func IsAliasPath(path dbus.ObjectPath) bool {
	prefix := AliasBasePath + "/"
	return strings.HasPrefix(string(path), prefix)
}
