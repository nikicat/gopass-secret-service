// Package store, multi.go: a Store implementation that routes operations
// between a primary (durable) store and a session (volatile) store.
package store

import (
	"context"
	"fmt"
)

// MultiStore implements Store by delegating to one of two underlying stores
// based on the collection name. Calls referencing SessionCollectionName go to
// the session store; everything else goes to the primary. Operations that
// don't reference a collection (most alias operations, Close) are handled by
// fan-out or by the primary alone.
type MultiStore struct {
	Primary Store
	Session Store
}

// NewMultiStore returns a MultiStore that routes the session collection to
// session and everything else to primary.
func NewMultiStore(primary, session Store) *MultiStore {
	return &MultiStore{Primary: primary, Session: session}
}

func (m *MultiStore) routeByCollection(name string) Store {
	if name == SessionCollectionName {
		return m.Session
	}
	return m.Primary
}

func (m *MultiStore) Collections(ctx context.Context) ([]string, error) {
	primary, err := m.Primary.Collections(ctx)
	if err != nil {
		return nil, err
	}
	// Surface the session collection as a peer of primary collections so
	// Service.Collections includes it without special-casing.
	for _, name := range primary {
		if name == SessionCollectionName {
			return primary, nil
		}
	}
	return append(primary, SessionCollectionName), nil
}

func (m *MultiStore) GetCollection(ctx context.Context, name string) (*CollectionData, error) {
	return m.routeByCollection(name).GetCollection(ctx, name)
}

func (m *MultiStore) CreateCollection(ctx context.Context, name, label string) error {
	return m.routeByCollection(name).CreateCollection(ctx, name, label)
}

func (m *MultiStore) DeleteCollection(ctx context.Context, name string) error {
	return m.routeByCollection(name).DeleteCollection(ctx, name)
}

func (m *MultiStore) SetCollectionLabel(ctx context.Context, name, label string) error {
	return m.routeByCollection(name).SetCollectionLabel(ctx, name, label)
}

func (m *MultiStore) Items(ctx context.Context, collection string) ([]string, error) {
	return m.routeByCollection(collection).Items(ctx, collection)
}

func (m *MultiStore) GetItem(ctx context.Context, collection, id string) (*ItemData, error) {
	return m.routeByCollection(collection).GetItem(ctx, collection, id)
}

func (m *MultiStore) CreateItem(ctx context.Context, collection string, item *ItemData) (string, error) {
	return m.routeByCollection(collection).CreateItem(ctx, collection, item)
}

func (m *MultiStore) UpdateItem(ctx context.Context, collection, id string, item *ItemData) error {
	return m.routeByCollection(collection).UpdateItem(ctx, collection, id, item)
}

func (m *MultiStore) DeleteItem(ctx context.Context, collection, id string) error {
	return m.routeByCollection(collection).DeleteItem(ctx, collection, id)
}

func (m *MultiStore) SearchItems(ctx context.Context, collection string, attributes map[string]string) ([]*ItemData, error) {
	return m.routeByCollection(collection).SearchItems(ctx, collection, attributes)
}

// SearchAllItems fans out across both stores. A failure in either is logged
// upstream by returning the first error; partial results from the other store
// are dropped to avoid surprising callers with half a result set.
func (m *MultiStore) SearchAllItems(ctx context.Context, attributes map[string]string) (map[string][]*ItemData, error) {
	primary, err := m.Primary.SearchAllItems(ctx, attributes)
	if err != nil {
		return nil, err
	}
	session, err := m.Session.SearchAllItems(ctx, attributes)
	if err != nil {
		return nil, err
	}
	if primary == nil {
		primary = map[string][]*ItemData{}
	}
	for k, v := range session {
		primary[k] = v
	}
	return primary, nil
}

func (m *MultiStore) LockCollection(ctx context.Context, name string) error {
	return m.routeByCollection(name).LockCollection(ctx, name)
}

func (m *MultiStore) UnlockCollection(ctx context.Context, name string) error {
	return m.routeByCollection(name).UnlockCollection(ctx, name)
}

// GetAlias resolves "session" locally; everything else goes to the primary.
// The session alias is hardcoded so callers can always reach the volatile
// collection by alias even if the alias table on the primary is missing.
func (m *MultiStore) GetAlias(ctx context.Context, alias string) (string, error) {
	if alias == "session" {
		return SessionCollectionName, nil
	}
	return m.Primary.GetAlias(ctx, alias)
}

// SetAlias rejects assigning the "session" alias (it's reserved) and forwards
// everything else to the primary.
func (m *MultiStore) SetAlias(ctx context.Context, alias, collection string) error {
	if alias == "session" {
		return fmt.Errorf("the 'session' alias is reserved for the volatile collection")
	}
	return m.Primary.SetAlias(ctx, alias, collection)
}

func (m *MultiStore) Close(ctx context.Context) error {
	primaryErr := m.Primary.Close(ctx)
	sessionErr := m.Session.Close(ctx)
	if primaryErr != nil {
		return primaryErr
	}
	return sessionErr
}
