package service

import (
	"context"
	"sync"

	"github.com/godbus/dbus/v5"

	dbtypes "github.com/nikicat/gopass-secret-service/internal/dbus"
)

// Item represents a D-Bus Secret Service item
type Item struct {
	path       dbus.ObjectPath
	collection string
	id         string
	svc        *Service
	mu         sync.RWMutex
}

// NewItem creates a new Item instance
func NewItem(svc *Service, collection, id string) *Item {
	return &Item{
		path:       dbtypes.ItemPath(collection, id),
		collection: collection,
		id:         id,
		svc:        svc,
	}
}

// Path returns the item's D-Bus path
func (i *Item) Path() dbus.ObjectPath {
	return i.path
}

// itemPropsHandler implements org.freedesktop.DBus.Properties for items,
// reading from the store on every access instead of caching at export time.
type itemPropsHandler struct {
	item *Item
}

func (h *itemPropsHandler) Get(iface, property string) (dbus.Variant, *dbus.Error) {
	if iface != dbtypes.ItemInterface {
		return dbus.Variant{}, ErrUnsupported("unknown interface: " + iface)
	}

	ctx := context.Background()
	data, err := h.item.svc.store.GetItem(ctx, h.item.collection, h.item.id)
	if err != nil {
		// Return zero values if store is unavailable
		switch property {
		case "Label":
			return dbus.MakeVariant(""), nil
		case "Attributes":
			return dbus.MakeVariant(map[string]string{}), nil
		case "Locked":
			return dbus.MakeVariant(false), nil
		case "Created", "Modified":
			return dbus.MakeVariant(uint64(0)), nil
		default:
			return dbus.Variant{}, ErrUnsupported("unknown property: " + property)
		}
	}

	switch property {
	case "Label":
		return dbus.MakeVariant(data.Label), nil
	case "Attributes":
		attrs := data.Attributes
		if attrs == nil {
			attrs = map[string]string{}
		}
		return dbus.MakeVariant(attrs), nil
	case "Locked":
		return dbus.MakeVariant(data.Locked), nil
	case "Created":
		return dbus.MakeVariant(uint64(data.Created.Unix())), nil
	case "Modified":
		return dbus.MakeVariant(uint64(data.Modified.Unix())), nil
	default:
		return dbus.Variant{}, ErrUnsupported("unknown property: " + property)
	}
}

func (h *itemPropsHandler) GetAll(iface string) (map[string]dbus.Variant, *dbus.Error) {
	if iface != dbtypes.ItemInterface {
		return nil, ErrUnsupported("unknown interface: " + iface)
	}

	result := map[string]dbus.Variant{
		"Label":      dbus.MakeVariant(""),
		"Attributes": dbus.MakeVariant(map[string]string{}),
		"Locked":     dbus.MakeVariant(false),
		"Created":    dbus.MakeVariant(uint64(0)),
		"Modified":   dbus.MakeVariant(uint64(0)),
	}

	ctx := context.Background()
	data, err := h.item.svc.store.GetItem(ctx, h.item.collection, h.item.id)
	if err != nil {
		return result, nil
	}

	attrs := data.Attributes
	if attrs == nil {
		attrs = map[string]string{}
	}
	result["Label"] = dbus.MakeVariant(data.Label)
	result["Attributes"] = dbus.MakeVariant(attrs)
	result["Locked"] = dbus.MakeVariant(data.Locked)
	result["Created"] = dbus.MakeVariant(uint64(data.Created.Unix()))
	result["Modified"] = dbus.MakeVariant(uint64(data.Modified.Unix()))

	return result, nil
}

func (h *itemPropsHandler) Set(iface, property string, value dbus.Variant) *dbus.Error {
	if iface != dbtypes.ItemInterface {
		return ErrUnsupported("unknown interface: " + iface)
	}

	switch property {
	case "Label":
		label, ok := value.Value().(string)
		if !ok {
			return ErrUnsupported("invalid label type")
		}
		return h.item.setLabel(label)
	case "Attributes":
		attrs, ok := value.Value().(map[string]string)
		if !ok {
			return ErrUnsupported("invalid attributes type")
		}
		return h.item.setAttributes(attrs)
	default:
		return ErrUnsupported("property is read-only: " + property)
	}
}

// Export exports the item to D-Bus
func (i *Item) Export() error {
	conn := i.svc.conn

	// Export the item interface
	if err := conn.Export(i, i.path, dbtypes.ItemInterface); err != nil {
		return err
	}

	// Export a custom Properties handler that reads from the store on every access
	handler := &itemPropsHandler{item: i}
	if err := conn.Export(handler, i.path, "org.freedesktop.DBus.Properties"); err != nil {
		conn.Export(nil, i.path, dbtypes.ItemInterface)
		return err
	}

	// Export introspection - must include Properties interface for clients
	introXML := `<node>
  <interface name="org.freedesktop.DBus.Properties">
    <method name="Get">
      <arg name="interface" type="s" direction="in"/>
      <arg name="property" type="s" direction="in"/>
      <arg name="value" type="v" direction="out"/>
    </method>
    <method name="Set">
      <arg name="interface" type="s" direction="in"/>
      <arg name="property" type="s" direction="in"/>
      <arg name="value" type="v" direction="in"/>
    </method>
    <method name="GetAll">
      <arg name="interface" type="s" direction="in"/>
      <arg name="properties" type="a{sv}" direction="out"/>
    </method>
  </interface>
  <interface name="org.freedesktop.Secret.Item">
    <method name="Delete">
      <arg name="prompt" type="o" direction="out"/>
    </method>
    <method name="GetSecret">
      <arg name="session" type="o" direction="in"/>
      <arg name="secret" type="(oayays)" direction="out"/>
    </method>
    <method name="SetSecret">
      <arg name="secret" type="(oayays)" direction="in"/>
    </method>
    <property name="Locked" type="b" access="read"/>
    <property name="Attributes" type="a{ss}" access="readwrite"/>
    <property name="Label" type="s" access="readwrite"/>
    <property name="Created" type="t" access="read"/>
    <property name="Modified" type="t" access="read"/>
  </interface>
</node>`
	if err := conn.Export(introspect(introXML), i.path, "org.freedesktop.DBus.Introspectable"); err != nil {
		return err
	}

	return nil
}

// Unexport removes the item from D-Bus
func (i *Item) Unexport() {
	conn := i.svc.conn
	conn.Export(nil, i.path, dbtypes.ItemInterface)
	conn.Export(nil, i.path, "org.freedesktop.DBus.Properties")
	conn.Export(nil, i.path, "org.freedesktop.DBus.Introspectable")
}

// Delete implements org.freedesktop.Secret.Item.Delete
func (i *Item) Delete() (dbus.ObjectPath, *dbus.Error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	ctx := context.Background()
	if err := i.svc.store.DeleteItem(ctx, i.collection, i.id); err != nil {
		return "/", ErrObjectNotFound(err.Error())
	}

	// Remove from item manager (this also unexports)
	i.svc.items.Remove(i.path)

	// Update collection's Items property
	if coll, ok := i.svc.collections.Get(i.collection); ok {
		coll.refreshItems()
	}

	// Emit ItemDeleted signal
	i.svc.emitItemDeleted(i.collection, i.path)

	// Return "/" to indicate no prompt needed
	return "/", nil
}

// GetSecret implements org.freedesktop.Secret.Item.GetSecret
func (i *Item) GetSecret(sessionPath dbus.ObjectPath) (dbtypes.Secret, *dbus.Error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	session, ok := i.svc.sessions.GetSession(sessionPath)
	if !ok {
		return dbtypes.Secret{}, ErrSessionNotFound("session not found")
	}

	ctx := context.Background()
	item, err := i.svc.store.GetItem(ctx, i.collection, i.id)
	if err != nil {
		return dbtypes.Secret{}, ErrObjectNotFound(err.Error())
	}

	params, ciphertext, err := session.Encrypt(item.Secret)
	if err != nil {
		return dbtypes.Secret{}, ErrUnsupported(err.Error())
	}

	return dbtypes.Secret{
		Session:     sessionPath,
		Parameters:  params,
		Value:       ciphertext,
		ContentType: item.ContentType,
	}, nil
}

// SetSecret implements org.freedesktop.Secret.Item.SetSecret
func (i *Item) SetSecret(secret dbtypes.Secret) *dbus.Error {
	i.mu.Lock()
	defer i.mu.Unlock()

	session, ok := i.svc.sessions.GetSession(secret.Session)
	if !ok {
		return ErrSessionNotFound("session not found")
	}

	plaintext, err := session.Decrypt(secret.Parameters, secret.Value)
	if err != nil {
		return ErrUnsupported(err.Error())
	}

	ctx := context.Background()
	item, err := i.svc.store.GetItem(ctx, i.collection, i.id)
	if err != nil {
		return ErrObjectNotFound(err.Error())
	}

	item.Secret = plaintext
	item.ContentType = secret.ContentType

	if err := i.svc.store.UpdateItem(ctx, i.collection, i.id, item); err != nil {
		return ErrUnsupported(err.Error())
	}

	// Emit ItemChanged signal
	i.svc.emitItemChanged(i.collection, i.path)

	return nil
}

func (i *Item) setAttributes(attrs map[string]string) *dbus.Error {
	ctx := context.Background()
	item, err := i.svc.store.GetItem(ctx, i.collection, i.id)
	if err != nil {
		return ErrObjectNotFound(err.Error())
	}

	item.Attributes = attrs

	if err := i.svc.store.UpdateItem(ctx, i.collection, i.id, item); err != nil {
		return ErrUnsupported(err.Error())
	}

	i.svc.emitItemChanged(i.collection, i.path)
	return nil
}

func (i *Item) setLabel(label string) *dbus.Error {
	ctx := context.Background()
	item, err := i.svc.store.GetItem(ctx, i.collection, i.id)
	if err != nil {
		return ErrObjectNotFound(err.Error())
	}

	item.Label = label

	if err := i.svc.store.UpdateItem(ctx, i.collection, i.id, item); err != nil {
		return ErrUnsupported(err.Error())
	}

	i.svc.emitItemChanged(i.collection, i.path)
	return nil
}

// ItemManager manages items for the service
type ItemManager struct {
	items map[dbus.ObjectPath]*Item
	mu    sync.RWMutex
	svc   *Service
}

// NewItemManager creates a new item manager
func NewItemManager(svc *Service) *ItemManager {
	return &ItemManager{
		items: make(map[dbus.ObjectPath]*Item),
		svc:   svc,
	}
}

// GetOrCreate returns an existing item or creates a new one
func (m *ItemManager) GetOrCreate(collection, id string) (*Item, error) {
	path := dbtypes.ItemPath(collection, id)

	m.mu.Lock()
	defer m.mu.Unlock()

	if item, ok := m.items[path]; ok {
		return item, nil
	}

	item := NewItem(m.svc, collection, id)
	if err := item.Export(); err != nil {
		return nil, err
	}

	m.items[path] = item
	return item, nil
}

// Remove removes an item from the manager
func (m *ItemManager) Remove(path dbus.ObjectPath) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if item, ok := m.items[path]; ok {
		item.Unexport()
		delete(m.items, path)
	}
}

// RemoveCollection removes all items for a collection
func (m *ItemManager) RemoveCollection(collection string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	prefix := dbtypes.CollectionPath(collection) + "/"
	for path, item := range m.items {
		if len(string(path)) > len(prefix) && string(path)[:len(prefix)] == string(prefix) {
			item.Unexport()
			delete(m.items, path)
		}
	}
}

// GetItem returns an item by path
func (m *ItemManager) GetItem(path dbus.ObjectPath) (*Item, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	item, ok := m.items[path]
	return item, ok
}

// ExportAllItems exports all items for a collection
func (m *ItemManager) ExportAllItems(collection string) error {
	ctx := context.Background()
	items, err := m.svc.store.Items(ctx, collection)
	if err != nil {
		return err
	}

	for _, id := range items {
		if _, err := m.GetOrCreate(collection, id); err != nil {
			return err
		}
	}

	return nil
}
