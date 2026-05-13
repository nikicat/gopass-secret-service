// Package store, keyring.go: in-memory session storage backed by the Linux
// kernel keyring.
//
// Items live in a per-daemon child keyring attached to the process keyring.
// When the daemon exits, the kernel reclaims the keyring; that matches the
// freedesktop session-collection contract of "non-persistent storage tied to
// the application session".
//
// Quota note: a non-root user has a default of 200 keys / 20 000 bytes per UID
// (see /proc/sys/kernel/keys/{maxkeys,maxbytes}). OIDC tokens are ~1–5 KB, so
// the budget covers a handful of session items per user. Exceeding the quota
// surfaces as EDQUOT on Create/Update; callers can fall back to the primary
// store.
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

// SessionCollectionName is the internal name of the volatile, in-memory
// collection exposed via the freedesktop "session" alias.
const SessionCollectionName = "session"

const (
	keyringKeyType         = "user"
	keyringRootDescription = "gopass-secret-service-session"
	// Kernel per-key payload limit. KEYCTL_READ on a buffer smaller than the
	// actual payload returns the real size without copying; we use this as
	// the upper bound so a single read suffices.
	keyringMaxPayload = 32 * 1024
)

// KeyringStore implements Store backed by a per-daemon child of the Linux
// process keyring. It serves a single collection (SessionCollectionName);
// callers that want a multi-collection facade should wrap it with MultiStore.
type KeyringStore struct {
	mu        sync.RWMutex
	ringID    int            // numerical ID of our child keyring
	items     map[string]int // item ID -> kernel key ID
	collLabel string
	collTime  time.Time
}

// NewKeyringStore creates a child keyring under the process keyring and
// returns a store that operates on it. The keyring is reclaimed when the
// daemon process exits.
func NewKeyringStore() (*KeyringStore, error) {
	parent, err := unix.KeyctlGetKeyringID(unix.KEY_SPEC_PROCESS_KEYRING, true)
	if err != nil {
		return nil, fmt.Errorf("resolve process keyring: %w", err)
	}
	ringID, err := unix.AddKey("keyring", keyringRootDescription, nil, parent)
	if err != nil {
		return nil, fmt.Errorf("create session keyring: %w", err)
	}
	now := time.Now()
	return &KeyringStore{
		ringID:    ringID,
		items:     make(map[string]int),
		collLabel: "Session",
		collTime:  now,
	}, nil
}

// itemPayload is the on-key encoding of an ItemData. ID is omitted because the
// kernel key's description carries it.
type itemPayload struct {
	Label       string            `json:"label,omitempty"`
	Secret      []byte            `json:"secret,omitempty"`
	ContentType string            `json:"content_type,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty"`
	Created     time.Time         `json:"created"`
	Modified    time.Time         `json:"modified"`
}

func encodeItem(item *ItemData) ([]byte, error) {
	return json.Marshal(&itemPayload{
		Label:       item.Label,
		Secret:      item.Secret,
		ContentType: item.ContentType,
		Attributes:  item.Attributes,
		Created:     item.Created,
		Modified:    item.Modified,
	})
}

func decodeItem(id string, payload []byte) (*ItemData, error) {
	var p itemPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("decode item %s: %w", id, err)
	}
	return &ItemData{
		ID:          id,
		Label:       p.Label,
		Secret:      p.Secret,
		ContentType: p.ContentType,
		Attributes:  p.Attributes,
		Created:     p.Created,
		Modified:    p.Modified,
	}, nil
}

// readKeyPayload returns the payload bytes of a kernel key. The kernel
// truncates if our buffer is too small but reports the true size; we use a
// buffer at the per-key cap so a single read suffices.
func readKeyPayload(keyID int) ([]byte, error) {
	buf := make([]byte, keyringMaxPayload)
	n, err := unix.KeyctlBuffer(unix.KEYCTL_READ, keyID, buf, len(buf))
	if err != nil {
		return nil, fmt.Errorf("read key %d: %w", keyID, err)
	}
	if n > len(buf) {
		return nil, fmt.Errorf("key %d payload (%d bytes) exceeds %d-byte cap", keyID, n, len(buf))
	}
	return buf[:n], nil
}

func (s *KeyringStore) checkColl(name string) error {
	if name != SessionCollectionName {
		return fmt.Errorf("keyring store: unknown collection %q", name)
	}
	return nil
}

func (s *KeyringStore) Collections(ctx context.Context) ([]string, error) {
	return []string{SessionCollectionName}, nil
}

func (s *KeyringStore) GetCollection(ctx context.Context, name string) (*CollectionData, error) {
	if err := s.checkColl(name); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &CollectionData{
		Name:     name,
		Label:    s.collLabel,
		Created:  s.collTime,
		Modified: s.collTime,
		Locked:   false,
	}, nil
}

// CreateCollection is a label-only operation here: the session keyring is
// created in NewKeyringStore and can't be re-created. Callers that pass
// SessionCollectionName at startup get their label honoured.
func (s *KeyringStore) CreateCollection(ctx context.Context, name, label string) error {
	if err := s.checkColl(name); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if label != "" {
		s.collLabel = label
	}
	return nil
}

func (s *KeyringStore) DeleteCollection(ctx context.Context, name string) error {
	return fmt.Errorf("session collection cannot be deleted")
}

func (s *KeyringStore) SetCollectionLabel(ctx context.Context, name, label string) error {
	if err := s.checkColl(name); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.collLabel = label
	s.collTime = time.Now()
	return nil
}

func (s *KeyringStore) Items(ctx context.Context, collection string) ([]string, error) {
	if err := s.checkColl(collection); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.items))
	for id := range s.items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

func (s *KeyringStore) GetItem(ctx context.Context, collection, id string) (*ItemData, error) {
	if err := s.checkColl(collection); err != nil {
		return nil, err
	}
	s.mu.RLock()
	keyID, ok := s.items[id]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("item not found: %s", id)
	}
	payload, err := readKeyPayload(keyID)
	if err != nil {
		return nil, err
	}
	return decodeItem(id, payload)
}

// CreateItem assigns an ID if absent and stores the item as a fresh kernel
// key. If the ID already maps to an existing key, AddKey updates the payload
// in place (kernel-level same-description coalescing).
func (s *KeyringStore) CreateItem(ctx context.Context, collection string, item *ItemData) (string, error) {
	if err := s.checkColl(collection); err != nil {
		return "", err
	}
	if item.ID == "" {
		rawID := uuid.New()
		item.ID = fmt.Sprintf("i%x", rawID[:])
	}
	now := time.Now()
	if item.Created.IsZero() {
		item.Created = now
	}
	item.Modified = now
	if item.ContentType == "" {
		item.ContentType = "text/plain"
	}

	payload, err := encodeItem(item)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	keyID, err := unix.AddKey(keyringKeyType, item.ID, payload, s.ringID)
	if err != nil {
		return "", fmt.Errorf("add key: %w", err)
	}
	s.items[item.ID] = keyID
	return item.ID, nil
}

// UpdateItem rewrites the payload of an existing item, preserving Created.
// AddKey with the same description+type+ringid coalesces onto the existing
// key, so the key ID is stable across updates.
func (s *KeyringStore) UpdateItem(ctx context.Context, collection, id string, item *ItemData) error {
	if err := s.checkColl(collection); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	existingID, ok := s.items[id]
	if !ok {
		return fmt.Errorf("item not found: %s", id)
	}
	// Preserve Created from the on-disk record.
	if existingPayload, err := readKeyPayload(existingID); err == nil {
		if prev, err := decodeItem(id, existingPayload); err == nil {
			item.Created = prev.Created
		}
	}
	item.ID = id
	item.Modified = time.Now()
	if item.ContentType == "" {
		item.ContentType = "text/plain"
	}
	payload, err := encodeItem(item)
	if err != nil {
		return err
	}
	keyID, err := unix.AddKey(keyringKeyType, id, payload, s.ringID)
	if err != nil {
		return fmt.Errorf("update key: %w", err)
	}
	s.items[id] = keyID
	return nil
}

func (s *KeyringStore) DeleteItem(ctx context.Context, collection, id string) error {
	if err := s.checkColl(collection); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	keyID, ok := s.items[id]
	if !ok {
		return fmt.Errorf("item not found: %s", id)
	}
	if _, err := unix.KeyctlInt(unix.KEYCTL_UNLINK, keyID, s.ringID, 0, 0); err != nil {
		return fmt.Errorf("unlink key %d: %w", keyID, err)
	}
	delete(s.items, id)
	return nil
}

func (s *KeyringStore) SearchItems(ctx context.Context, collection string, attributes map[string]string) ([]*ItemData, error) {
	if err := s.checkColl(collection); err != nil {
		return nil, err
	}
	ids, err := s.Items(ctx, collection)
	if err != nil {
		return nil, err
	}
	var matches []*ItemData
	for _, id := range ids {
		item, err := s.GetItem(ctx, collection, id)
		if err != nil {
			continue
		}
		if matchesAttributes(item, attributes) {
			matches = append(matches, item)
		}
	}
	return matches, nil
}

func (s *KeyringStore) SearchAllItems(ctx context.Context, attributes map[string]string) (map[string][]*ItemData, error) {
	matches, err := s.SearchItems(ctx, SessionCollectionName, attributes)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return map[string][]*ItemData{}, nil
	}
	return map[string][]*ItemData{SessionCollectionName: matches}, nil
}

// LockCollection / UnlockCollection are no-ops: the session keyring is always
// unlocked (the kernel handles isolation by process).
func (s *KeyringStore) LockCollection(ctx context.Context, name string) error {
	return s.checkColl(name)
}

func (s *KeyringStore) UnlockCollection(ctx context.Context, name string) error {
	return s.checkColl(name)
}

// GetAlias / SetAlias should be routed to the primary store by the dispatcher;
// the keyring store doesn't track aliases. Implemented for interface
// completeness only.
func (s *KeyringStore) GetAlias(ctx context.Context, alias string) (string, error) {
	return "", fmt.Errorf("keyring store has no aliases")
}

func (s *KeyringStore) SetAlias(ctx context.Context, alias, collection string) error {
	return fmt.Errorf("keyring store does not support SetAlias")
}

// Close clears the child keyring. The kernel would reclaim it when the process
// exits regardless; explicit clear lets unit tests run multiple stores cleanly
// in the same process.
func (s *KeyringStore) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ringID == 0 {
		return nil
	}
	if _, err := unix.KeyctlInt(unix.KEYCTL_CLEAR, s.ringID, 0, 0, 0); err != nil {
		return fmt.Errorf("clear session keyring: %w", err)
	}
	s.items = map[string]int{}
	return nil
}
