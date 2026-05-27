package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/gopasspw/gopass/pkg/gopass"
	"github.com/gopasspw/gopass/pkg/gopass/secrets"
)

// fakeGopassStore is a minimal gopass.Store that records how many times each
// path is decrypted, so tests can assert that the cache avoids re-decryption.
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

func (f *fakeGopassStore) put(name, password string) {
	sec := secrets.New()
	sec.SetPassword(password)
	f.data[name] = sec
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

func (f *fakeGopassStore) Set(ctx context.Context, name string, sec gopass.Byter) error {
	s := secrets.New()
	s.SetPassword(string(sec.Bytes()))
	f.data[name] = s
	return nil
}

func (f *fakeGopassStore) Revisions(ctx context.Context, name string) ([]string, error) {
	return []string{"latest"}, nil
}

func (f *fakeGopassStore) Remove(ctx context.Context, name string) error {
	delete(f.data, name)
	return nil
}

func (f *fakeGopassStore) RemoveAll(ctx context.Context, prefix string) error { return nil }
func (f *fakeGopassStore) Rename(ctx context.Context, src, dest string) error { return nil }
func (f *fakeGopassStore) Sync(ctx context.Context) error                     { return nil }
func (f *fakeGopassStore) Close(ctx context.Context) error                    { return nil }

func TestCachingStoreServesRepeatedReadsFromCache(t *testing.T) {
	ctx := context.Background()
	inner := newFakeGopassStore()
	inner.put("secret-service/default/item-a", "secret-a")

	cache := newCachingStore(inner)

	for i := range 5 {
		sec, err := cache.Get(ctx, "secret-service/default/item-a", "latest")
		if err != nil {
			t.Fatalf("Get #%d: %v", i, err)
		}
		if got := sec.Password(); got != "secret-a" {
			t.Fatalf("Get #%d: password = %q, want %q", i, got, "secret-a")
		}
	}

	if n := inner.getCount["secret-service/default/item-a"]; n != 1 {
		t.Fatalf("inner decryptions = %d, want 1 (subsequent reads should hit cache)", n)
	}
}

func TestCachingStoreBypassesCacheForExplicitRevision(t *testing.T) {
	ctx := context.Background()
	inner := newFakeGopassStore()
	inner.put("secret-service/default/item-a", "secret-a")

	cache := newCachingStore(inner)

	for i := range 3 {
		if _, err := cache.Get(ctx, "secret-service/default/item-a", "HEAD~1"); err != nil {
			t.Fatalf("Get #%d: %v", i, err)
		}
	}

	if n := inner.getCount["secret-service/default/item-a"]; n != 3 {
		t.Fatalf("inner decryptions = %d, want 3 (explicit revisions must bypass cache)", n)
	}
}

func TestCachingStoreCachesMiss(t *testing.T) {
	ctx := context.Background()
	inner := newFakeGopassStore()
	cache := newCachingStore(inner)

	// Errors must not be cached: a path that does not exist yet should be
	// retried on the inner store rather than masked by a cached failure.
	if _, err := cache.Get(ctx, "missing", "latest"); err == nil {
		t.Fatal("expected error for missing secret")
	}
	inner.put("missing", "now-present")
	sec, err := cache.Get(ctx, "missing", "latest")
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if got := sec.Password(); got != "now-present" {
		t.Fatalf("password = %q, want %q", got, "now-present")
	}
}
