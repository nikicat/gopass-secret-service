package store

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
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

// GopassStore implements Store using the gopass CLI
type GopassStore struct {
	mapper *Mapper
	locked map[string]bool // collection name -> locked state
}

// NewGopassStore creates a new GoPass-backed store
func NewGopassStore(prefix string) *GopassStore {
	return &GopassStore{
		mapper: NewMapper(prefix),
		locked: make(map[string]bool),
	}
}

// Collections returns all collection names
func (s *GopassStore) Collections() ([]string, error) {
	out, err := s.gopass("ls", "--flat", s.mapper.prefix)
	if err != nil {
		// If the prefix doesn't exist, return empty list
		if strings.Contains(err.Error(), "not found") || strings.Contains(string(out), "not found") {
			return []string{}, nil
		}
		return nil, err
	}

	collections := make(map[string]bool)
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		coll, _, err := s.mapper.ParsePath(line)
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
func (s *GopassStore) GetCollection(name string) (*CollectionData, error) {
	metaPath := s.mapper.CollectionMetaPath(name)

	out, err := s.gopass("show", "-n", metaPath)
	if err != nil {
		// Check if collection exists by looking for any items
		items, err := s.Items(name)
		if err != nil || len(items) == 0 {
			// Try listing to see if collection exists
			collPath := s.mapper.CollectionPath(name)
			out, listErr := s.gopass("ls", "--flat", collPath)
			if listErr != nil || len(out) == 0 {
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

	s.parseMetadata(string(out), func(key, value string) {
		switch key {
		case collLabelKey:
			data.Label = value
		case collCreatedKey:
			if ts, err := time.Parse(time.RFC3339, value); err == nil {
				data.Created = ts
			}
		case collModifiedKey:
			if ts, err := time.Parse(time.RFC3339, value); err == nil {
				data.Modified = ts
			}
		}
	})

	return data, nil
}

// CreateCollection creates a new collection
func (s *GopassStore) CreateCollection(name, label string) error {
	name = SanitizeName(name)
	metaPath := s.mapper.CollectionMetaPath(name)

	now := time.Now().Format(time.RFC3339)
	content := fmt.Sprintf("collection-metadata\n---\n%s: %s\n%s: %s\n%s: %s\n",
		collLabelKey, label,
		collCreatedKey, now,
		collModifiedKey, now,
	)

	return s.gopassInsert(metaPath, content)
}

// DeleteCollection deletes a collection and all its items
func (s *GopassStore) DeleteCollection(name string) error {
	collPath := s.mapper.CollectionPath(name)
	_, err := s.gopass("rm", "-rf", collPath)
	return err
}

// SetCollectionLabel updates a collection's label
func (s *GopassStore) SetCollectionLabel(name, label string) error {
	existing, err := s.GetCollection(name)
	if err != nil {
		return err
	}

	metaPath := s.mapper.CollectionMetaPath(name)
	now := time.Now().Format(time.RFC3339)
	content := fmt.Sprintf("collection-metadata\n---\n%s: %s\n%s: %s\n%s: %s\n",
		collLabelKey, label,
		collCreatedKey, existing.Created.Format(time.RFC3339),
		collModifiedKey, now,
	)

	return s.gopassInsert(metaPath, content)
}

// Items returns all item IDs in a collection
func (s *GopassStore) Items(collection string) ([]string, error) {
	collPath := s.mapper.CollectionPath(collection)
	out, err := s.gopass("ls", "--flat", collPath)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return []string{}, nil
		}
		return nil, err
	}

	var items []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		_, itemID, err := s.mapper.ParsePath(line)
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
func (s *GopassStore) GetItem(collection, id string) (*ItemData, error) {
	itemPath := s.mapper.ItemPath(collection, id)

	out, err := s.gopass("show", "-n", itemPath)
	if err != nil {
		return nil, fmt.Errorf("item not found: %s/%s", collection, id)
	}

	return s.parseItem(id, string(out))
}

// CreateItem creates a new item in a collection
func (s *GopassStore) CreateItem(collection string, item *ItemData) (string, error) {
	// Generate UUID if not provided
	if item.ID == "" {
		item.ID = uuid.New().String()
	}

	itemPath := s.mapper.ItemPath(collection, item.ID)

	// Ensure collection exists
	_, err := s.GetCollection(collection)
	if err != nil {
		// Create collection with default label
		if err := s.CreateCollection(collection, collection); err != nil {
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

	content := s.formatItem(item)
	if err := s.gopassInsert(itemPath, content); err != nil {
		return "", err
	}

	return item.ID, nil
}

// UpdateItem updates an existing item
func (s *GopassStore) UpdateItem(collection, id string, item *ItemData) error {
	existing, err := s.GetItem(collection, id)
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

	itemPath := s.mapper.ItemPath(collection, id)
	content := s.formatItem(item)
	return s.gopassInsert(itemPath, content)
}

// DeleteItem deletes an item
func (s *GopassStore) DeleteItem(collection, id string) error {
	itemPath := s.mapper.ItemPath(collection, id)
	_, err := s.gopass("rm", "-f", itemPath)
	return err
}

// SearchItems searches for items matching the given attributes
func (s *GopassStore) SearchItems(collection string, attributes map[string]string) ([]*ItemData, error) {
	items, err := s.Items(collection)
	if err != nil {
		return nil, err
	}

	var results []*ItemData
	for _, id := range items {
		item, err := s.GetItem(collection, id)
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
func (s *GopassStore) SearchAllItems(attributes map[string]string) (map[string][]*ItemData, error) {
	collections, err := s.Collections()
	if err != nil {
		return nil, err
	}

	results := make(map[string][]*ItemData)
	for _, coll := range collections {
		items, err := s.SearchItems(coll, attributes)
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
func (s *GopassStore) LockCollection(name string) error {
	s.locked[name] = true
	return nil
}

// UnlockCollection unlocks a collection
func (s *GopassStore) UnlockCollection(name string) error {
	s.locked[name] = false
	return nil
}

// GetAlias returns the collection name for an alias
func (s *GopassStore) GetAlias(alias string) (string, error) {
	aliasPath := s.mapper.AliasesPath()
	out, err := s.gopass("show", "-n", aliasPath)
	if err != nil {
		// Handle default alias specially
		if alias == "default" {
			return "default", nil
		}
		return "", fmt.Errorf("alias not found: %s", alias)
	}

	var result string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, alias+":") {
			result = strings.TrimSpace(strings.TrimPrefix(line, alias+":"))
			break
		}
	}

	if result == "" {
		if alias == "default" {
			return "default", nil
		}
		return "", fmt.Errorf("alias not found: %s", alias)
	}

	return result, nil
}

// SetAlias sets an alias for a collection
func (s *GopassStore) SetAlias(alias, collection string) error {
	aliasPath := s.mapper.AliasesPath()

	// Get existing aliases
	aliases := make(map[string]string)
	out, err := s.gopass("show", "-n", aliasPath)
	if err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(out))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || line == "---" || strings.HasPrefix(line, "_") {
				continue
			}
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				aliases[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
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
	var content strings.Builder
	content.WriteString("aliases\n---\n")
	for k, v := range aliases {
		content.WriteString(fmt.Sprintf("%s: %s\n", k, v))
	}

	return s.gopassInsert(aliasPath, content.String())
}

// Close closes the store
func (s *GopassStore) Close() error {
	return nil
}

// Helper methods

func (s *GopassStore) gopass(args ...string) ([]byte, error) {
	cmd := exec.Command("gopass", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("gopass %s failed: %w: %s", strings.Join(args, " "), err, string(out))
	}
	return out, nil
}

func (s *GopassStore) gopassInsert(path, content string) error {
	cmd := exec.Command("gopass", "insert", "-f", path)
	cmd.Stdin = strings.NewReader(content)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gopass insert failed: %w: %s", err, string(out))
	}
	return nil
}

func (s *GopassStore) parseMetadata(content string, handler func(key, value string)) {
	lines := strings.Split(content, "\n")
	inMeta := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "---" {
			inMeta = true
			continue
		}
		if !inMeta {
			continue
		}

		// Split on ": " to handle keys with colons (e.g., "xdg:schema")
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) == 2 {
			key := parts[0]
			value := parts[1]
			handler(key, value)
		}
	}
}

func (s *GopassStore) parseItem(id, content string) (*ItemData, error) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty item content")
	}

	item := &ItemData{
		ID:          id,
		Secret:      []byte(lines[0]),
		ContentType: "text/plain",
		Attributes:  make(map[string]string),
	}

	inMeta := false
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "---" {
			inMeta = true
			continue
		}
		if !inMeta {
			continue
		}

		// Split on ": " to handle keys with colons (e.g., "xdg:schema")
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := parts[1]

		switch key {
		case labelKey:
			item.Label = value
		case createdKey:
			if ts, err := time.Parse(time.RFC3339, value); err == nil {
				item.Created = ts
			}
		case modifiedKey:
			if ts, err := time.Parse(time.RFC3339, value); err == nil {
				item.Modified = ts
			}
		case contentTypeKey:
			item.ContentType = value
		default:
			// Regular attribute
			if !strings.HasPrefix(key, metaPrefix) {
				item.Attributes[key] = value
			}
		}
	}

	return item, nil
}

func (s *GopassStore) formatItem(item *ItemData) string {
	var content strings.Builder

	// First line is the secret
	content.Write(item.Secret)
	content.WriteString("\n---\n")

	// Metadata
	content.WriteString(fmt.Sprintf("%s: %s\n", labelKey, item.Label))
	content.WriteString(fmt.Sprintf("%s: %s\n", createdKey, item.Created.Format(time.RFC3339)))
	content.WriteString(fmt.Sprintf("%s: %s\n", modifiedKey, item.Modified.Format(time.RFC3339)))
	content.WriteString(fmt.Sprintf("%s: %s\n", contentTypeKey, item.ContentType))

	// User attributes
	keys := make([]string, 0, len(item.Attributes))
	for k := range item.Attributes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		content.WriteString(fmt.Sprintf("%s: %s\n", k, item.Attributes[k]))
	}

	return content.String()
}

func matchesAttributes(item *ItemData, attrs map[string]string) bool {
	for k, v := range attrs {
		if item.Attributes[k] != v {
			return false
		}
	}
	return true
}
