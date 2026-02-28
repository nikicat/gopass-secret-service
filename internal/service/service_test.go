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
}

func newMockStore() *mockStore {
	return &mockStore{
		collections: make(map[string]*store.CollectionData),
		aliases:     make(map[string]string),
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

func (m *mockStore) Items(_ context.Context, _ string) ([]string, error)     { return nil, nil }
func (m *mockStore) GetItem(_ context.Context, _, _ string) (*store.ItemData, error) {
	return nil, fmt.Errorf("not found")
}
func (m *mockStore) CreateItem(_ context.Context, _ string, _ *store.ItemData) (string, error) {
	return "", nil
}
func (m *mockStore) UpdateItem(_ context.Context, _, _ string, _ *store.ItemData) error { return nil }
func (m *mockStore) DeleteItem(_ context.Context, _, _ string) error                    { return nil }
func (m *mockStore) SearchItems(_ context.Context, _ string, _ map[string]string) ([]*store.ItemData, error) {
	return nil, nil
}
func (m *mockStore) SearchAllItems(_ context.Context, _ map[string]string) (map[string][]*store.ItemData, error) {
	return nil, nil
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
		cmd.Process.Kill()
		t.Fatalf("dial bus: %v", err)
	}
	if err := conn.Auth(nil); err != nil {
		conn.Close()
		cmd.Process.Kill()
		t.Fatalf("auth: %v", err)
	}
	if err := conn.Hello(); err != nil {
		conn.Close()
		cmd.Process.Kill()
		t.Fatalf("hello: %v", err)
	}

	cleanup := func() {
		conn.Close()
		cmd.Process.Kill()
		cmd.Wait()
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
		svc.Stop()
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

func containsPath(paths []dbus.ObjectPath, target dbus.ObjectPath) bool {
	for _, p := range paths {
		if p == target {
			return true
		}
	}
	return false
}
