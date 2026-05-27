package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
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

	// metaCache memoizes the decrypted *metadata* of an entry (its gopass
	// Keys()/values: labels, timestamps and searchable attributes) keyed by
	// store path. It deliberately never holds the secret payload (Password or
	// Body): SearchItems only needs attributes to match, and decryption
	// dominates its latency, so caching metadata turns a per-lookup O(N)
	// decryption of the whole collection into a single decryption per entry.
	// The actual secret value is always re-decrypted on demand in GetItem and
	// never retained. Entries are invalidated on every local mutation; there is
	// no TTL, so out-of-process changes require a daemon restart.
	cacheMu   sync.RWMutex
	metaCache map[string]map[string]string
}

// NewGopassStore creates a new GoPass-backed store
func NewGopassStore(ctx context.Context, prefix string) (*GopassStore, error) {
	store, err := api.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize gopass: %w", err)
	}
	return NewGopassStoreWithBackend(store, prefix), nil
}

// NewGopassStoreWithBackend builds a store over an arbitrary gopass.Store
// backend. It is the dependency-injection seam used by tests to substitute a
// fake backend; production code uses NewGopassStore.
func NewGopassStoreWithBackend(backend gopass.Store, prefix string) *GopassStore {
	return &GopassStore{
		store:     backend,
		mapper:    NewMapper(prefix),
		locked:    make(map[string]bool),
		metaCache: make(map[string]map[string]string),
	}
}

// metaFromSecret extracts a decrypted entry's metadata key/value pairs. By
// construction it copies only gopass Keys() — never Password() or Body() — so
// the result is safe to cache without retaining the secret value.
func metaFromSecret(sec gopass.Secret) map[string]string {
	keys := sec.Keys()
	m := make(map[string]string, len(keys))
	for _, k := range keys {
		if v, ok := sec.Get(k); ok {
			m[k] = v
		}
	}
	return m
}

// metaFor returns an entry's cached metadata, decrypting and caching it on the
// first access. The secret payload is discarded after extraction.
func (s *GopassStore) metaFor(ctx context.Context, path string) (map[string]string, error) {
	s.cacheMu.RLock()
	m, ok := s.metaCache[path]
	s.cacheMu.RUnlock()
	if ok {
		return m, nil
	}

	sec, err := s.store.Get(ctx, path, "latest")
	if err != nil {
		return nil, err
	}
	m = metaFromSecret(sec)

	s.cacheMu.Lock()
	s.metaCache[path] = m
	s.cacheMu.Unlock()
	return m, nil
}

// putMeta refreshes the cached metadata for a path (used when an entry has just
// been decrypted for another reason).
func (s *GopassStore) putMeta(path string, meta map[string]string) {
	s.cacheMu.Lock()
	s.metaCache[path] = meta
	s.cacheMu.Unlock()
}

// invalidateMeta drops the cached metadata for a single path.
func (s *GopassStore) invalidateMeta(path string) {
	s.cacheMu.Lock()
	delete(s.metaCache, path)
	s.cacheMu.Unlock()
}

// invalidateMetaPrefix drops cached metadata for a path and everything beneath
// it (used when a whole collection is removed).
func (s *GopassStore) invalidateMetaPrefix(prefix string) {
	s.cacheMu.Lock()
	for k := range s.metaCache {
		if k == prefix || strings.HasPrefix(k, prefix+"/") {
			delete(s.metaCache, k)
		}
	}
	s.cacheMu.Unlock()
}

// applyItemMeta fills an ItemData's metadata fields from a cached metadata map.
// It never touches ItemData.Secret.
func applyItemMeta(item *ItemData, meta map[string]string) {
	for key, val := range meta {
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

	meta, err := s.metaFor(ctx, metaPath)
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

	for key, val := range meta {
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
	if err := sec.Set(collLabelKey, label); err != nil {
		return fmt.Errorf("set label: %w", err)
	}
	if err := sec.Set(collCreatedKey, now); err != nil {
		return fmt.Errorf("set created: %w", err)
	}
	if err := sec.Set(collModifiedKey, now); err != nil {
		return fmt.Errorf("set modified: %w", err)
	}

	if err := s.store.Set(ctx, metaPath, sec); err != nil {
		return err
	}
	s.invalidateMeta(metaPath)
	return nil
}

// DeleteCollection deletes a collection and all its items
func (s *GopassStore) DeleteCollection(ctx context.Context, name string) error {
	collPath := s.mapper.CollectionPath(name)
	if err := s.store.RemoveAll(ctx, collPath); err != nil {
		return err
	}
	s.invalidateMetaPrefix(collPath)
	return nil
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
	if err := sec.Set(collLabelKey, label); err != nil {
		return fmt.Errorf("set label: %w", err)
	}
	if err := sec.Set(collCreatedKey, existing.Created.Format(time.RFC3339)); err != nil {
		return fmt.Errorf("set created: %w", err)
	}
	if err := sec.Set(collModifiedKey, now); err != nil {
		return fmt.Errorf("set modified: %w", err)
	}

	if err := s.store.Set(ctx, metaPath, sec); err != nil {
		return err
	}
	s.invalidateMeta(metaPath)
	return nil
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

	// Secret retrieval always decrypts fresh — the password is never cached.
	sec, err := s.store.Get(ctx, itemPath, "latest")
	if err != nil {
		return nil, fmt.Errorf("item not found: %s/%s", collection, id)
	}

	// Refresh the metadata cache opportunistically since we just decrypted.
	meta := metaFromSecret(sec)
	s.putMeta(itemPath, meta)

	item := &ItemData{
		ID:          id,
		Secret:      []byte(sec.Password()),
		ContentType: "text/plain",
		Attributes:  make(map[string]string),
	}
	applyItemMeta(item, meta)

	return item, nil
}

// CreateItem creates a new item in a collection
func (s *GopassStore) CreateItem(ctx context.Context, collection string, item *ItemData) (string, error) {
	// Generate a D-Bus-safe ID if not provided. The ID becomes an object-path
	// element, which forbids hyphens, so use the same "i"+hex encoding as the
	// keyring store and the service layer rather than a raw hyphenated UUID.
	if item.ID == "" {
		rawID := uuid.New()
		item.ID = fmt.Sprintf("i%x", rawID[:])
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
	for _, kv := range []struct{ k, v string }{
		{labelKey, item.Label},
		{createdKey, item.Created.Format(time.RFC3339)},
		{modifiedKey, item.Modified.Format(time.RFC3339)},
		{contentTypeKey, item.ContentType},
	} {
		if err := sec.Set(kv.k, kv.v); err != nil {
			return "", fmt.Errorf("set %s: %w", kv.k, err)
		}
	}

	// Add user attributes (sorted for consistency)
	keys := make([]string, 0, len(item.Attributes))
	for k := range item.Attributes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := sec.Set(k, item.Attributes[k]); err != nil {
			return "", fmt.Errorf("set attr %s: %w", k, err)
		}
	}

	itemPath := s.mapper.ItemPath(collection, item.ID)
	if err := s.store.Set(ctx, itemPath, sec); err != nil {
		return "", err
	}
	s.invalidateMeta(itemPath)

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
	for _, kv := range []struct{ k, v string }{
		{labelKey, item.Label},
		{createdKey, item.Created.Format(time.RFC3339)},
		{modifiedKey, item.Modified.Format(time.RFC3339)},
		{contentTypeKey, item.ContentType},
	} {
		if err := sec.Set(kv.k, kv.v); err != nil {
			return fmt.Errorf("set %s: %w", kv.k, err)
		}
	}

	// Add user attributes (sorted for consistency)
	keys := make([]string, 0, len(item.Attributes))
	for k := range item.Attributes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := sec.Set(k, item.Attributes[k]); err != nil {
			return fmt.Errorf("set attr %s: %w", k, err)
		}
	}

	itemPath := s.mapper.ItemPath(collection, id)
	if err := s.store.Set(ctx, itemPath, sec); err != nil {
		return err
	}
	s.invalidateMeta(itemPath)
	return nil
}

// DeleteItem deletes an item
func (s *GopassStore) DeleteItem(ctx context.Context, collection, id string) error {
	itemPath := s.mapper.ItemPath(collection, id)
	if err := s.store.Remove(ctx, itemPath); err != nil {
		return err
	}
	s.invalidateMeta(itemPath)
	return nil
}

// SearchItems searches for items matching the given attributes
func (s *GopassStore) SearchItems(ctx context.Context, collection string, attributes map[string]string) ([]*ItemData, error) {
	items, err := s.Items(ctx, collection)
	if err != nil {
		return nil, err
	}

	var results []*ItemData
	for _, id := range items {
		// Matching needs only attributes, so read cached metadata rather than
		// decrypting the secret. The returned ItemData carries no Secret; the
		// payload is decrypted lazily by GetItem when a secret is actually read.
		meta, err := s.metaFor(ctx, s.mapper.ItemPath(collection, id))
		if err != nil {
			continue
		}

		item := &ItemData{
			ID:          id,
			ContentType: "text/plain",
			Attributes:  make(map[string]string),
		}
		applyItemMeta(item, meta)

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
		if err := newSec.Set(k, v); err != nil {
			return fmt.Errorf("set alias %q: %w", k, err)
		}
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
