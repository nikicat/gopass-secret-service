// Package store, keyring.go: in-memory session storage backed by the Linux
// kernel keyring.
//
// All kernel keyring syscalls run on a single dedicated OS thread, pinned via
// runtime.LockOSThread. The reason is non-obvious enough to spell out: in
// Linux, the references to the process/session/thread keyrings live in the
// per-task `struct cred`. Resolving KEY_SPEC_PROCESS_KEYRING with create=true
// calls install_process_keyring → prepare_creds → commit_creds(new), which
// only updates the *calling task's* cred. Other threads in the same TGID keep
// pointing at the old cred, so for them cred->process_keyring is NULL and the
// child keyring we just created isn't reachable via possession from their
// subscribed keyrings. Add_key syscalls from those threads then fail with
// EACCES even though the daemon owns the child keyring — they see only
// USR_VIEW (no USR_WRITE) on it. Pinning all syscalls to one thread sidesteps
// the per-cred divergence.
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
	"errors"
	"fmt"
	"runtime"
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
//
// All state mutations happen on a single dedicated worker goroutine — see the
// package comment for why. Methods marshal their work onto that worker via
// the requests channel and wait for the response.
type KeyringStore struct {
	ringID    int
	items     map[string]int // managed only by the worker goroutine
	collLabel string
	collTime  time.Time

	requests   chan keyringJob
	closed     chan struct{} // signals worker to exit
	workerDone chan struct{} // worker closes this just before returning
	closeOnce  sync.Once
}

// keyringJob is a unit of work for the worker goroutine. fn runs on the
// pinned thread; its return value travels back via resp.
type keyringJob struct {
	fn   func() (any, error)
	resp chan keyringResult
}

type keyringResult struct {
	value any
	err   error
}

// NewKeyringStore starts the worker goroutine, has it install the daemon's
// process keyring and create the child keyring under it, and returns the
// store. If any of that fails, the worker exits and the error propagates.
func NewKeyringStore() (*KeyringStore, error) {
	s := &KeyringStore{
		items:      make(map[string]int),
		requests:   make(chan keyringJob),
		closed:     make(chan struct{}),
		workerDone: make(chan struct{}),
	}
	initDone := make(chan error, 1)
	go s.worker(initDone)
	if err := <-initDone; err != nil {
		// Worker already exited cleanly on init failure.
		return nil, err
	}
	return s, nil
}

func (s *KeyringStore) worker(initDone chan<- error) {
	runtime.LockOSThread()
	// Note: we deliberately don't UnlockOSThread on exit. The Go runtime
	// destroys the thread on goroutine exit if it was locked, which is what
	// we want — the thread, its cred, and (effectively) the process keyring
	// child should die with the store.
	defer close(s.workerDone)

	parent, err := unix.KeyctlGetKeyringID(unix.KEY_SPEC_PROCESS_KEYRING, true)
	if err != nil {
		initDone <- fmt.Errorf("resolve process keyring: %w", err)
		return
	}
	ringID, err := unix.AddKey("keyring", keyringRootDescription, nil, parent)
	if err != nil {
		initDone <- fmt.Errorf("create session keyring: %w", err)
		return
	}
	s.ringID = ringID
	s.collLabel = "Session"
	s.collTime = time.Now()
	close(initDone)

	for {
		select {
		case <-s.closed:
			// Best-effort: clear the child keyring so its contents are
			// reclaimed promptly even if the daemon keeps running.
			if s.ringID != 0 {
				_, _ = unix.KeyctlInt(unix.KEYCTL_CLEAR, s.ringID, 0, 0, 0)
			}
			s.items = map[string]int{}
			return
		case job := <-s.requests:
			value, err := job.fn()
			job.resp <- keyringResult{value: value, err: err}
		}
	}
}

// do submits fn to the worker and waits for its result.
func (s *KeyringStore) do(fn func() (any, error)) (any, error) {
	select {
	case <-s.closed:
		return nil, errors.New("keyring store is closed")
	default:
	}
	resp := make(chan keyringResult, 1)
	select {
	case s.requests <- keyringJob{fn: fn, resp: resp}:
	case <-s.closed:
		return nil, errors.New("keyring store is closed")
	}
	r := <-resp
	return r.value, r.err
}

// --- payload encoding (id is the kernel key's description, not in payload) ---

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

// readKeyPayload runs on the worker thread. Caller must be on the worker.
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
	v, err := s.do(func() (any, error) {
		return &CollectionData{
			Name:     SessionCollectionName,
			Label:    s.collLabel,
			Created:  s.collTime,
			Modified: s.collTime,
			Locked:   false,
		}, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*CollectionData), nil
}

// CreateCollection is a label-only operation here: the session keyring is
// created in NewKeyringStore and can't be re-created. Callers that pass
// SessionCollectionName at startup get their label honoured.
func (s *KeyringStore) CreateCollection(ctx context.Context, name, label string) error {
	if err := s.checkColl(name); err != nil {
		return err
	}
	_, err := s.do(func() (any, error) {
		if label != "" {
			s.collLabel = label
		}
		return nil, nil
	})
	return err
}

func (s *KeyringStore) DeleteCollection(ctx context.Context, name string) error {
	return fmt.Errorf("session collection cannot be deleted")
}

func (s *KeyringStore) SetCollectionLabel(ctx context.Context, name, label string) error {
	if err := s.checkColl(name); err != nil {
		return err
	}
	_, err := s.do(func() (any, error) {
		s.collLabel = label
		s.collTime = time.Now()
		return nil, nil
	})
	return err
}

func (s *KeyringStore) Items(ctx context.Context, collection string) ([]string, error) {
	if err := s.checkColl(collection); err != nil {
		return nil, err
	}
	v, err := s.do(func() (any, error) {
		ids := make([]string, 0, len(s.items))
		for id := range s.items {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		return ids, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]string), nil
}

func (s *KeyringStore) GetItem(ctx context.Context, collection, id string) (*ItemData, error) {
	if err := s.checkColl(collection); err != nil {
		return nil, err
	}
	v, err := s.do(func() (any, error) {
		keyID, ok := s.items[id]
		if !ok {
			return nil, fmt.Errorf("item not found: %s", id)
		}
		payload, err := readKeyPayload(keyID)
		if err != nil {
			return nil, err
		}
		return decodeItem(id, payload)
	})
	if err != nil {
		return nil, err
	}
	return v.(*ItemData), nil
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
	v, err := s.do(func() (any, error) {
		keyID, err := unix.AddKey(keyringKeyType, item.ID, payload, s.ringID)
		if err != nil {
			return "", fmt.Errorf("add key: %w", err)
		}
		s.items[item.ID] = keyID
		return item.ID, nil
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

// UpdateItem rewrites the payload of an existing item, preserving Created.
// AddKey with the same description+type+ringid coalesces onto the existing
// key, so the key ID is stable across updates.
func (s *KeyringStore) UpdateItem(ctx context.Context, collection, id string, item *ItemData) error {
	if err := s.checkColl(collection); err != nil {
		return err
	}
	_, err := s.do(func() (any, error) {
		existingID, ok := s.items[id]
		if !ok {
			return nil, fmt.Errorf("item not found: %s", id)
		}
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
			return nil, err
		}
		keyID, err := unix.AddKey(keyringKeyType, id, payload, s.ringID)
		if err != nil {
			return nil, fmt.Errorf("update key: %w", err)
		}
		s.items[id] = keyID
		return nil, nil
	})
	return err
}

func (s *KeyringStore) DeleteItem(ctx context.Context, collection, id string) error {
	if err := s.checkColl(collection); err != nil {
		return err
	}
	_, err := s.do(func() (any, error) {
		keyID, ok := s.items[id]
		if !ok {
			return nil, fmt.Errorf("item not found: %s", id)
		}
		if _, err := unix.KeyctlInt(unix.KEYCTL_UNLINK, keyID, s.ringID, 0, 0); err != nil {
			return nil, fmt.Errorf("unlink key %d: %w", keyID, err)
		}
		delete(s.items, id)
		return nil, nil
	})
	return err
}

func (s *KeyringStore) SearchItems(ctx context.Context, collection string, attributes map[string]string) ([]*ItemData, error) {
	if err := s.checkColl(collection); err != nil {
		return nil, err
	}
	v, err := s.do(func() (any, error) {
		var matches []*ItemData
		for id, keyID := range s.items {
			payload, err := readKeyPayload(keyID)
			if err != nil {
				continue
			}
			item, err := decodeItem(id, payload)
			if err != nil {
				continue
			}
			if matchesAttributes(item, attributes) {
				matches = append(matches, item)
			}
		}
		return matches, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]*ItemData), nil
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

// Close signals the worker to clear the child keyring and exit, then blocks
// until the worker is fully done. Safe to call multiple times.
func (s *KeyringStore) Close(ctx context.Context) error {
	s.closeOnce.Do(func() { close(s.closed) })
	select {
	case <-s.workerDone:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}
