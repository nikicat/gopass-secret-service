package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// fakeStore is an in-memory Store implementation used to test MultiStore
// routing without touching gopass or the kernel keyring.
type fakeStore struct {
	name        string                          // a tag we put into errors to identify which fake handled a call
	collections map[string]*CollectionData      // by name
	items       map[string]map[string]*ItemData // collection -> id -> item
	aliases     map[string]string
	closeCount  int
}

func newFakeStore(name string) *fakeStore {
	return &fakeStore{
		name:        name,
		collections: map[string]*CollectionData{},
		items:       map[string]map[string]*ItemData{},
		aliases:     map[string]string{},
	}
}

func (f *fakeStore) Collections(ctx context.Context) ([]string, error) {
	out := make([]string, 0, len(f.collections))
	for k := range f.collections {
		out = append(out, k)
	}
	return out, nil
}

func (f *fakeStore) GetCollection(ctx context.Context, name string) (*CollectionData, error) {
	c, ok := f.collections[name]
	if !ok {
		return nil, fmt.Errorf("%s: not found %s", f.name, name)
	}
	return c, nil
}

func (f *fakeStore) CreateCollection(ctx context.Context, name, label string) error {
	f.collections[name] = &CollectionData{Name: name, Label: label, Created: time.Now()}
	if f.items[name] == nil {
		f.items[name] = map[string]*ItemData{}
	}
	return nil
}

func (f *fakeStore) DeleteCollection(ctx context.Context, name string) error {
	delete(f.collections, name)
	delete(f.items, name)
	return nil
}

func (f *fakeStore) SetCollectionLabel(ctx context.Context, name, label string) error {
	if c, ok := f.collections[name]; ok {
		c.Label = label
	}
	return nil
}

func (f *fakeStore) Items(ctx context.Context, collection string) ([]string, error) {
	m, ok := f.items[collection]
	if !ok {
		return nil, fmt.Errorf("%s: no collection %s", f.name, collection)
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out, nil
}

func (f *fakeStore) GetItem(ctx context.Context, collection, id string) (*ItemData, error) {
	m := f.items[collection]
	if it, ok := m[id]; ok {
		return it, nil
	}
	return nil, fmt.Errorf("%s: item not found %s/%s", f.name, collection, id)
}

func (f *fakeStore) CreateItem(ctx context.Context, collection string, item *ItemData) (string, error) {
	if f.items[collection] == nil {
		f.items[collection] = map[string]*ItemData{}
	}
	if item.ID == "" {
		item.ID = fmt.Sprintf("%s-item-%d", f.name, len(f.items[collection]))
	}
	f.items[collection][item.ID] = item
	return item.ID, nil
}

func (f *fakeStore) UpdateItem(ctx context.Context, collection, id string, item *ItemData) error {
	f.items[collection][id] = item
	return nil
}

func (f *fakeStore) DeleteItem(ctx context.Context, collection, id string) error {
	delete(f.items[collection], id)
	return nil
}

func (f *fakeStore) SearchItems(ctx context.Context, collection string, attributes map[string]string) ([]*ItemData, error) {
	var out []*ItemData
	for _, it := range f.items[collection] {
		if matchesAttributes(it, attributes) {
			out = append(out, it)
		}
	}
	return out, nil
}

func (f *fakeStore) SearchAllItems(ctx context.Context, attributes map[string]string) (map[string][]*ItemData, error) {
	out := map[string][]*ItemData{}
	for coll, m := range f.items {
		var matches []*ItemData
		for _, it := range m {
			if matchesAttributes(it, attributes) {
				matches = append(matches, it)
			}
		}
		if len(matches) > 0 {
			out[coll] = matches
		}
	}
	return out, nil
}

func (f *fakeStore) LockCollection(ctx context.Context, name string) error   { return nil }
func (f *fakeStore) UnlockCollection(ctx context.Context, name string) error { return nil }

func (f *fakeStore) GetAlias(ctx context.Context, alias string) (string, error) {
	if v, ok := f.aliases[alias]; ok {
		return v, nil
	}
	return "", fmt.Errorf("%s: alias not found %s", f.name, alias)
}

func (f *fakeStore) SetAlias(ctx context.Context, alias, collection string) error {
	if collection == "" {
		delete(f.aliases, alias)
	} else {
		f.aliases[alias] = collection
	}
	return nil
}

func (f *fakeStore) Close(ctx context.Context) error {
	f.closeCount++
	return nil
}

func newMulti() (*MultiStore, *fakeStore, *fakeStore) {
	primary := newFakeStore("primary")
	session := newFakeStore("session")
	_ = primary.CreateCollection(context.Background(), "default", "Default")
	_ = session.CreateCollection(context.Background(), SessionCollectionName, "Session")
	return NewMultiStore(primary, session), primary, session
}

func TestMultiStore_RouteByCollectionName(t *testing.T) {
	m, primary, session := newMulti()
	ctx := context.Background()

	if _, err := m.CreateItem(ctx, "default", &ItemData{Label: "p"}); err != nil {
		t.Fatalf("CreateItem default: %v", err)
	}
	if _, err := m.CreateItem(ctx, SessionCollectionName, &ItemData{Label: "s"}); err != nil {
		t.Fatalf("CreateItem session: %v", err)
	}
	if len(primary.items["default"]) != 1 {
		t.Errorf("primary should have 1 item in default, has %d", len(primary.items["default"]))
	}
	if len(session.items[SessionCollectionName]) != 1 {
		t.Errorf("session should have 1 item, has %d", len(session.items[SessionCollectionName]))
	}
	if len(primary.items[SessionCollectionName]) != 0 {
		t.Errorf("primary should not have items in session collection")
	}
}

func TestMultiStore_CollectionsIncludesSession(t *testing.T) {
	m, _, _ := newMulti()
	cols, err := m.Collections(context.Background())
	if err != nil {
		t.Fatalf("Collections: %v", err)
	}
	found := false
	for _, c := range cols {
		if c == SessionCollectionName {
			found = true
		}
	}
	if !found {
		t.Errorf("Collections = %v, want it to include %q", cols, SessionCollectionName)
	}
}

func TestMultiStore_SessionCollectionsNotDuplicated(t *testing.T) {
	// If the primary store already lists the session name (shouldn't happen
	// in practice but defend against it), MultiStore must not double it.
	primary := newFakeStore("primary")
	session := newFakeStore("session")
	_ = primary.CreateCollection(context.Background(), SessionCollectionName, "old")
	m := NewMultiStore(primary, session)

	cols, _ := m.Collections(context.Background())
	count := 0
	for _, c := range cols {
		if c == SessionCollectionName {
			count++
		}
	}
	if count != 1 {
		t.Errorf("session collection appeared %d times in %v, want 1", count, cols)
	}
}

func TestMultiStore_GetAliasResolvesSessionLocally(t *testing.T) {
	m, _, _ := newMulti()
	got, err := m.GetAlias(context.Background(), "session")
	if err != nil {
		t.Fatalf("GetAlias(session): %v", err)
	}
	if got != SessionCollectionName {
		t.Errorf("GetAlias(session) = %q, want %q", got, SessionCollectionName)
	}
}

func TestMultiStore_SetAliasRejectsSession(t *testing.T) {
	m, _, _ := newMulti()
	if err := m.SetAlias(context.Background(), "session", "somewhere-else"); err == nil {
		t.Errorf("SetAlias(session) should be rejected as reserved")
	}
}

func TestMultiStore_SetAliasOtherForwardsToPrimary(t *testing.T) {
	m, primary, _ := newMulti()
	if err := m.SetAlias(context.Background(), "default", "default"); err != nil {
		t.Fatalf("SetAlias: %v", err)
	}
	if primary.aliases["default"] != "default" {
		t.Errorf("alias not forwarded to primary: %+v", primary.aliases)
	}
}

func TestMultiStore_SearchAllItemsMergesBoth(t *testing.T) {
	m, primary, session := newMulti()
	ctx := context.Background()
	primary.items["default"]["p1"] = &ItemData{ID: "p1", Attributes: map[string]string{"k": "v"}}
	session.items[SessionCollectionName]["s1"] = &ItemData{ID: "s1", Attributes: map[string]string{"k": "v"}}

	got, err := m.SearchAllItems(ctx, map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("SearchAllItems: %v", err)
	}
	if len(got["default"]) != 1 || got["default"][0].ID != "p1" {
		t.Errorf("default results wrong: %+v", got["default"])
	}
	if len(got[SessionCollectionName]) != 1 || got[SessionCollectionName][0].ID != "s1" {
		t.Errorf("session results wrong: %+v", got[SessionCollectionName])
	}
}

func TestMultiStore_CloseClosesBoth(t *testing.T) {
	m, primary, session := newMulti()
	if err := m.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if primary.closeCount != 1 || session.closeCount != 1 {
		t.Errorf("Close counts: primary=%d session=%d, want 1/1", primary.closeCount, session.closeCount)
	}
}
