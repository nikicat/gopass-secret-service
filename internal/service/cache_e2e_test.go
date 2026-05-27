package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/gopasspw/gopass/pkg/gopass"
	"github.com/gopasspw/gopass/pkg/gopass/secrets"

	"github.com/nikicat/gopass-secret-service/internal/config"
	dbtypes "github.com/nikicat/gopass-secret-service/internal/dbus"
	"github.com/nikicat/gopass-secret-service/internal/store"
)

// countingBackend is a hermetic gopass.Store that records how many times each
// path is decrypted (Get), letting the e2e assert the metadata cache avoids
// re-decryption. It never invokes GPG.
type countingBackend struct {
	mu       sync.Mutex
	data     map[string]gopass.Secret
	getCount map[string]int
}

func newCountingBackend() *countingBackend {
	return &countingBackend{
		data:     make(map[string]gopass.Secret),
		getCount: make(map[string]int),
	}
}

func (b *countingBackend) decryptions(path string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.getCount[path]
}

func (b *countingBackend) String() string { return "countingBackend" }

func (b *countingBackend) List(ctx context.Context) ([]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, 0, len(b.data))
	for k := range b.data {
		out = append(out, k)
	}
	return out, nil
}

func (b *countingBackend) Get(ctx context.Context, name, revision string) (gopass.Secret, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.getCount[name]++
	sec, ok := b.data[name]
	if !ok {
		return nil, fmt.Errorf("not found: %s", name)
	}
	return sec, nil
}

func (b *countingBackend) Set(ctx context.Context, name string, by gopass.Byter) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	// GopassStore always passes a *secrets.AKV (also a gopass.Secret), so we can
	// store it verbatim and serve its metadata back on Get.
	if sec, ok := by.(gopass.Secret); ok {
		b.data[name] = sec
		return nil
	}
	sec := secrets.New()
	sec.SetPassword(string(by.Bytes()))
	b.data[name] = sec
	return nil
}

func (b *countingBackend) Revisions(ctx context.Context, name string) ([]string, error) {
	return []string{"latest"}, nil
}

func (b *countingBackend) Remove(ctx context.Context, name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.data, name)
	return nil
}

func (b *countingBackend) RemoveAll(ctx context.Context, prefix string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for k := range b.data {
		if k == prefix || strings.HasPrefix(k, prefix+"/") {
			delete(b.data, k)
		}
	}
	return nil
}

func (b *countingBackend) Rename(ctx context.Context, src, dest string) error { return nil }
func (b *countingBackend) Sync(ctx context.Context) error                     { return nil }
func (b *countingBackend) Close(ctx context.Context) error                    { return nil }

// TestE2E_CacheLifecycleOverDBus drives the full store→cache→modify→retrieve
// lifecycle through the real D-Bus Secret Service stack, backed by a gopass
// store whose decryptions are counted:
//
//  1. store a secret,
//  2. look it up (populates the metadata cache),
//  3. look it up again and assert the backend was NOT decrypted (cache works),
//  4. read the secret value over D-Bus (== v1),
//  5. modify it via CreateItem(replace=true),
//  6. read the secret value again over D-Bus and assert it is the new value
//     (cache invalidated end to end).
func TestE2E_CacheLifecycleOverDBus(t *testing.T) {
	ctx := context.Background()
	conn, cleanup := startTestBus(t)
	defer cleanup()

	backend := newCountingBackend()
	gs := store.NewGopassStoreWithBackend(backend, "test")

	// 1. Store a secret through the store's own write path.
	id, err := gs.CreateItem(ctx, "default", &store.ItemData{
		Label:       "etherscan",
		Secret:      []byte("value-v1"),
		ContentType: "text/plain",
		Attributes:  map[string]string{"service": "etherscan.io"},
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	itemPath := dbtypes.ItemPath("default", id)
	storePath := "test/default/" + id

	svc := &Service{
		conn:  conn,
		store: gs,
		cfg:   &config.Config{DefaultCollection: "default", Prefix: "test", Replace: true},
	}
	svc.sessions = NewSessionManager(conn)
	svc.prompts = NewPromptManager(conn)
	svc.collections = NewCollectionManager(svc)
	svc.items = NewItemManager(svc)
	if err := svc.Start(); err != nil {
		t.Fatalf("start service: %v", err)
	}
	defer func() { _ = svc.Stop() }()

	svcObj := conn.Object("org.freedesktop.secrets", dbtypes.ServicePath)
	search := func() []dbus.ObjectPath {
		t.Helper()
		var unlocked, locked []dbus.ObjectPath
		if err := svcObj.Call(dbtypes.SecretServiceInterface+".SearchItems", 0,
			map[string]string{"service": "etherscan.io"}).Store(&unlocked, &locked); err != nil {
			t.Fatalf("SearchItems: %v", err)
		}
		return append(unlocked, locked...)
	}

	// 2. First lookup over D-Bus warms the metadata cache.
	if got := search(); !containsPath(got, itemPath) {
		t.Fatalf("first SearchItems %v missing %s", got, itemPath)
	}
	afterWarm := backend.decryptions(storePath)

	// 3. Second lookup must be served from cache — backend NOT decrypted again.
	if got := search(); !containsPath(got, itemPath) {
		t.Fatalf("second SearchItems %v missing %s", got, itemPath)
	}
	if n := backend.decryptions(storePath); n != afterWarm {
		t.Fatalf("second lookup decrypted the backend: %d -> %d (expected cache hit)", afterWarm, n)
	}

	// 4. Read the secret value over D-Bus.
	session := openPlainSession(t, svc)
	itemObj := conn.Object("org.freedesktop.secrets", itemPath)
	var secret dbtypes.Secret
	if err := itemObj.Call(dbtypes.ItemInterface+".GetSecret", 0, session).Store(&secret); err != nil {
		t.Fatalf("Item.GetSecret (v1): %v", err)
	}
	if string(secret.Value) != "value-v1" {
		t.Fatalf("GetSecret v1 = %q, want value-v1", secret.Value)
	}

	// 5. Modify the secret via CreateItem(replace=true) with matching attributes.
	collObj := conn.Object("org.freedesktop.secrets", dbtypes.CollectionPath("default"))
	properties := map[string]dbus.Variant{
		"org.freedesktop.Secret.Item.Label":      dbus.MakeVariant("etherscan"),
		"org.freedesktop.Secret.Item.Attributes": dbus.MakeVariant(map[string]string{"service": "etherscan.io"}),
	}
	newSecret := dbtypes.Secret{
		Session:     session,
		Parameters:  []byte{},
		Value:       []byte("value-v2"),
		ContentType: "text/plain",
	}
	var gotPath, promptPath dbus.ObjectPath
	if err := collObj.Call(dbtypes.CollectionInterface+".CreateItem", 0,
		properties, newSecret, true).Store(&gotPath, &promptPath); err != nil {
		t.Fatalf("CreateItem(replace=true): %v", err)
	}

	// 6. Reading again must return the modified value — no stale cache.
	if err := itemObj.Call(dbtypes.ItemInterface+".GetSecret", 0, session).Store(&secret); err != nil {
		t.Fatalf("Item.GetSecret (v2): %v", err)
	}
	if string(secret.Value) != "value-v2" {
		t.Fatalf("GetSecret after modify = %q, want value-v2 (stale cache through the D-Bus stack)", secret.Value)
	}
}
