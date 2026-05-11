package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/nikicat/gopass-secret-service/internal/config"
	dbtypes "github.com/nikicat/gopass-secret-service/internal/dbus"
	"github.com/nikicat/gopass-secret-service/internal/store"
)

// mockStore is a minimal in-memory store for testing.
type mockStore struct {
	mu          sync.Mutex
	collections map[string]*store.CollectionData
	aliases     map[string]string
	items       map[string]map[string]*store.ItemData // collection -> id -> item
}

func newMockStore() *mockStore {
	return &mockStore{
		collections: make(map[string]*store.CollectionData),
		aliases:     make(map[string]string),
		items:       make(map[string]map[string]*store.ItemData),
	}
}

func (m *mockStore) Collections(_ context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0, len(m.collections))
	for n := range m.collections {
		names = append(names, n)
	}
	return names, nil
}

func (m *mockStore) GetCollection(_ context.Context, name string) (*store.CollectionData, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.collections[name]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return c, nil
}

func (m *mockStore) CreateCollection(_ context.Context, name, label string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.collections[name] = &store.CollectionData{Name: name, Label: label, Created: now, Modified: now}
	return nil
}

func (m *mockStore) DeleteCollection(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.collections[name]; !ok {
		return fmt.Errorf("not found")
	}
	delete(m.collections, name)
	return nil
}

func (m *mockStore) SetCollectionLabel(_ context.Context, name, label string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.collections[name]; ok {
		c.Label = label
		return nil
	}
	return fmt.Errorf("not found")
}

func (m *mockStore) Items(_ context.Context, collection string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	collItems := m.items[collection]
	ids := make([]string, 0, len(collItems))
	for id := range collItems {
		ids = append(ids, id)
	}
	return ids, nil
}

func (m *mockStore) GetItem(_ context.Context, collection, id string) (*store.ItemData, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if collItems, ok := m.items[collection]; ok {
		if item, ok := collItems[id]; ok {
			return item, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockStore) CreateItem(_ context.Context, collection string, item *store.ItemData) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.items[collection] == nil {
		m.items[collection] = make(map[string]*store.ItemData)
	}
	m.items[collection][item.ID] = item
	return item.ID, nil
}

func (m *mockStore) UpdateItem(_ context.Context, collection, id string, item *store.ItemData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.items[collection] == nil {
		return fmt.Errorf("not found")
	}
	m.items[collection][id] = item
	return nil
}

func (m *mockStore) DeleteItem(_ context.Context, collection, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if collItems, ok := m.items[collection]; ok {
		delete(collItems, id)
	}
	return nil
}

func (m *mockStore) SearchItems(_ context.Context, collection string, attrs map[string]string) ([]*store.ItemData, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var results []*store.ItemData
	for _, item := range m.items[collection] {
		match := true
		for k, v := range attrs {
			if item.Attributes[k] != v {
				match = false
				break
			}
		}
		if match {
			results = append(results, item)
		}
	}
	return results, nil
}

func (m *mockStore) SearchAllItems(_ context.Context, attrs map[string]string) (map[string][]*store.ItemData, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	results := make(map[string][]*store.ItemData)
	for coll, collItems := range m.items {
		for _, item := range collItems {
			match := true
			for k, v := range attrs {
				if item.Attributes[k] != v {
					match = false
					break
				}
			}
			if match {
				results[coll] = append(results[coll], item)
			}
		}
	}
	return results, nil
}
func (m *mockStore) LockCollection(_ context.Context, _ string) error   { return nil }
func (m *mockStore) UnlockCollection(_ context.Context, _ string) error { return nil }

func (m *mockStore) GetAlias(_ context.Context, alias string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.aliases[alias]; ok {
		return v, nil
	}
	return "", fmt.Errorf("not found")
}

func (m *mockStore) SetAlias(_ context.Context, alias, collection string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.aliases[alias] = collection
	return nil
}

func (m *mockStore) Close(_ context.Context) error { return nil }

// startTestBus starts an isolated dbus-daemon and returns a connection and cleanup func.
func startTestBus(t *testing.T) (*dbus.Conn, func()) {
	t.Helper()

	dir := t.TempDir()
	sock := filepath.Join(dir, "bus.sock")
	addr := "unix:path=" + sock

	cmd := exec.Command("dbus-daemon", "--session", "--nofork", "--address="+addr)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start dbus-daemon: %v", err)
	}

	// Wait for socket
	for range 50 {
		if _, err := os.Stat(sock); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	conn, err := dbus.Dial(addr)
	if err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("dial bus: %v", err)
	}
	if err := conn.Auth(nil); err != nil {
		conn.Close()
		_ = cmd.Process.Kill()
		t.Fatalf("auth: %v", err)
	}
	if err := conn.Hello(); err != nil {
		conn.Close()
		_ = cmd.Process.Kill()
		t.Fatalf("hello: %v", err)
	}

	cleanup := func() {
		conn.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
	return conn, cleanup
}

// newTestService creates a Service with a mock store on an isolated bus.
func newTestService(t *testing.T) (*Service, *mockStore, func()) {
	t.Helper()
	conn, cleanup := startTestBus(t)
	ms := newMockStore()

	cfg := &config.Config{
		DefaultCollection: "default",
		Prefix:            "test",
		Replace:           true,
	}

	svc := &Service{
		conn:  conn,
		store: ms,
		cfg:   cfg,
	}
	svc.sessions = NewSessionManager(conn)
	svc.prompts = NewPromptManager(conn)
	svc.collections = NewCollectionManager(svc)
	svc.items = NewItemManager(svc)

	if err := svc.Start(); err != nil {
		cleanup()
		t.Fatalf("start service: %v", err)
	}

	return svc, ms, func() {
		_ = svc.Stop()
		cleanup()
	}
}

func TestDeleteCollection_RemovesFromMap(t *testing.T) {
	svc, _, cleanup := newTestService(t)
	defer cleanup()

	// Create a collection
	props := map[string]dbus.Variant{
		"org.freedesktop.Secret.Collection.Label": dbus.MakeVariant("test-coll"),
	}
	path, _, dbusErr := svc.CreateCollection(props, "testcoll")
	if dbusErr != nil {
		t.Fatalf("CreateCollection: %v", dbusErr)
	}
	if path == "/" {
		t.Fatal("CreateCollection returned /")
	}

	// Verify it exists in the map
	if _, ok := svc.collections.Get("testcoll"); !ok {
		t.Fatal("collection not in map after creation")
	}

	// Delete via the Collection object
	coll, ok := svc.collections.Get("testcoll")
	if !ok {
		t.Fatal("collection not found")
	}
	prompt, dbusErr := coll.Delete()
	if dbusErr != nil {
		t.Fatalf("Delete: %v", dbusErr)
	}
	if prompt != "/" {
		t.Fatalf("Delete returned prompt %s, want /", prompt)
	}

	// Verify removed from map
	if _, ok := svc.collections.Get("testcoll"); ok {
		t.Error("collection still in map after Delete")
	}
}

func TestDeleteCollection_AllowsRecreation(t *testing.T) {
	svc, _, cleanup := newTestService(t)
	defer cleanup()

	props := map[string]dbus.Variant{
		"org.freedesktop.Secret.Collection.Label": dbus.MakeVariant("recreate-me"),
	}

	// Create
	path1, _, dbusErr := svc.CreateCollection(props, "recreate")
	if dbusErr != nil {
		t.Fatalf("first CreateCollection: %v", dbusErr)
	}

	// Delete
	coll, _ := svc.collections.Get("recreate")
	if _, dbusErr := coll.Delete(); dbusErr != nil {
		t.Fatalf("Delete: %v", dbusErr)
	}

	// Re-create with same name — this was the original bug
	path2, _, dbusErr := svc.CreateCollection(props, "recreate")
	if dbusErr != nil {
		t.Fatalf("second CreateCollection (re-create) failed: %v", dbusErr)
	}
	if path2 == "/" {
		t.Fatal("re-creation returned /")
	}
	if path2 != path1 {
		t.Logf("paths differ: %s vs %s (expected, same name)", path1, path2)
	}
}

func TestDeleteCollection_UpdatesCollectionsProperty(t *testing.T) {
	svc, _, cleanup := newTestService(t)
	defer cleanup()

	props := map[string]dbus.Variant{
		"org.freedesktop.Secret.Collection.Label": dbus.MakeVariant("prop-test"),
	}
	collPath, _, dbusErr := svc.CreateCollection(props, "proptest")
	if dbusErr != nil {
		t.Fatalf("CreateCollection: %v", dbusErr)
	}

	// Read Collections property — should contain our collection
	v, err := svc.props.Get(dbtypes.SecretServiceInterface, "Collections")
	if err != nil {
		t.Fatalf("get Collections property: %v", err)
	}
	paths := v.Value().([]dbus.ObjectPath)
	if !containsPath(paths, collPath) {
		t.Fatalf("Collections property %v does not contain %s", paths, collPath)
	}

	// Delete the collection
	coll, _ := svc.collections.Get("proptest")
	if _, dbusErr := coll.Delete(); dbusErr != nil {
		t.Fatalf("Delete: %v", dbusErr)
	}

	// Collections property should no longer contain it
	v, err = svc.props.Get(dbtypes.SecretServiceInterface, "Collections")
	if err != nil {
		t.Fatalf("get Collections property after delete: %v", err)
	}
	paths = v.Value().([]dbus.ObjectPath)
	if containsPath(paths, collPath) {
		t.Errorf("Collections property %v still contains deleted collection %s", paths, collPath)
	}
}

func TestUnlock_AliasPath(t *testing.T) {
	svc, ms, cleanup := newTestService(t)
	defer cleanup()

	// Set up alias "default" -> "login"
	ms.mu.Lock()
	ms.aliases["default"] = "login"
	ms.collections["login"] = &store.CollectionData{Name: "login", Label: "Login"}
	ms.mu.Unlock()

	// Unlock using alias path (what go-keyring sends)
	aliasPath := dbtypes.AliasPath("default")
	unlocked, prompt, dbusErr := svc.Unlock([]dbus.ObjectPath{aliasPath})
	if dbusErr != nil {
		t.Fatalf("Unlock: %v", dbusErr)
	}
	if prompt != "/" {
		t.Fatalf("Unlock returned prompt %s, want /", prompt)
	}
	if len(unlocked) != 1 {
		t.Fatalf("Unlock returned %d unlocked objects, want 1", len(unlocked))
	}
	if unlocked[0] != aliasPath {
		t.Errorf("Unlock returned %s, want %s", unlocked[0], aliasPath)
	}
}

func TestUnlock_CollectionPath(t *testing.T) {
	svc, ms, cleanup := newTestService(t)
	defer cleanup()

	ms.mu.Lock()
	ms.collections["login"] = &store.CollectionData{Name: "login", Label: "Login"}
	ms.mu.Unlock()

	// Unlock using direct collection path
	collPath := dbtypes.CollectionPath("login")
	unlocked, _, dbusErr := svc.Unlock([]dbus.ObjectPath{collPath})
	if dbusErr != nil {
		t.Fatalf("Unlock: %v", dbusErr)
	}
	if len(unlocked) != 1 {
		t.Fatalf("Unlock returned %d unlocked objects, want 1", len(unlocked))
	}
	if unlocked[0] != collPath {
		t.Errorf("Unlock returned %s, want %s", unlocked[0], collPath)
	}
}

func TestLock_AliasPath(t *testing.T) {
	svc, ms, cleanup := newTestService(t)
	defer cleanup()

	ms.mu.Lock()
	ms.aliases["default"] = "login"
	ms.collections["login"] = &store.CollectionData{Name: "login", Label: "Login"}
	ms.mu.Unlock()

	// Lock using alias path
	aliasPath := dbtypes.AliasPath("default")
	locked, prompt, dbusErr := svc.Lock([]dbus.ObjectPath{aliasPath})
	if dbusErr != nil {
		t.Fatalf("Lock: %v", dbusErr)
	}
	if prompt != "/" {
		t.Fatalf("Lock returned prompt %s, want /", prompt)
	}
	if len(locked) != 1 {
		t.Fatalf("Lock returned %d locked objects, want 1", len(locked))
	}
	if locked[0] != aliasPath {
		t.Errorf("Lock returned %s, want %s", locked[0], aliasPath)
	}
}

func TestUnlock_ItemPath(t *testing.T) {
	svc, ms, cleanup := newTestService(t)
	defer cleanup()

	ms.mu.Lock()
	ms.collections["default"] = &store.CollectionData{Name: "default", Label: "Default"}
	ms.mu.Unlock()

	// Unlock using item path (what gh does after CreateItem)
	itemPath := dbtypes.ItemPath("default", "i52f9c2333e2246e1bd6e533333f68788")
	unlocked, prompt, dbusErr := svc.Unlock([]dbus.ObjectPath{itemPath})
	if dbusErr != nil {
		t.Fatalf("Unlock: %v", dbusErr)
	}
	if prompt != "/" {
		t.Fatalf("Unlock returned prompt %s, want /", prompt)
	}
	if len(unlocked) != 1 {
		t.Fatalf("Unlock returned %d unlocked objects, want 1", len(unlocked))
	}
	if unlocked[0] != itemPath {
		t.Errorf("Unlock returned %s, want %s", unlocked[0], itemPath)
	}
}

func TestItemProperties_ReflectStoreData(t *testing.T) {
	svc, ms, cleanup := newTestService(t)
	defer cleanup()

	// Pre-populate the store with a collection and item
	ms.mu.Lock()
	ms.collections["default"] = &store.CollectionData{Name: "default", Label: "Default"}
	ms.items["default"] = map[string]*store.ItemData{
		"i52f9c2333e2246e1bd6e533333f68788": {
			ID:    "i52f9c2333e2246e1bd6e533333f68788",
			Label: "My Secret",
			Attributes: map[string]string{
				"service": "google-workspace",
				"user":    "alice@example.com",
			},
			Created:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Modified: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	ms.mu.Unlock()

	// Directly export the item (simulates what ExportAllItems does)
	_, err := svc.items.GetOrCreate("default", "i52f9c2333e2246e1bd6e533333f68788")
	if err != nil {
		t.Fatalf("GetOrCreate item: %v", err)
	}

	// Read D-Bus properties via the connection
	itemPath := dbtypes.ItemPath("default", "i52f9c2333e2246e1bd6e533333f68788")
	obj := svc.conn.Object("org.freedesktop.secrets", itemPath)

	variant, err := obj.GetProperty(dbtypes.ItemInterface + ".Label")
	if err != nil {
		t.Fatalf("GetProperty Label: %v", err)
	}
	label, ok := variant.Value().(string)
	if !ok {
		t.Fatalf("Label is not a string: %T", variant.Value())
	}
	if label != "My Secret" {
		t.Errorf("Label = %q, want %q", label, "My Secret")
	}

	variant, err = obj.GetProperty(dbtypes.ItemInterface + ".Attributes")
	if err != nil {
		t.Fatalf("GetProperty Attributes: %v", err)
	}
	attrs, ok := variant.Value().(map[string]string)
	if !ok {
		t.Fatalf("Attributes is not map[string]string: %T", variant.Value())
	}
	if attrs["service"] != "google-workspace" {
		t.Errorf("Attributes[service] = %q, want %q", attrs["service"], "google-workspace")
	}
	if attrs["user"] != "alice@example.com" {
		t.Errorf("Attributes[user] = %q, want %q", attrs["user"], "alice@example.com")
	}
}

func TestItemProperties_EmptyWhenStoreFailsDuringExport(t *testing.T) {
	svc, ms, cleanup := newTestService(t)
	defer cleanup()

	// Store has NO item data yet — simulates gpg-agent not ready at startup
	ms.mu.Lock()
	ms.collections["default"] = &store.CollectionData{Name: "default", Label: "Default"}
	ms.mu.Unlock()

	// Export item — refreshProperties will call GetItem which returns "not found"
	_, err := svc.items.GetOrCreate("default", "i52f9c2333e2246e1bd6e533333f68788")
	if err != nil {
		t.Fatalf("GetOrCreate item: %v", err)
	}

	// NOW add the item to the store (simulates gpg-agent becoming available)
	ms.mu.Lock()
	ms.items["default"] = map[string]*store.ItemData{
		"i52f9c2333e2246e1bd6e533333f68788": {
			ID:    "i52f9c2333e2246e1bd6e533333f68788",
			Label: "My Secret",
			Attributes: map[string]string{
				"service": "google-workspace",
				"user":    "alice@example.com",
			},
			Created:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Modified: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	ms.mu.Unlock()

	// Read D-Bus properties — they should reflect the store, not the stale empty defaults
	itemPath := dbtypes.ItemPath("default", "i52f9c2333e2246e1bd6e533333f68788")
	obj := svc.conn.Object("org.freedesktop.secrets", itemPath)

	variant, err := obj.GetProperty(dbtypes.ItemInterface + ".Label")
	if err != nil {
		t.Fatalf("GetProperty Label: %v", err)
	}
	label, ok := variant.Value().(string)
	if !ok {
		t.Fatalf("Label is not a string: %T", variant.Value())
	}
	if label != "My Secret" {
		t.Errorf("Label = %q, want %q", label, "My Secret")
	}

	variant, err = obj.GetProperty(dbtypes.ItemInterface + ".Attributes")
	if err != nil {
		t.Fatalf("GetProperty Attributes: %v", err)
	}
	attrs, ok := variant.Value().(map[string]string)
	if !ok {
		t.Fatalf("Attributes is not map[string]string: %T", variant.Value())
	}
	if attrs["service"] != "google-workspace" {
		t.Errorf("Attributes[service] = %q, want %q", attrs["service"], "google-workspace")
	}
	if attrs["user"] != "alice@example.com" {
		t.Errorf("Attributes[user] = %q, want %q", attrs["user"], "alice@example.com")
	}
}

func containsPath(paths []dbus.ObjectPath, target dbus.ObjectPath) bool {
	for _, p := range paths {
		if p == target {
			return true
		}
	}
	return false
}

// openPlainSession opens a "plain" (no-encryption) session over D-Bus and
// returns its path. Test helper used by D-Bus-level regression tests.
func openPlainSession(t *testing.T, svc *Service) dbus.ObjectPath {
	t.Helper()
	svcObj := svc.conn.Object("org.freedesktop.secrets", dbtypes.ServicePath)
	var output dbus.Variant
	var sessionPath dbus.ObjectPath
	if err := svcObj.Call(dbtypes.SecretServiceInterface+".OpenSession", 0,
		"plain", dbus.MakeVariant("")).Store(&output, &sessionPath); err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	return sessionPath
}

// TestSearchItems_ExportsResultsOverDBus is the regression test for the bug
// where SearchItems returned paths derived from the on-disk store but did NOT
// register an Item proxy at those paths. Any subsequent Item.GetSecret call
// then failed with "Object does not implement the interface
// 'org.freedesktop.Secret.Item'" because the ItemManager's in-memory set —
// the source of D-Bus exports — was disjoint from the store after a service
// restart, an external write, or a CreateItem(replace=true).
func TestSearchItems_ExportsResultsOverDBus(t *testing.T) {
	svc, ms, cleanup := newTestService(t)
	defer cleanup()

	const itemID = "i775990bb890547499e8234803760a350"
	ms.mu.Lock()
	ms.collections["default"] = &store.CollectionData{Name: "default", Label: "Default"}
	ms.items["default"] = map[string]*store.ItemData{
		itemID: {
			ID:          itemID,
			Label:       "kubelogin token cache",
			Secret:      []byte("eyJhbGc-fake-token"),
			ContentType: "text/plain",
			Attributes: map[string]string{
				"service":  "kubelogin",
				"username": "kubelogin/tokencache/abc",
			},
		},
	}
	ms.mu.Unlock()

	// Deliberately DO NOT call svc.items.GetOrCreate. This is the
	// post-restart / external-write state: the store has the item, but the
	// ItemManager does not. The fix in SearchItems must close that gap.

	sessionPath := openPlainSession(t, svc)
	svcObj := svc.conn.Object("org.freedesktop.secrets", dbtypes.ServicePath)

	// Service.SearchItems — discover the path.
	var unlocked, locked []dbus.ObjectPath
	if err := svcObj.Call(dbtypes.SecretServiceInterface+".SearchItems", 0,
		map[string]string{"service": "kubelogin"}).Store(&unlocked, &locked); err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	all := append(append([]dbus.ObjectPath{}, unlocked...), locked...)
	wantPath := dbtypes.ItemPath("default", itemID)
	if !containsPath(all, wantPath) {
		t.Fatalf("SearchItems result %v does not contain %s", all, wantPath)
	}

	// Item.GetSecret over D-Bus — the failing call from the original bug.
	// Without the fix this returns
	//   org.freedesktop.DBus.Error.UnknownInterface: Object does not implement
	//   the interface 'org.freedesktop.Secret.Item'
	itemObj := svc.conn.Object("org.freedesktop.secrets", wantPath)
	var secret dbtypes.Secret
	if err := itemObj.Call(dbtypes.ItemInterface+".GetSecret", 0, sessionPath).Store(&secret); err != nil {
		t.Fatalf("Item.GetSecret over D-Bus: %v\n(the item path was returned by SearchItems but never bound to its D-Bus interface — bug regressed)", err)
	}

	if string(secret.Value) != "eyJhbGc-fake-token" {
		t.Errorf("secret.Value = %q, want %q", string(secret.Value), "eyJhbGc-fake-token")
	}
	if secret.ContentType != "text/plain" {
		t.Errorf("secret.ContentType = %q, want %q", secret.ContentType, "text/plain")
	}
}

// TestCreateItem_ReplaceTrueExportsExistingItem covers the original kubelogin
// failure path: a client calls CreateItem(replace=true) for attributes that
// match an item already in the store (created earlier and persisted via _meta,
// but with the ItemManager empty after a service restart). The replace=true
// branch in Collection.CreateItem updates the store and, before the fix, did
// NOT register an Item proxy at the returned path — so the very next
// Item.GetSecret would fail with UnknownInterface.
func TestCreateItem_ReplaceTrueExportsExistingItem(t *testing.T) {
	svc, ms, cleanup := newTestService(t)
	defer cleanup()

	const itemID = "i333333333333333333333333cccccccc"
	const oldSecret = "old-token"
	ms.mu.Lock()
	ms.collections["default"] = &store.CollectionData{Name: "default", Label: "Default"}
	ms.items["default"] = map[string]*store.ItemData{
		itemID: {
			ID:          itemID,
			Label:       "kubelogin token cache",
			Secret:      []byte(oldSecret),
			ContentType: "text/plain",
			Attributes: map[string]string{
				"service":  "kubelogin",
				"username": "kubelogin/tokencache/abc",
			},
		},
	}
	ms.mu.Unlock()

	sessionPath := openPlainSession(t, svc)

	// CreateItem(replace=true) with matching attributes → hits the
	// existingItem-with-replace branch in Collection.CreateItem.
	const newSecret = "new-rotated-token"
	collObj := svc.conn.Object("org.freedesktop.secrets", dbtypes.CollectionPath("default"))
	properties := map[string]dbus.Variant{
		"org.freedesktop.Secret.Item.Label": dbus.MakeVariant("kubelogin token cache"),
		"org.freedesktop.Secret.Item.Attributes": dbus.MakeVariant(map[string]string{
			"service":  "kubelogin",
			"username": "kubelogin/tokencache/abc",
		}),
	}
	secretIn := dbtypes.Secret{
		Session:     sessionPath,
		Parameters:  []byte{},
		Value:       []byte(newSecret),
		ContentType: "text/plain",
	}
	var gotPath, promptPath dbus.ObjectPath
	if err := collObj.Call(dbtypes.CollectionInterface+".CreateItem", 0,
		properties, secretIn, true).Store(&gotPath, &promptPath); err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	wantPath := dbtypes.ItemPath("default", itemID)
	if gotPath != wantPath {
		t.Fatalf("CreateItem returned %s, want %s", gotPath, wantPath)
	}

	// Without the fix this returns
	// "Object does not implement the interface 'org.freedesktop.Secret.Item'".
	itemObj := svc.conn.Object("org.freedesktop.secrets", gotPath)
	var secret dbtypes.Secret
	if err := itemObj.Call(dbtypes.ItemInterface+".GetSecret", 0, sessionPath).Store(&secret); err != nil {
		t.Fatalf("Item.GetSecret over D-Bus after CreateItem(replace=true): %v\n(replace branch did not export the item — bug regressed)", err)
	}
	if string(secret.Value) != newSecret {
		t.Errorf("secret.Value = %q, want %q (rotated value should be returned)", string(secret.Value), newSecret)
	}
}

// TestCollectionItemsProperty_LiveReadsStore covers the cached-property bug:
// Collection.Items used to be set from prop.Export at construction time and
// only refreshed on add/delete events, so items added to the store *between*
// service start and the property read (e.g. external `gopass insert`, or any
// item not picked up by ExportAllItems) were invisible. With the live
// collectionPropsHandler the property reads from the store on every Get.
func TestCollectionItemsProperty_LiveReadsStore(t *testing.T) {
	svc, ms, cleanup := newTestService(t)
	defer cleanup()

	// newTestService starts the service before any items exist in the store.
	// Now add one externally, the way an external CLI or a pre-existing
	// on-disk file would surface.
	const itemID = "i222222222222222222222222bbbbbbbb"
	ms.mu.Lock()
	ms.collections["default"] = &store.CollectionData{Name: "default", Label: "Default"}
	ms.items["default"] = map[string]*store.ItemData{
		itemID: {
			ID:          itemID,
			Label:       "props secret",
			Secret:      []byte("v"),
			ContentType: "text/plain",
			Attributes:  map[string]string{},
		},
	}
	ms.mu.Unlock()

	collObj := svc.conn.Object("org.freedesktop.secrets", dbtypes.CollectionPath("default"))
	v, err := collObj.GetProperty(dbtypes.CollectionInterface + ".Items")
	if err != nil {
		t.Fatalf("GetProperty Items: %v", err)
	}
	paths, ok := v.Value().([]dbus.ObjectPath)
	if !ok {
		t.Fatalf("Items is not []ObjectPath: %T", v.Value())
	}
	wantPath := dbtypes.ItemPath("default", itemID)
	if !containsPath(paths, wantPath) {
		t.Fatalf("Items property %v does not contain %s\n(property was cached at Export and never refreshed — stale-cache bug regressed)", paths, wantPath)
	}
}

// TestCollectionLabelProperty_LiveReadsStore guards the same staleness fix
// for the Label property: if the store mutates outside of D-Bus, Label must
// reflect the new value on the next read.
func TestCollectionLabelProperty_LiveReadsStore(t *testing.T) {
	svc, ms, cleanup := newTestService(t)
	defer cleanup()

	// Service starts with the default collection auto-created with empty
	// label data. Mutate the label externally.
	ms.mu.Lock()
	ms.collections["default"] = &store.CollectionData{Name: "default", Label: "Updated Externally"}
	ms.mu.Unlock()

	collObj := svc.conn.Object("org.freedesktop.secrets", dbtypes.CollectionPath("default"))
	v, err := collObj.GetProperty(dbtypes.CollectionInterface + ".Label")
	if err != nil {
		t.Fatalf("GetProperty Label: %v", err)
	}
	label, ok := v.Value().(string)
	if !ok {
		t.Fatalf("Label is not a string: %T", v.Value())
	}
	if label != "Updated Externally" {
		t.Errorf("Label = %q, want %q (cached pre-mutation value returned — stale-cache bug regressed)", label, "Updated Externally")
	}
}

// TestCollectionSearchItems_ExportsResultsOverDBus covers the same regression
// at the Collection.SearchItems entry point — the per-collection variant, used
// by clients that have already located a collection and are searching within
// it.
func TestCollectionSearchItems_ExportsResultsOverDBus(t *testing.T) {
	svc, ms, cleanup := newTestService(t)
	defer cleanup()

	const itemID = "i111111111111111111111111aaaaaaaa"
	ms.mu.Lock()
	ms.collections["default"] = &store.CollectionData{Name: "default", Label: "Default"}
	ms.items["default"] = map[string]*store.ItemData{
		itemID: {
			ID:          itemID,
			Label:       "secret",
			Secret:      []byte("value"),
			ContentType: "text/plain",
			Attributes:  map[string]string{"app": "x"},
		},
	}
	ms.mu.Unlock()

	sessionPath := openPlainSession(t, svc)

	collObj := svc.conn.Object("org.freedesktop.secrets", dbtypes.CollectionPath("default"))
	var paths []dbus.ObjectPath
	if err := collObj.Call(dbtypes.CollectionInterface+".SearchItems", 0,
		map[string]string{"app": "x"}).Store(&paths); err != nil {
		t.Fatalf("Collection.SearchItems: %v", err)
	}
	wantPath := dbtypes.ItemPath("default", itemID)
	if !containsPath(paths, wantPath) {
		t.Fatalf("Collection.SearchItems result %v does not contain %s", paths, wantPath)
	}

	itemObj := svc.conn.Object("org.freedesktop.secrets", wantPath)
	var secret dbtypes.Secret
	if err := itemObj.Call(dbtypes.ItemInterface+".GetSecret", 0, sessionPath).Store(&secret); err != nil {
		t.Fatalf("Item.GetSecret over D-Bus after Collection.SearchItems: %v", err)
	}
	if string(secret.Value) != "value" {
		t.Errorf("secret.Value = %q, want %q", string(secret.Value), "value")
	}
}

// TestCollectionItemsProperty_ExportsAllItems exercises the third leak site:
// the Items property accessor on a collection returns paths from the store,
// so clients that iterate the property and then call Item.GetSecret on each
// path must find every entry exported on D-Bus.
func TestCollectionItemsProperty_ExportsAllItems(t *testing.T) {
	svc, ms, cleanup := newTestService(t)
	defer cleanup()

	const itemID = "i222222222222222222222222bbbbbbbb"
	ms.mu.Lock()
	ms.collections["default"] = &store.CollectionData{Name: "default", Label: "Default"}
	ms.items["default"] = map[string]*store.ItemData{
		itemID: {
			ID:          itemID,
			Label:       "props secret",
			Secret:      []byte("v"),
			ContentType: "text/plain",
			Attributes:  map[string]string{},
		},
	}
	ms.mu.Unlock()

	sessionPath := openPlainSession(t, svc)

	collObj := svc.conn.Object("org.freedesktop.secrets", dbtypes.CollectionPath("default"))
	v, err := collObj.GetProperty(dbtypes.CollectionInterface + ".Items")
	if err != nil {
		t.Fatalf("GetProperty Items: %v", err)
	}
	paths, ok := v.Value().([]dbus.ObjectPath)
	if !ok {
		t.Fatalf("Items is not []ObjectPath: %T", v.Value())
	}
	wantPath := dbtypes.ItemPath("default", itemID)
	if !containsPath(paths, wantPath) {
		t.Fatalf("Items property %v does not contain %s", paths, wantPath)
	}

	itemObj := svc.conn.Object("org.freedesktop.secrets", wantPath)
	var secret dbtypes.Secret
	if err := itemObj.Call(dbtypes.ItemInterface+".GetSecret", 0, sessionPath).Store(&secret); err != nil {
		t.Fatalf("Item.GetSecret over D-Bus after reading Items property: %v", err)
	}
}
