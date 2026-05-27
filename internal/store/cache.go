package store

import (
	"context"
	"sync"

	"github.com/gopasspw/gopass/pkg/gopass"
)

// cachingStore wraps a gopass.Store with an in-memory cache of decrypted
// secrets, keyed by path.
//
// GPG decryption dominates read latency, and because gopass stores attributes
// inside the encrypted payload there is no plaintext index: SearchItems must
// decrypt every entry in a collection on every call (O(N) decryptions at
// roughly 0.4s each). Caching collapses that to a single decryption per entry
// for the lifetime of the process.
//
// The cache has no TTL and is never invalidated. The daemon is assumed to be
// the sole owner of its gopass store, so the only way to discard cached
// plaintext is to restart the daemon. Two consequences follow:
//   - Out-of-process mutations (e.g. the gopass CLI, a manual git pull) are not
//     observed until the daemon restarts.
//   - Reads that follow an in-process write to the same path return the cached
//     pre-write value until restart. The daemon writes gopass entries rarely
//     (session/keyring writes bypass this store), so this is acceptable.
type cachingStore struct {
	gopass.Store

	mu    sync.RWMutex
	items map[string]gopass.Secret
}

// newCachingStore wraps inner so that "latest" reads are served from memory
// after their first decryption.
func newCachingStore(inner gopass.Store) *cachingStore {
	return &cachingStore{
		Store: inner,
		items: make(map[string]gopass.Secret),
	}
}

// Get returns the decrypted secret for name. The common "latest" read is served
// from cache after the first decryption; explicit historical revisions bypass
// the cache so they always reflect the requested revision.
func (c *cachingStore) Get(ctx context.Context, name, revision string) (gopass.Secret, error) {
	if revision != "" && revision != "latest" {
		return c.Store.Get(ctx, name, revision)
	}

	c.mu.RLock()
	sec, ok := c.items[name]
	c.mu.RUnlock()
	if ok {
		return sec, nil
	}

	sec, err := c.Store.Get(ctx, name, revision)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.items[name] = sec
	c.mu.Unlock()
	return sec, nil
}
