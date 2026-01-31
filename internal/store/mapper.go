package store

import (
	"fmt"
	"path"
	"strings"
)

// Mapper handles path mapping between D-Bus and GoPass
type Mapper struct {
	prefix string
}

// NewMapper creates a new path mapper with the given prefix
func NewMapper(prefix string) *Mapper {
	return &Mapper{prefix: prefix}
}

// CollectionPath returns the GoPass path for a collection
func (m *Mapper) CollectionPath(name string) string {
	return path.Join(m.prefix, name)
}

// ItemPath returns the GoPass path for an item
func (m *Mapper) ItemPath(collection, id string) string {
	return path.Join(m.prefix, collection, id)
}

// AliasesPath returns the GoPass path for the aliases file
func (m *Mapper) AliasesPath() string {
	return path.Join(m.prefix, "_aliases")
}

// CollectionMetaPath returns the GoPass path for collection metadata
func (m *Mapper) CollectionMetaPath(name string) string {
	return path.Join(m.prefix, name, "_meta")
}

// ParsePath parses a GoPass path and returns the collection and item ID
func (m *Mapper) ParsePath(gopassPath string) (collection, itemID string, err error) {
	if !strings.HasPrefix(gopassPath, m.prefix+"/") {
		return "", "", fmt.Errorf("path does not start with prefix: %s", gopassPath)
	}

	rest := strings.TrimPrefix(gopassPath, m.prefix+"/")
	parts := strings.SplitN(rest, "/", 2)

	if len(parts) == 1 {
		return parts[0], "", nil
	}
	return parts[0], parts[1], nil
}

// IsCollectionMeta checks if a path is a collection metadata path
func (m *Mapper) IsCollectionMeta(gopassPath string) bool {
	return strings.HasSuffix(gopassPath, "/_meta")
}

// IsAliases checks if a path is the aliases file
func (m *Mapper) IsAliases(gopassPath string) bool {
	return gopassPath == m.AliasesPath()
}

// SanitizeName sanitizes a name for use in GoPass paths
func SanitizeName(name string) string {
	// Replace problematic characters with underscores
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, " ", "_")
	return name
}
