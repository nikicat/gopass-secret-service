package store

import (
	"context"
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
