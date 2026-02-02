package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gopasspw/gopass/pkg/gopass"
	"github.com/gopasspw/gopass/pkg/gopass/api"
	"github.com/gopasspw/gopass/pkg/gopass/secrets"
)

const (
	metaPrefix      = "_ss_"
	labelKey        = "_ss_label"
	createdKey      = "_ss_created"
	modifiedKey     = "_ss_modified"
	contentTypeKey  = "_ss_content_type"
	collLabelKey    = "_ss_coll_label"
	collCreatedKey  = "_ss_coll_created"
	collModifiedKey = "_ss_coll_modified"
)

// GopassStore implements Store using the gopass Go API
type GopassStore struct {
	store  gopass.Store
	mapper *Mapper
	locked map[string]bool // collection name -> locked state
}

// NewGopassStore creates a new GoPass-backed store
func NewGopassStore(ctx context.Context, prefix string) (*GopassStore, error) {
	store, err := api.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize gopass: %w", err)
	}
	return &GopassStore{
		store:  store,
		mapper: NewMapper(prefix),
		locked: make(map[string]bool),
	}, nil
}

// Collections returns all collection names
func (s *GopassStore) Collections(ctx context.Context) ([]string, error) {
	allPaths, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}

	collections := make(map[string]bool)
	for _, p := range allPaths {
		// Only include paths under our prefix
		if !strings.HasPrefix(p, s.mapper.prefix+"/") {
			continue
		}

		coll, _, err := s.mapper.ParsePath(p)
		if err != nil {
			continue
		}

		// Skip special entries
		if coll == "_aliases" || strings.HasPrefix(coll, "_") {
			continue
		}

		collections[coll] = true
	}

	result := make([]string, 0, len(collections))
	for coll := range collections {
		result = append(result, coll)
	}
	sort.Strings(result)
	return result, nil
}

// GetCollection returns collection data by name
func (s *GopassStore) GetCollection(ctx context.Context, name string) (*CollectionData, error) {
	metaPath := s.mapper.CollectionMetaPath(name)

	sec, err := s.store.Get(ctx, metaPath, "latest")
	if err != nil {
		// Check if collection exists by looking for any items
		items, err := s.Items(ctx, name)
		if err != nil || len(items) == 0 {
			// Try listing to see if collection exists
			allPaths, listErr := s.store.List(ctx)
			if listErr != nil {
				return nil, fmt.Errorf("collection not found: %s", name)
			}
			collPath := s.mapper.CollectionPath(name)
			found := false
			for _, p := range allPaths {
				if strings.HasPrefix(p, collPath+"/") {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("collection not found: %s", name)
			}
		}
		// Collection exists but no metadata, return defaults
		return &CollectionData{
			Name:    name,
			Label:   name,
			Created: time.Now(),
			Locked:  s.locked[name],
		}, nil
	}

	data := &CollectionData{
		Name:   name,
		Label:  name,
		Locked: s.locked[name],
	}

	for _, key := range sec.Keys() {
		val, ok := sec.Get(key)
		if !ok {
			continue
		}
		switch key {
		case collLabelKey:
			data.Label = val
		case collCreatedKey:
			if ts, err := time.Parse(time.RFC3339, val); err == nil {
				data.Created = ts
			}
		case collModifiedKey:
			if ts, err := time.Parse(time.RFC3339, val); err == nil {
				data.Modified = ts
			}
		}
	}

	return data, nil
}

// CreateCollection creates a new collection
func (s *GopassStore) CreateCollection(ctx context.Context, name, label string) error {
	name = SanitizeName(name)
	metaPath := s.mapper.CollectionMetaPath(name)

	now := time.Now().Format(time.RFC3339)

	sec := secrets.New()
	sec.SetPassword("collection-metadata")
	sec.Set(collLabelKey, label)
	sec.Set(collCreatedKey, now)
	sec.Set(collModifiedKey, now)

	return s.store.Set(ctx, metaPath, sec)
}

// DeleteCollection deletes a collection and all its items
func (s *GopassStore) DeleteCollection(ctx context.Context, name string) error {
	collPath := s.mapper.CollectionPath(name)
	return s.store.RemoveAll(ctx, collPath)
}

// SetCollectionLabel updates a collection's label
func (s *GopassStore) SetCollectionLabel(ctx context.Context, name, label string) error {
	existing, err := s.GetCollection(ctx, name)
	if err != nil {
		return err
	}

	metaPath := s.mapper.CollectionMetaPath(name)
	now := time.Now().Format(time.RFC3339)

	sec := secrets.New()
	sec.SetPassword("collection-metadata")
	sec.Set(collLabelKey, label)
	sec.Set(collCreatedKey, existing.Created.Format(time.RFC3339))
	sec.Set(collModifiedKey, now)

	return s.store.Set(ctx, metaPath, sec)
}

// Items returns all item IDs in a collection
func (s *GopassStore) Items(ctx context.Context, collection string) ([]string, error) {
	allPaths, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}

	collPath := s.mapper.CollectionPath(collection)
	var items []string

	for _, p := range allPaths {
		if !strings.HasPrefix(p, collPath+"/") {
			continue
		}

		_, itemID, err := s.mapper.ParsePath(p)
		if err != nil || itemID == "" {
			continue
		}

		// Skip metadata entries
		if strings.HasPrefix(itemID, "_") {
			continue
		}

		items = append(items, itemID)
	}

	return items, nil
}

// GetItem returns an item by collection and ID
func (s *GopassStore) GetItem(ctx context.Context, collection, id string) (*ItemData, error) {
	itemPath := s.mapper.ItemPath(collection, id)

	sec, err := s.store.Get(ctx, itemPath, "latest")
	if err != nil {
		return nil, fmt.Errorf("item not found: %s/%s", collection, id)
	}

	item := &ItemData{
		ID:          id,
		Secret:      []byte(sec.Password()),
		ContentType: "text/plain",
		Attributes:  make(map[string]string),
	}

	for _, key := range sec.Keys() {
		val, ok := sec.Get(key)
		if !ok {
			continue
		}

		switch key {
		case labelKey:
			item.Label = val
		case createdKey:
			if ts, err := time.Parse(time.RFC3339, val); err == nil {
				item.Created = ts
			}
		case modifiedKey:
			if ts, err := time.Parse(time.RFC3339, val); err == nil {
				item.Modified = ts
			}
		case contentTypeKey:
			item.ContentType = val
		default:
			// Regular attribute (skip internal metadata)
			if !strings.HasPrefix(key, metaPrefix) {
				item.Attributes[key] = val
			}
		}
	}

	return item, nil
}

// CreateItem creates a new item in a collection
func (s *GopassStore) CreateItem(ctx context.Context, collection string, item *ItemData) (string, error) {
	// Generate UUID if not provided
	if item.ID == "" {
		item.ID = uuid.New().String()
	}

	// Ensure collection exists
	_, err := s.GetCollection(ctx, collection)
	if err != nil {
		// Create collection with default label
		if err := s.CreateCollection(ctx, collection, collection); err != nil {
			return "", fmt.Errorf("failed to create collection: %w", err)
		}
	}

	now := time.Now()
	if item.Created.IsZero() {
		item.Created = now
	}
	item.Modified = now

	if item.ContentType == "" {
		item.ContentType = "text/plain"
	}

	sec := secrets.New()
	sec.SetPassword(string(item.Secret))
	sec.Set(labelKey, item.Label)
	sec.Set(createdKey, item.Created.Format(time.RFC3339))
	sec.Set(modifiedKey, item.Modified.Format(time.RFC3339))
	sec.Set(contentTypeKey, item.ContentType)

	// Add user attributes (sorted for consistency)
	keys := make([]string, 0, len(item.Attributes))
	for k := range item.Attributes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sec.Set(k, item.Attributes[k])
	}

	itemPath := s.mapper.ItemPath(collection, item.ID)
	if err := s.store.Set(ctx, itemPath, sec); err != nil {
		return "", err
	}

	return item.ID, nil
}

// UpdateItem updates an existing item
func (s *GopassStore) UpdateItem(ctx context.Context, collection, id string, item *ItemData) error {
	existing, err := s.GetItem(ctx, collection, id)
	if err != nil {
		return err
	}

	// Preserve creation time
	item.ID = id
	item.Created = existing.Created
	item.Modified = time.Now()

	if item.ContentType == "" {
		item.ContentType = existing.ContentType
	}

	sec := secrets.New()
	sec.SetPassword(string(item.Secret))
	sec.Set(labelKey, item.Label)
	sec.Set(createdKey, item.Created.Format(time.RFC3339))
	sec.Set(modifiedKey, item.Modified.Format(time.RFC3339))
	sec.Set(contentTypeKey, item.ContentType)

	// Add user attributes (sorted for consistency)
	keys := make([]string, 0, len(item.Attributes))
	for k := range item.Attributes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sec.Set(k, item.Attributes[k])
	}

	itemPath := s.mapper.ItemPath(collection, id)
	return s.store.Set(ctx, itemPath, sec)
}

// DeleteItem deletes an item
func (s *GopassStore) DeleteItem(ctx context.Context, collection, id string) error {
	itemPath := s.mapper.ItemPath(collection, id)
	return s.store.Remove(ctx, itemPath)
}

// SearchItems searches for items matching the given attributes
func (s *GopassStore) SearchItems(ctx context.Context, collection string, attributes map[string]string) ([]*ItemData, error) {
	items, err := s.Items(ctx, collection)
	if err != nil {
		return nil, err
	}

	var results []*ItemData
	for _, id := range items {
		item, err := s.GetItem(ctx, collection, id)
		if err != nil {
			continue
		}

		if matchesAttributes(item, attributes) {
			results = append(results, item)
		}
	}

	return results, nil
}

// SearchAllItems searches across all collections
func (s *GopassStore) SearchAllItems(ctx context.Context, attributes map[string]string) (map[string][]*ItemData, error) {
	collections, err := s.Collections(ctx)
	if err != nil {
		return nil, err
	}

	results := make(map[string][]*ItemData)
	for _, coll := range collections {
		items, err := s.SearchItems(ctx, coll, attributes)
		if err != nil {
			continue
		}
		if len(items) > 0 {
			results[coll] = items
		}
	}

	return results, nil
}

// LockCollection locks a collection
func (s *GopassStore) LockCollection(ctx context.Context, name string) error {
	s.locked[name] = true
	return nil
}

// UnlockCollection unlocks a collection
func (s *GopassStore) UnlockCollection(ctx context.Context, name string) error {
	s.locked[name] = false
	return nil
}

// GetAlias returns the collection name for an alias
func (s *GopassStore) GetAlias(ctx context.Context, alias string) (string, error) {
	aliasPath := s.mapper.AliasesPath()
	sec, err := s.store.Get(ctx, aliasPath, "latest")
	if err != nil {
		// Handle default alias specially
		if alias == "default" {
			return "default", nil
		}
		return "", fmt.Errorf("alias not found: %s", alias)
	}

	result, ok := sec.Get(alias)
	if !ok || result == "" {
		if alias == "default" {
			return "default", nil
		}
		return "", fmt.Errorf("alias not found: %s", alias)
	}

	return result, nil
}

// SetAlias sets an alias for a collection
func (s *GopassStore) SetAlias(ctx context.Context, alias, collection string) error {
	aliasPath := s.mapper.AliasesPath()

	// Get existing aliases
	aliases := make(map[string]string)
	sec, err := s.store.Get(ctx, aliasPath, "latest")
	if err == nil {
		for _, key := range sec.Keys() {
			if val, ok := sec.Get(key); ok {
				aliases[key] = val
			}
		}
	}

	// Update alias
	if collection == "" {
		delete(aliases, alias)
	} else {
		aliases[alias] = collection
	}

	// Write back
	newSec := secrets.New()
	newSec.SetPassword("aliases")
	for k, v := range aliases {
		newSec.Set(k, v)
	}

	return s.store.Set(ctx, aliasPath, newSec)
}

// Close closes the store
func (s *GopassStore) Close(ctx context.Context) error {
	return s.store.Close(ctx)
}

func matchesAttributes(item *ItemData, attrs map[string]string) bool {
	for k, v := range attrs {
		if item.Attributes[k] != v {
			return false
		}
	}
	return true
}
