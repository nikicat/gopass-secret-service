package store

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gopasspw/gopass/pkg/gopass"
	"github.com/gopasspw/gopass/pkg/gopass/secrets"
)

// fakeGopassStore is a minimal gopass.Store used to exercise GopassStore's
// metadata cache without GPG. It records how many times each path is decrypted
// (Get) so tests can assert the cache avoids re-decryption.
type fakeGopassStore struct {
	data     map[string]gopass.Secret
	getCount map[string]int
}

func newFakeGopassStore() *fakeGopassStore {
	return &fakeGopassStore{
		data:     make(map[string]gopass.Secret),
		getCount: make(map[string]int),
	}
}

// putSecret stores a secret at path with the given password and attributes,
// mirroring how GopassStore lays out an item (attributes as metadata keys).
func (f *fakeGopassStore) putSecret(path, password string, attrs map[string]string) {
	sec := secrets.New()
	sec.SetPassword(password)
	for k, v := range attrs {
		_ = sec.Set(k, v)
	}
	f.data[path] = sec
}

func (f *fakeGopassStore) String() string { return "fakeGopassStore" }

func (f *fakeGopassStore) List(ctx context.Context) ([]string, error) {
	out := make([]string, 0, len(f.data))
	for k := range f.data {
		out = append(out, k)
	}
	return out, nil
}

func (f *fakeGopassStore) Get(ctx context.Context, name, revision string) (gopass.Secret, error) {
	f.getCount[name]++
	sec, ok := f.data[name]
	if !ok {
		return nil, fmt.Errorf("not found: %s", name)
	}
	return sec, nil
}

func (f *fakeGopassStore) Set(ctx context.Context, name string, b gopass.Byter) error {
	// GopassStore always passes a *secrets.AKV, which is also a gopass.Secret,
	// so we can store it verbatim and serve its metadata back on Get.
	if sec, ok := b.(gopass.Secret); ok {
		f.data[name] = sec
		return nil
	}
	sec := secrets.New()
	sec.SetPassword(string(b.Bytes()))
	f.data[name] = sec
	return nil
}

func (f *fakeGopassStore) Revisions(ctx context.Context, name string) ([]string, error) {
	return []string{"latest"}, nil
}

func (f *fakeGopassStore) Remove(ctx context.Context, name string) error {
	delete(f.data, name)
	return nil
}

func (f *fakeGopassStore) RemoveAll(ctx context.Context, prefix string) error {
	for k := range f.data {
		if k == prefix || strings.HasPrefix(k, prefix+"/") {
			delete(f.data, k)
		}
	}
	return nil
}

func (f *fakeGopassStore) Rename(ctx context.Context, src, dest string) error { return nil }
func (f *fakeGopassStore) Sync(ctx context.Context) error                     { return nil }
func (f *fakeGopassStore) Close(ctx context.Context) error                    { return nil }

func newTestGopassStore(inner gopass.Store) *GopassStore {
	return &GopassStore{
		store:     inner,
		mapper:    NewMapper("secret-service"),
		locked:    make(map[string]bool),
		metaCache: make(map[string]map[string]string),
	}
}

// TestSearchItemsCachesMetadata verifies the perf fix: repeated searches must
// decrypt each entry at most once.
func TestSearchItemsCachesMetadata(t *testing.T) {
	ctx := context.Background()
	fake := newFakeGopassStore()
	s := newTestGopassStore(fake)
	path := s.mapper.ItemPath("default", "item-a")
	fake.putSecret(path, "secret-a", map[string]string{"service": "etherscan.io"})

	for i := range 5 {
		res, err := s.SearchItems(ctx, "default", map[string]string{"service": "etherscan.io"})
		if err != nil {
			t.Fatalf("SearchItems #%d: %v", i, err)
		}
		if len(res) != 1 || res[0].ID != "item-a" {
			t.Fatalf("SearchItems #%d: got %d results, want item-a", i, len(res))
		}
	}

	if n := fake.getCount[path]; n != 1 {
		t.Fatalf("decryptions = %d, want 1 (repeated searches must hit cache)", n)
	}
}

// TestSearchItemsNeverCachesSecret is the regression test for the rule that the
// secret value must never be retained in memory. The metadata cache must hold
// attributes only, and the search path must not surface the password.
func TestSearchItemsNeverCachesSecret(t *testing.T) {
	ctx := context.Background()
	// A distinctive stand-in for a decrypted secret payload. Named/worded to
	// avoid gosec's hardcoded-credential heuristic; it is not a real credential.
	const payload = "decrypted-payload-marker-xyz"

	fake := newFakeGopassStore()
	s := newTestGopassStore(fake)
	path := s.mapper.ItemPath("default", "item-a")
	fake.putSecret(path, payload, map[string]string{"service": "etherscan.io"})

	res, err := s.SearchItems(ctx, "default", map[string]string{"service": "etherscan.io"})
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("got %d results, want 1", len(res))
	}
	// The search result must not carry the decrypted secret.
	if len(res[0].Secret) != 0 {
		t.Fatalf("SearchItems result leaked secret value: %q", res[0].Secret)
	}

	// Reading the actual secret must work and must not poison the cache.
	item, err := s.GetItem(ctx, "default", "item-a")
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if string(item.Secret) != payload {
		t.Fatalf("GetItem secret = %q, want %q", item.Secret, payload)
	}

	assertCacheHasNoSecret(t, s, payload)
}

// TestWriteInvalidatesCache is the regression test for the cache-coherence bug:
// an in-process update must be reflected by subsequent reads rather than
// returning a stale, cached pre-write value.
func TestWriteInvalidatesCache(t *testing.T) {
	ctx := context.Background()
	fake := newFakeGopassStore()
	s := newTestGopassStore(fake)
	path := s.mapper.ItemPath("default", "item-a")
	fake.putSecret(path, "secret-a", map[string]string{
		labelKey:  "etherscan key",
		"service": "etherscan.io",
	})

	// Warm the cache with the original attributes.
	if res, _ := s.SearchItems(ctx, "default", map[string]string{"service": "etherscan.io"}); len(res) != 1 {
		t.Fatalf("pre-update search: got %d, want 1", len(res))
	}

	// Update the item's attributes via the daemon's own write path.
	err := s.UpdateItem(ctx, "default", "item-a", &ItemData{
		Label:      "etherscan key",
		Secret:     []byte("secret-a"),
		Attributes: map[string]string{"service": "etherscan-v2.io"},
	})
	if err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}

	// The stale attribute must no longer match...
	if res, _ := s.SearchItems(ctx, "default", map[string]string{"service": "etherscan.io"}); len(res) != 0 {
		t.Fatalf("search by stale attribute returned %d results, want 0 (cache not invalidated)", len(res))
	}
	// ...and the new attribute must match.
	if res, _ := s.SearchItems(ctx, "default", map[string]string{"service": "etherscan-v2.io"}); len(res) != 1 {
		t.Fatalf("search by updated attribute returned %d results, want 1", len(res))
	}
}

// TestItemLifecycle_StoreCacheModifyRetrieve walks the full lifecycle end to
// end against a decryption-counting backend:
//
//  1. store a secret,
//  2. retrieve it (populates the cache),
//  3. retrieve it again and assert the backend was NOT hit (cache works),
//  4. modify it,
//  5. retrieve again and assert the new value is returned (cache invalidated).
//
// It also pins the deliberate split between the two read paths: attribute
// lookups (SearchItems) are cached, while the secret value (GetItem) is always
// decrypted fresh and never retained.
func TestItemLifecycle_StoreCacheModifyRetrieve(t *testing.T) {
	ctx := context.Background()
	fake := newFakeGopassStore()
	s := newTestGopassStore(fake)

	// 1. Store a secret via the daemon's own write path.
	id, err := s.CreateItem(ctx, "default", &ItemData{
		Label:      "etherscan",
		Secret:     []byte("value-v1"),
		Attributes: map[string]string{"service": "etherscan.io"},
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	path := s.mapper.ItemPath("default", id)
	search := func() []*ItemData {
		t.Helper()
		res, err := s.SearchItems(ctx, "default", map[string]string{"service": "etherscan.io"})
		if err != nil {
			t.Fatalf("SearchItems: %v", err)
		}
		return res
	}

	// 2. Retrieve by attribute — first lookup decrypts and caches.
	if got := search(); len(got) != 1 {
		t.Fatalf("first lookup: got %d results, want 1", len(got))
	}
	afterFirst := fake.getCount[path]
	if afterFirst == 0 {
		t.Fatal("first lookup should have decrypted the item at least once")
	}

	// 3. Retrieve again — served from cache, backend NOT hit.
	if got := search(); len(got) != 1 {
		t.Fatalf("second lookup: got %d results, want 1", len(got))
	}
	if got := fake.getCount[path]; got != afterFirst {
		t.Fatalf("second lookup hit storage: decryptions %d -> %d (expected cache hit)", afterFirst, got)
	}
	// A cached lookup must never expose the secret value.
	if got := search(); len(got[0].Secret) != 0 {
		t.Fatalf("lookup leaked secret value: %q", got[0].Secret)
	}

	// The secret value itself is never cached: GetItem always decrypts fresh.
	beforeGet := fake.getCount[path]
	item, err := s.GetItem(ctx, "default", id)
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if string(item.Secret) != "value-v1" {
		t.Fatalf("GetItem secret = %q, want value-v1", item.Secret)
	}
	if got := fake.getCount[path]; got != beforeGet+1 {
		t.Fatalf("GetItem should always decrypt the secret: count %d -> %d", beforeGet, got)
	}

	// 4. Modify the secret value and an attribute.
	if err := s.UpdateItem(ctx, "default", id, &ItemData{
		Label:      "etherscan",
		Secret:     []byte("value-v2"),
		Attributes: map[string]string{"service": "etherscan-v2.io"},
	}); err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}

	// 5. Retrieve again — the post-modification value must be returned.
	item, err = s.GetItem(ctx, "default", id)
	if err != nil {
		t.Fatalf("GetItem after update: %v", err)
	}
	if string(item.Secret) != "value-v2" {
		t.Fatalf("after update GetItem secret = %q, want value-v2 (stale cache?)", item.Secret)
	}
	// And lookups reflect the new attribute (old one no longer matches).
	if got := search(); len(got) != 0 {
		t.Fatalf("lookup by stale attribute returned %d results, want 0 (cache not invalidated)", len(got))
	}
	if got, _ := s.SearchItems(ctx, "default", map[string]string{"service": "etherscan-v2.io"}); len(got) != 1 {
		t.Fatalf("lookup by updated attribute returned %d results, want 1", len(got))
	}
}

// assertCacheHasNoSecret fails if the decrypted secret value appears anywhere in
// the metadata cache.
func assertCacheHasNoSecret(t *testing.T, s *GopassStore, secret string) {
	t.Helper()
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	if len(s.metaCache) == 0 {
		t.Fatal("metadata cache is empty; expected at least one cached entry")
	}
	for path, meta := range s.metaCache {
		for k, v := range meta {
			if v == secret || strings.Contains(v, secret) {
				t.Fatalf("secret value leaked into metadata cache at %s[%q] = %q", path, k, v)
			}
		}
	}
}
