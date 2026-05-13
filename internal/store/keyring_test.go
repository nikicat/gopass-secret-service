package store

import (
	"context"
	"runtime"
	"sync"
	"testing"
)

func newTestKeyringStore(t *testing.T) *KeyringStore {
	t.Helper()
	s, err := NewKeyringStore()
	if err != nil {
		t.Skipf("kernel keyring unavailable: %v", err)
	}
	t.Cleanup(func() { _ = s.Close(context.Background()) })
	return s
}

func TestKeyringStore_CreateItemAssignsIDAndRoundTrip(t *testing.T) {
	s := newTestKeyringStore(t)
	ctx := context.Background()

	item := &ItemData{
		Label:       "kubelogin token",
		Secret:      []byte("opaque-payload"),
		ContentType: "application/json",
		Attributes:  map[string]string{"service": "kubelogin", "username": "user1"},
	}
	id, err := s.CreateItem(ctx, SessionCollectionName, item)
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	if id == "" {
		t.Fatalf("CreateItem returned empty id")
	}

	got, err := s.GetItem(ctx, SessionCollectionName, id)
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if got.Label != "kubelogin token" || string(got.Secret) != "opaque-payload" || got.ContentType != "application/json" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if got.Attributes["service"] != "kubelogin" || got.Attributes["username"] != "user1" {
		t.Errorf("attributes lost: %+v", got.Attributes)
	}
	if got.Created.IsZero() || got.Modified.IsZero() {
		t.Errorf("timestamps not set: created=%v modified=%v", got.Created, got.Modified)
	}
}

func TestKeyringStore_UpdatePreservesCreated(t *testing.T) {
	s := newTestKeyringStore(t)
	ctx := context.Background()

	id, err := s.CreateItem(ctx, SessionCollectionName, &ItemData{
		Label:      "v1",
		Secret:     []byte("v1-secret"),
		Attributes: map[string]string{"k": "v"},
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	first, err := s.GetItem(ctx, SessionCollectionName, id)
	if err != nil {
		t.Fatalf("GetItem 1: %v", err)
	}

	updated := &ItemData{
		Label:      "v2",
		Secret:     []byte("v2-secret"),
		Attributes: map[string]string{"k": "v"},
	}
	if err := s.UpdateItem(ctx, SessionCollectionName, id, updated); err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}
	second, err := s.GetItem(ctx, SessionCollectionName, id)
	if err != nil {
		t.Fatalf("GetItem 2: %v", err)
	}
	if !second.Created.Equal(first.Created) {
		t.Errorf("Update changed Created: was %v, now %v", first.Created, second.Created)
	}
	if second.Modified.Before(first.Modified) {
		t.Errorf("Modified went backwards: was %v, now %v", first.Modified, second.Modified)
	}
	if second.Label != "v2" || string(second.Secret) != "v2-secret" {
		t.Errorf("update did not apply: %+v", second)
	}
}

func TestKeyringStore_DeleteItemRemovesIt(t *testing.T) {
	s := newTestKeyringStore(t)
	ctx := context.Background()

	id, err := s.CreateItem(ctx, SessionCollectionName, &ItemData{Label: "x", Secret: []byte("y")})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	if err := s.DeleteItem(ctx, SessionCollectionName, id); err != nil {
		t.Fatalf("DeleteItem: %v", err)
	}
	if _, err := s.GetItem(ctx, SessionCollectionName, id); err == nil {
		t.Errorf("GetItem after Delete unexpectedly succeeded")
	}
	ids, _ := s.Items(ctx, SessionCollectionName)
	if len(ids) != 0 {
		t.Errorf("Items after Delete = %v, want empty", ids)
	}
}

func TestKeyringStore_SearchItemsByAttributes(t *testing.T) {
	s := newTestKeyringStore(t)
	ctx := context.Background()

	_, err := s.CreateItem(ctx, SessionCollectionName, &ItemData{
		Label:      "match",
		Secret:     []byte("a"),
		Attributes: map[string]string{"service": "kubelogin", "username": "u1"},
	})
	if err != nil {
		t.Fatalf("CreateItem 1: %v", err)
	}
	_, err = s.CreateItem(ctx, SessionCollectionName, &ItemData{
		Label:      "other",
		Secret:     []byte("b"),
		Attributes: map[string]string{"service": "gh"},
	})
	if err != nil {
		t.Fatalf("CreateItem 2: %v", err)
	}

	matches, err := s.SearchItems(ctx, SessionCollectionName, map[string]string{"service": "kubelogin"})
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if len(matches) != 1 || matches[0].Label != "match" {
		t.Errorf("SearchItems matched %d items; want 1 (match): %+v", len(matches), matches)
	}
}

func TestKeyringStore_RejectsNonSessionCollection(t *testing.T) {
	s := newTestKeyringStore(t)
	ctx := context.Background()

	if _, err := s.CreateItem(ctx, "other", &ItemData{Secret: []byte("x")}); err == nil {
		t.Errorf("CreateItem on non-session collection should fail")
	}
	if _, err := s.GetCollection(ctx, "other"); err == nil {
		t.Errorf("GetCollection on non-session should fail")
	}
}

func TestKeyringStore_CollectionsListsExactlyOne(t *testing.T) {
	s := newTestKeyringStore(t)
	ctx := context.Background()
	cols, err := s.Collections(ctx)
	if err != nil {
		t.Fatalf("Collections: %v", err)
	}
	if len(cols) != 1 || cols[0] != SessionCollectionName {
		t.Errorf("Collections = %v, want [%q]", cols, SessionCollectionName)
	}
}

func TestKeyringStore_CloseClearsItems(t *testing.T) {
	s, err := NewKeyringStore()
	if err != nil {
		t.Skipf("kernel keyring unavailable: %v", err)
	}
	ctx := context.Background()
	if _, err := s.CreateItem(ctx, SessionCollectionName, &ItemData{Secret: []byte("x")}); err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	if err := s.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if len(s.items) != 0 {
		t.Errorf("items map should be empty after Close, got %d", len(s.items))
	}
}

// TestKeyringStore_CRUDFromForeignThread is the regression test for the
// per-task-cred bug. It performs CRUD from a goroutine that's pinned to its
// *own* OS thread via runtime.LockOSThread — a thread that, by construction,
// was not the one that installed the daemon's process keyring. Before the
// dedicated-worker fix, the syscall would land on this thread's M, whose
// per-task cred lacks process_keyring, no possession is established on the
// child keyring, and AddKey returns EACCES. After the fix, all syscalls
// marshal onto the worker thread regardless of which goroutine called in.
func TestKeyringStore_CRUDFromForeignThread(t *testing.T) {
	s := newTestKeyringStore(t)
	ctx := context.Background()

	type result struct {
		id  string
		err error
	}

	// Goroutine that's deliberately on a different OS thread than the
	// worker. LockOSThread is what guarantees the syscalls (if they
	// happened on this M) would land on a cred with no process_keyring.
	creates := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		id, err := s.CreateItem(ctx, SessionCollectionName, &ItemData{
			Label:      "from foreign thread",
			Secret:     []byte("foreign-thread-payload"),
			Attributes: map[string]string{"k": "v"},
		})
		creates <- result{id, err}
	}()
	r := <-creates
	if r.err != nil {
		t.Fatalf("CreateItem from foreign thread: %v", r.err)
	}

	gets := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		got, err := s.GetItem(ctx, SessionCollectionName, r.id)
		if err != nil {
			gets <- err
			return
		}
		if string(got.Secret) != "foreign-thread-payload" {
			gets <- &mismatchErr{want: "foreign-thread-payload", got: string(got.Secret)}
			return
		}
		gets <- nil
	}()
	if err := <-gets; err != nil {
		t.Fatalf("GetItem from foreign thread: %v", err)
	}

	dels := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		dels <- s.DeleteItem(ctx, SessionCollectionName, r.id)
	}()
	if err := <-dels; err != nil {
		t.Fatalf("DeleteItem from foreign thread: %v", err)
	}
}

type mismatchErr struct{ want, got string }

func (e *mismatchErr) Error() string { return "secret mismatch: want " + e.want + ", got " + e.got }

// TestKeyringStore_ConcurrentCRUD runs goroutines hammering the store in
// parallel. The worker serializes their syscalls, so this should never race
// or fail. Two failure modes it would catch: (1) a regression to direct
// syscalls from the calling goroutine (would surface EACCES on some Ms), and
// (2) lost updates from unguarded mutation of s.items.
//
// Each goroutine creates ONE item up-front and then loops Get/Update on it
// rather than churning create/delete. The kernel keyring's per-UID byte
// quota (default 20 000) accounts unlinked-but-not-yet-GC'd keys against the
// budget until gc_delay (default 300s) elapses — a tight create/delete loop
// burns through it in seconds.
func TestKeyringStore_ConcurrentCRUD(t *testing.T) {
	s := newTestKeyringStore(t)
	ctx := context.Background()

	const goroutines = 6
	const opsPerGoroutine = 10

	type slot struct {
		id  string
		err error
	}
	ids := make([]string, goroutines)
	for i := range ids {
		id, err := s.CreateItem(ctx, SessionCollectionName, &ItemData{
			Label:      "concurrent",
			Secret:     []byte("v0"),
			Attributes: map[string]string{"slot": "v"},
		})
		if err != nil {
			t.Fatalf("seed CreateItem: %v", err)
		}
		ids[i] = id
	}

	errs := make(chan error, goroutines*opsPerGoroutine)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := range goroutines {
		go func(slotID string) {
			defer wg.Done()
			for range opsPerGoroutine {
				if _, err := s.GetItem(ctx, SessionCollectionName, slotID); err != nil {
					errs <- err
					return
				}
				if err := s.UpdateItem(ctx, SessionCollectionName, slotID, &ItemData{
					Label:      "concurrent",
					Secret:     []byte("updated"),
					Attributes: map[string]string{"slot": "v"},
				}); err != nil {
					errs <- err
					return
				}
			}
		}(ids[g])
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent op: %v", err)
	}
}

// TestKeyringStore_OperationsAfterCloseFail ensures Close shuts down cleanly
// and subsequent calls return a sensible error rather than hanging forever or
// crashing the worker.
func TestKeyringStore_OperationsAfterCloseFail(t *testing.T) {
	s := newTestKeyringStore(t) // newTestKeyringStore registers its own Close in t.Cleanup
	ctx := context.Background()
	if err := s.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := s.CreateItem(ctx, SessionCollectionName, &ItemData{Secret: []byte("x")}); err == nil {
		t.Errorf("CreateItem after Close should error, got nil")
	}
	if err := s.Close(ctx); err != nil {
		t.Errorf("second Close should be a no-op, got %v", err)
	}
}
