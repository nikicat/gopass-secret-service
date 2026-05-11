package service

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/google/uuid"

	dbtypes "github.com/nikicat/gopass-secret-service/internal/dbus"
	"github.com/nikicat/gopass-secret-service/internal/store"
)

// Collection represents a D-Bus Secret Service collection.
//
// Properties (Items, Label, Locked, Created, Modified) are served by
// collectionPropsHandler, which reads from the store on every Get rather than
// caching values at export time. PropertiesChanged signals are still emitted
// from mutation paths (CreateItem, setLabel) so subscribers get notified, but
// the cache-and-emit pattern would have been a source of bugs whenever the
// store mutated through any path other than D-Bus (e.g. external `gopass`
// CLI, or items that existed on disk before the service started).
type Collection struct {
	path dbus.ObjectPath
	name string
	svc  *Service
	mu   sync.RWMutex
}

// NewCollection creates a new Collection instance
func NewCollection(svc *Service, name string) *Collection {
	return &Collection{
		path: dbtypes.CollectionPath(name),
		name: name,
		svc:  svc,
	}
}

// collectionPropsHandler implements org.freedesktop.DBus.Properties for the
// org.freedesktop.Secret.Collection interface by reading from the store on
// every access. Replaces godbus's prop.Export, which caches values at export
// time and can only refresh them via SetMust — that would be wrong here
// because the underlying gopass store can mutate outside of D-Bus (e.g. via
// the gopass CLI or items that pre-existed on disk), and we have no hook to
// observe those changes.
type collectionPropsHandler struct {
	coll *Collection
}

func (h *collectionPropsHandler) Get(iface, property string) (dbus.Variant, *dbus.Error) {
	if iface != dbtypes.CollectionInterface {
		return dbus.Variant{}, ErrUnsupported("unknown interface: " + iface)
	}
	switch property {
	case "Items":
		return dbus.MakeVariant(h.coll.getItemPaths()), nil
	case "Label", "Locked", "Created", "Modified":
		// Fall through to collection data lookup below.
	default:
		return dbus.Variant{}, ErrUnsupported("unknown property: " + property)
	}

	collData, _ := h.coll.svc.store.GetCollection(context.Background(), h.coll.name)
	switch property {
	case "Label":
		if collData == nil {
			return dbus.MakeVariant(h.coll.name), nil
		}
		return dbus.MakeVariant(collData.Label), nil
	case "Locked":
		if collData == nil {
			return dbus.MakeVariant(false), nil
		}
		return dbus.MakeVariant(collData.Locked), nil
	case "Created":
		if collData == nil {
			return dbus.MakeVariant(uint64(0)), nil
		}
		return dbus.MakeVariant(uint64(collData.Created.Unix())), nil
	case "Modified":
		if collData == nil {
			return dbus.MakeVariant(uint64(0)), nil
		}
		return dbus.MakeVariant(uint64(collData.Modified.Unix())), nil
	}
	// Unreachable; kept to satisfy the compiler.
	return dbus.Variant{}, ErrUnsupported("unknown property: " + property)
}

func (h *collectionPropsHandler) GetAll(iface string) (map[string]dbus.Variant, *dbus.Error) {
	if iface != dbtypes.CollectionInterface {
		return nil, ErrUnsupported("unknown interface: " + iface)
	}
	collData, _ := h.coll.svc.store.GetCollection(context.Background(), h.coll.name)
	label := h.coll.name
	locked := false
	created := uint64(0)
	modified := uint64(0)
	if collData != nil {
		label = collData.Label
		locked = collData.Locked
		created = uint64(collData.Created.Unix())
		modified = uint64(collData.Modified.Unix())
	}
	return map[string]dbus.Variant{
		"Items":    dbus.MakeVariant(h.coll.getItemPaths()),
		"Label":    dbus.MakeVariant(label),
		"Locked":   dbus.MakeVariant(locked),
		"Created":  dbus.MakeVariant(created),
		"Modified": dbus.MakeVariant(modified),
	}, nil
}

func (h *collectionPropsHandler) Set(iface, property string, value dbus.Variant) *dbus.Error {
	if iface != dbtypes.CollectionInterface {
		return ErrUnsupported("unknown interface: " + iface)
	}
	if property != "Label" {
		return ErrUnsupported("property is read-only: " + property)
	}
	label, ok := value.Value().(string)
	if !ok {
		return ErrUnsupported("invalid label type")
	}
	return h.coll.setLabel(label)
}

// Path returns the collection's D-Bus path
func (c *Collection) Path() dbus.ObjectPath {
	return c.path
}

// Name returns the collection name
func (c *Collection) Name() string {
	return c.name
}

// Export exports the collection to D-Bus
func (c *Collection) Export() error {
	log.Printf("Collection.Export: exporting %s at path %s", c.name, c.path)
	conn := c.svc.conn

	// Export the collection interface
	if err := conn.Export(c, c.path, dbtypes.CollectionInterface); err != nil {
		log.Printf("Collection.Export: failed to export interface: %v", err)
		return err
	}
	log.Printf("Collection.Export: interface exported")

	// Live properties handler — reads from the store on every Get rather
	// than caching at export time. See type comment.
	if err := conn.Export(&collectionPropsHandler{coll: c}, c.path, "org.freedesktop.DBus.Properties"); err != nil {
		conn.Export(nil, c.path, dbtypes.CollectionInterface)
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
  <interface name="org.freedesktop.Secret.Collection">
    <method name="Delete">
      <arg name="prompt" type="o" direction="out"/>
    </method>
    <method name="SearchItems">
      <arg name="attributes" type="a{ss}" direction="in"/>
      <arg name="results" type="ao" direction="out"/>
    </method>
    <method name="CreateItem">
      <arg name="properties" type="a{sv}" direction="in"/>
      <arg name="secret" type="(oayays)" direction="in"/>
      <arg name="replace" type="b" direction="in"/>
      <arg name="item" type="o" direction="out"/>
      <arg name="prompt" type="o" direction="out"/>
    </method>
    <signal name="ItemCreated">
      <arg name="item" type="o"/>
    </signal>
    <signal name="ItemDeleted">
      <arg name="item" type="o"/>
    </signal>
    <signal name="ItemChanged">
      <arg name="item" type="o"/>
    </signal>
    <property name="Items" type="ao" access="read"/>
    <property name="Label" type="s" access="readwrite"/>
    <property name="Locked" type="b" access="read"/>
    <property name="Created" type="t" access="read"/>
    <property name="Modified" type="t" access="read"/>
  </interface>
</node>`
	if err := conn.Export(introspect(introXML), c.path, "org.freedesktop.DBus.Introspectable"); err != nil {
		return err
	}

	// Export all items in this collection
	return c.svc.items.ExportAllItems(c.name)
}

// Unexport removes the collection from D-Bus
func (c *Collection) Unexport() {
	conn := c.svc.conn
	conn.Export(nil, c.path, dbtypes.CollectionInterface)
	conn.Export(nil, c.path, "org.freedesktop.DBus.Properties")
	conn.Export(nil, c.path, "org.freedesktop.DBus.Introspectable")

	// Remove all items
	c.svc.items.RemoveCollection(c.name)
}

// ExportAtPath exports the collection at an additional path (for aliases)
func (c *Collection) ExportAtPath(path dbus.ObjectPath) error {
	conn := c.svc.conn

	// Export the collection interface at the alias path
	if err := conn.Export(c, path, dbtypes.CollectionInterface); err != nil {
		return err
	}

	// Same live properties handler as the canonical path uses.
	if err := conn.Export(&collectionPropsHandler{coll: c}, path, "org.freedesktop.DBus.Properties"); err != nil {
		conn.Export(nil, path, dbtypes.CollectionInterface)
		return err
	}

	// Export introspection at alias path
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
  <interface name="org.freedesktop.Secret.Collection">
    <method name="Delete">
      <arg name="prompt" type="o" direction="out"/>
    </method>
    <method name="SearchItems">
      <arg name="attributes" type="a{ss}" direction="in"/>
      <arg name="results" type="ao" direction="out"/>
    </method>
    <method name="CreateItem">
      <arg name="properties" type="a{sv}" direction="in"/>
      <arg name="secret" type="(oayays)" direction="in"/>
      <arg name="replace" type="b" direction="in"/>
      <arg name="item" type="o" direction="out"/>
      <arg name="prompt" type="o" direction="out"/>
    </method>
    <signal name="ItemCreated">
      <arg name="item" type="o"/>
    </signal>
    <signal name="ItemDeleted">
      <arg name="item" type="o"/>
    </signal>
    <signal name="ItemChanged">
      <arg name="item" type="o"/>
    </signal>
    <property name="Items" type="ao" access="read"/>
    <property name="Label" type="s" access="readwrite"/>
    <property name="Locked" type="b" access="read"/>
    <property name="Created" type="t" access="read"/>
    <property name="Modified" type="t" access="read"/>
  </interface>
</node>`
	if err := conn.Export(introspect(introXML), path, "org.freedesktop.DBus.Introspectable"); err != nil {
		return err
	}

	return nil
}

// Delete implements org.freedesktop.Secret.Collection.Delete
func (c *Collection) Delete() (dbus.ObjectPath, *dbus.Error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx := context.Background()
	if err := c.svc.store.DeleteCollection(ctx, c.name); err != nil {
		return "/", ErrObjectNotFound(err.Error())
	}

	// Remove from collection manager (unexports from D-Bus and removes from in-memory map)
	c.svc.collections.Remove(c.name)

	// Emit CollectionDeleted signal and update Collections property
	c.svc.emitCollectionDeleted(c.path)
	c.svc.refreshCollections()

	// Return "/" to indicate no prompt needed
	return "/", nil
}

// SearchItems implements org.freedesktop.Secret.Collection.SearchItems
func (c *Collection) SearchItems(attributes map[string]string) ([]dbus.ObjectPath, *dbus.Error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	ctx := context.Background()
	items, err := c.svc.store.SearchItems(ctx, c.name, attributes)
	if err != nil {
		return nil, ErrObjectNotFound(err.Error())
	}

	paths := make([]dbus.ObjectPath, 0, len(items))
	for _, item := range items {
		c.svc.items.EnsureExported(c.name, item.ID)
		paths = append(paths, dbtypes.ItemPath(c.name, item.ID))
	}

	return paths, nil
}

// CreateItem implements org.freedesktop.Secret.Collection.CreateItem
func (c *Collection) CreateItem(properties map[string]dbus.Variant, secret dbtypes.Secret, replace bool) (dbus.ObjectPath, dbus.ObjectPath, *dbus.Error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Get session
	session, ok := c.svc.sessions.GetSession(secret.Session)
	if !ok {
		return "/", "/", ErrSessionNotFound("session not found")
	}

	// Decrypt secret
	plaintext, err := session.Decrypt(secret.Parameters, secret.Value)
	if err != nil {
		return "/", "/", ErrUnsupported(err.Error())
	}

	// Extract properties
	label := ""
	attributes := make(map[string]string)

	if v, ok := properties["org.freedesktop.Secret.Item.Label"]; ok {
		if s, ok := v.Value().(string); ok {
			label = s
		}
	}

	if v, ok := properties["org.freedesktop.Secret.Item.Attributes"]; ok {
		switch a := v.Value().(type) {
		case map[string]string:
			attributes = a
		case map[string]dbus.Variant:
			// D-Bus may send attributes as map[string]Variant
			for k, vv := range a {
				if s, ok := vv.Value().(string); ok {
					attributes[k] = s
				}
			}
		}
	}

	ctx := context.Background()

	// Check for existing item with same attributes
	// This prevents duplicates - a common practical requirement even though
	// the spec technically allows duplicates when replace=false
	var existingItem *store.ItemData
	if len(attributes) > 0 {
		existing, err := c.svc.store.SearchItems(ctx, c.name, attributes)
		if err == nil && len(existing) > 0 {
			// Find exact match (all attributes must match)
			for _, item := range existing {
				if attributesMatch(item.Attributes, attributes) {
					existingItem = item
					break
				}
			}
		}
	}

	var itemID string
	if existingItem != nil {
		if replace {
			// Update existing item's secret
			existingItem.Secret = plaintext
			existingItem.ContentType = secret.ContentType
			if label != "" {
				existingItem.Label = label
			}
			if err := c.svc.store.UpdateItem(ctx, c.name, existingItem.ID, existingItem); err != nil {
				return "/", "/", ErrUnsupported(err.Error())
			}
			itemID = existingItem.ID

			// Ensure the item is exported on D-Bus. Same reasoning as the
			// replace=false branch below: a successful prior CreateItem only
			// guarantees the item lives in the store, not that an *Item proxy
			// is bound to its D-Bus path, since across a service restart the
			// in-memory ItemManager is empty.
			c.svc.items.EnsureExported(c.name, itemID)

			// Emit ItemChanged
			itemPath := dbtypes.ItemPath(c.name, itemID)
			c.svc.emitItemChanged(c.name, itemPath)
		} else {
			// Return existing item without modification (prevents duplicates)
			itemID = existingItem.ID

			// Make sure the item is exported
			if _, err := c.svc.items.GetOrCreate(c.name, itemID); err != nil {
				return "/", "/", ErrUnsupported(err.Error())
			}
		}
	} else {
		// Create new item - use hex format without hyphens for D-Bus path compatibility
		item := &store.ItemData{
			Label:       label,
			Secret:      plaintext,
			ContentType: secret.ContentType,
			Attributes:  attributes,
		}
		rawID := uuid.New()
		item.ID = fmt.Sprintf("i%x", rawID[:])
		id, err := c.svc.store.CreateItem(ctx, c.name, item)
		if err != nil {
			return "/", "/", ErrUnsupported(err.Error())
		}
		itemID = id

		// Export the new item
		if _, err := c.svc.items.GetOrCreate(c.name, itemID); err != nil {
			return "/", "/", ErrUnsupported(err.Error())
		}

		// Emit ItemCreated
		itemPath := dbtypes.ItemPath(c.name, itemID)
		c.svc.emitItemCreated(c.name, itemPath)

		// Update Items property
		c.refreshItems()
	}

	itemPath := dbtypes.ItemPath(c.name, itemID)
	return itemPath, "/", nil // "/" means no prompt needed
}

func (c *Collection) setLabel(label string) *dbus.Error {
	ctx := context.Background()
	if err := c.svc.store.SetCollectionLabel(ctx, c.name, label); err != nil {
		return ErrUnsupported(err.Error())
	}

	c.emitPropertiesChanged(map[string]dbus.Variant{
		"Label": dbus.MakeVariant(label),
	})
	c.svc.emitCollectionChanged(c.path)
	return nil
}

// attributesMatch checks if two attribute maps are exactly equal
func attributesMatch(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

func (c *Collection) getItemPaths() []dbus.ObjectPath {
	ctx := context.Background()
	items, err := c.svc.store.Items(ctx, c.name)
	if err != nil {
		return []dbus.ObjectPath{}
	}

	paths := make([]dbus.ObjectPath, 0, len(items))
	for _, id := range items {
		c.svc.items.EnsureExported(c.name, id)
		paths = append(paths, dbtypes.ItemPath(c.name, id))
	}
	return paths
}

// refreshItems emits PropertiesChanged for the Items property. Subscribers that
// cache the property locally (e.g. seahorse) re-fetch it on the signal; we no
// longer hold the value ourselves since collectionPropsHandler reads live.
func (c *Collection) refreshItems() {
	c.emitPropertiesChanged(map[string]dbus.Variant{
		"Items": dbus.MakeVariant(c.getItemPaths()),
	})
}

// emitPropertiesChanged sends org.freedesktop.DBus.Properties.PropertiesChanged
// for this collection. Used by mutation paths (CreateItem, setLabel) where we
// know exactly which keys changed.
func (c *Collection) emitPropertiesChanged(changed map[string]dbus.Variant) {
	c.svc.conn.Emit(c.path, "org.freedesktop.DBus.Properties.PropertiesChanged",
		dbtypes.CollectionInterface, changed, []string{})
}

// CollectionManager manages collections for the service
type CollectionManager struct {
	collections map[string]*Collection
	mu          sync.RWMutex
	svc         *Service
}

// NewCollectionManager creates a new collection manager
func NewCollectionManager(svc *Service) *CollectionManager {
	return &CollectionManager{
		collections: make(map[string]*Collection),
		svc:         svc,
	}
}

// GetOrCreate returns an existing collection or creates a new one
func (m *CollectionManager) GetOrCreate(name string) (*Collection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if coll, ok := m.collections[name]; ok {
		log.Printf("GetOrCreate: collection %s already exists", name)
		return coll, nil
	}

	log.Printf("GetOrCreate: creating new collection %s", name)
	coll := NewCollection(m.svc, name)
	if err := coll.Export(); err != nil {
		log.Printf("GetOrCreate: failed to export collection %s: %v", name, err)
		return nil, err
	}

	m.collections[name] = coll
	log.Printf("GetOrCreate: successfully exported collection %s", name)
	return coll, nil
}

// Get returns a collection by name
func (m *CollectionManager) Get(name string) (*Collection, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	coll, ok := m.collections[name]
	return coll, ok
}

// Remove removes a collection from the manager
func (m *CollectionManager) Remove(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if coll, ok := m.collections[name]; ok {
		coll.Unexport()
		delete(m.collections, name)
	}
}

// All returns all collection names
func (m *CollectionManager) All() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.collections))
	for name := range m.collections {
		names = append(names, name)
	}
	return names
}

// ExportAll exports all collections from the store
func (m *CollectionManager) ExportAll() error {
	ctx := context.Background()
	names, err := m.svc.store.Collections(ctx)
	if err != nil {
		log.Printf("ExportAll: failed to get collections: %v", err)
		return err
	}

	log.Printf("ExportAll: found %d collections: %v", len(names), names)

	for _, name := range names {
		if _, err := m.GetOrCreate(name); err != nil {
			log.Printf("ExportAll: failed to export collection %s: %v", name, err)
			return err
		}
		log.Printf("ExportAll: exported collection %s", name)
	}

	return nil
}

// GetPaths returns all collection D-Bus paths
func (m *CollectionManager) GetPaths() []dbus.ObjectPath {
	m.mu.RLock()
	defer m.mu.RUnlock()

	paths := make([]dbus.ObjectPath, 0, len(m.collections))
	for _, coll := range m.collections {
		paths = append(paths, coll.Path())
	}
	return paths
}
