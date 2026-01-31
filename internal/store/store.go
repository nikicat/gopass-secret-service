package store

import (
	"time"
)

// ItemData represents a secret item's data
type ItemData struct {
	// ID is the unique identifier for the item (UUID)
	ID string

	// Label is the human-readable label
	Label string

	// Secret is the actual secret value
	Secret []byte

	// ContentType is the MIME type of the secret
	ContentType string

	// Attributes are the searchable attributes
	Attributes map[string]string

	// Created is the creation timestamp
	Created time.Time

	// Modified is the last modification timestamp
	Modified time.Time

	// Locked indicates if the item is locked
	Locked bool
}

// CollectionData represents a collection's data
type CollectionData struct {
	// Name is the collection name (used in paths)
	Name string

	// Label is the human-readable label
	Label string

	// Created is the creation timestamp
	Created time.Time

	// Modified is the last modification timestamp
	Modified time.Time

	// Locked indicates if the collection is locked
	Locked bool
}

// Store is the interface for secret storage backends
type Store interface {
	// Collections returns all collection names
	Collections() ([]string, error)

	// GetCollection returns collection data by name
	GetCollection(name string) (*CollectionData, error)

	// CreateCollection creates a new collection
	CreateCollection(name, label string) error

	// DeleteCollection deletes a collection and all its items
	DeleteCollection(name string) error

	// SetCollectionLabel updates a collection's label
	SetCollectionLabel(name, label string) error

	// Items returns all item IDs in a collection
	Items(collection string) ([]string, error)

	// GetItem returns an item by collection and ID
	GetItem(collection, id string) (*ItemData, error)

	// CreateItem creates a new item in a collection
	CreateItem(collection string, item *ItemData) (string, error)

	// UpdateItem updates an existing item
	UpdateItem(collection, id string, item *ItemData) error

	// DeleteItem deletes an item
	DeleteItem(collection, id string) error

	// SearchItems searches for items matching the given attributes
	SearchItems(collection string, attributes map[string]string) ([]*ItemData, error)

	// SearchAllItems searches across all collections
	SearchAllItems(attributes map[string]string) (map[string][]*ItemData, error)

	// Lock locks a collection
	LockCollection(name string) error

	// Unlock unlocks a collection
	UnlockCollection(name string) error

	// GetAlias returns the collection name for an alias
	GetAlias(alias string) (string, error)

	// SetAlias sets an alias for a collection
	SetAlias(alias, collection string) error

	// Close closes the store
	Close() error
}
