package service

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
	"github.com/google/uuid"

	dbtypes "github.com/nikicat/gopass-secret-service/internal/dbus"
	"github.com/nikicat/gopass-secret-service/internal/store"
)

// Collection represents a D-Bus Secret Service collection
type Collection struct {
	path  dbus.ObjectPath
	name  string
	svc   *Service
	mu    sync.RWMutex
	props *prop.Properties
}

// NewCollection creates a new Collection instance
func NewCollection(svc *Service, name string) *Collection {
	return &Collection{
		path: dbtypes.CollectionPath(name),
		name: name,
		svc:  svc,
	}
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

	log.Printf("Collection.Export: getting collection data...")
	// Get collection data for initial property values
	ctx := context.Background()
	collData, err := c.svc.store.GetCollection(ctx, c.name)
	if err != nil {
		log.Printf("Collection.Export: failed to get collection data: %v", err)
	}
	log.Printf("Collection.Export: got collection data")
	label := c.name
	locked := false
	created := uint64(0)
	modified := uint64(0)
	if collData != nil {
		label = collData.Label
		locked = collData.Locked
		created = uint64(collData.Created.Unix())
		modified = uint64(collData.Modified.Unix())
	}

	log.Printf("Collection.Export: getting item paths...")
	// Get items for initial property value
	items := c.getItemPaths()
	log.Printf("Collection.Export: got %d item paths", len(items))

	// Set up properties
	propsSpec := map[string]map[string]*prop.Prop{
		dbtypes.CollectionInterface: {
			"Items": {
				Value:    items,
				Writable: false,
				Emit:     prop.EmitTrue,
			},
			"Label": {
				Value:    label,
				Writable: true,
				Emit:     prop.EmitTrue,
				Callback: func(ch *prop.Change) *dbus.Error {
					newLabel, ok := ch.Value.(string)
					if !ok {
						return ErrUnsupported("invalid label type")
					}
					return c.setLabel(newLabel)
				},
			},
			"Locked": {
				Value:    locked,
				Writable: false,
				Emit:     prop.EmitTrue,
			},
			"Created": {
				Value:    created,
				Writable: false,
				Emit:     prop.EmitFalse,
			},
			"Modified": {
				Value:    modified,
				Writable: false,
				Emit:     prop.EmitFalse,
			},
		},
	}

	props, err := prop.Export(conn, c.path, propsSpec)
	if err != nil {
		conn.Export(nil, c.path, dbtypes.CollectionInterface)
		return err
	}
	c.props = props

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

	// Get collection data for property values
	ctx := context.Background()
	collData, _ := c.svc.store.GetCollection(ctx, c.name)
	label := c.name
	locked := false
	created := uint64(0)
	modified := uint64(0)
	if collData != nil {
		label = collData.Label
		locked = collData.Locked
		created = uint64(collData.Created.Unix())
		modified = uint64(collData.Modified.Unix())
	}
	items := c.getItemPaths()

	// Set up properties at the alias path
	propsSpec := map[string]map[string]*prop.Prop{
		dbtypes.CollectionInterface: {
			"Items": {
				Value:    items,
				Writable: false,
				Emit:     prop.EmitTrue,
			},
			"Label": {
				Value:    label,
				Writable: true,
				Emit:     prop.EmitTrue,
				Callback: func(ch *prop.Change) *dbus.Error {
					newLabel, ok := ch.Value.(string)
					if !ok {
						return ErrUnsupported("invalid label type")
					}
					return c.setLabel(newLabel)
				},
			},
			"Locked": {
				Value:    locked,
				Writable: false,
				Emit:     prop.EmitTrue,
			},
			"Created": {
				Value:    created,
				Writable: false,
				Emit:     prop.EmitFalse,
			},
			"Modified": {
				Value:    modified,
				Writable: false,
				Emit:     prop.EmitFalse,
			},
		},
	}

	if _, err := prop.Export(conn, path, propsSpec); err != nil {
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

	// Unexport from D-Bus
	c.Unexport()

	// Emit CollectionDeleted signal
	c.svc.emitCollectionDeleted(c.path)

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
		paths = append(paths, dbtypes.ItemPath(c.name, id))
	}
	return paths
}

func (c *Collection) refreshItems() {
	if c.props != nil {
		c.props.SetMust(dbtypes.CollectionInterface, "Items", c.getItemPaths())
	}
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
