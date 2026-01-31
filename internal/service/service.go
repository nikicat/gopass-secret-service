package service

import (
	"fmt"
	"log"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"

	"github.com/nblogist/gopass-secret-service/internal/config"
	dbtypes "github.com/nblogist/gopass-secret-service/internal/dbus"
	"github.com/nblogist/gopass-secret-service/internal/store"
)

// Service implements the org.freedesktop.Secret.Service interface
type Service struct {
	conn        *dbus.Conn
	store       store.Store
	cfg         *config.Config
	sessions    *SessionManager
	prompts     *PromptManager
	collections *CollectionManager
	items       *ItemManager
	props       *prop.Properties
	mu          sync.RWMutex
}

// New creates a new Secret Service
func New(cfg *config.Config) (*Service, error) {
	// Connect to session bus
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to session bus: %w", err)
	}

	// Create the store
	gopassStore := store.NewGopassStore(cfg.Prefix)

	svc := &Service{
		conn:  conn,
		store: gopassStore,
		cfg:   cfg,
	}

	// Initialize managers
	svc.sessions = NewSessionManager(conn)
	svc.prompts = NewPromptManager(conn)
	svc.collections = NewCollectionManager(svc)
	svc.items = NewItemManager(svc)

	return svc, nil
}

// Start starts the service and acquires the D-Bus name
func (s *Service) Start() error {
	// Export the service object
	if err := s.conn.Export(s, dbtypes.ServicePath, dbtypes.SecretServiceInterface); err != nil {
		return fmt.Errorf("failed to export service: %w", err)
	}

	// Set up properties
	collections := s.collections.GetPaths()
	propsSpec := map[string]map[string]*prop.Prop{
		dbtypes.SecretServiceInterface: {
			"Collections": {
				Value:    collections,
				Writable: false,
				Emit:     prop.EmitTrue,
			},
		},
	}

	props, err := prop.Export(s.conn, dbtypes.ServicePath, propsSpec)
	if err != nil {
		return fmt.Errorf("failed to export properties: %w", err)
	}
	s.props = props

	// Export introspection
	introXML := s.introspectionXML()
	if err := s.conn.Export(introspect(introXML), dbtypes.ServicePath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return fmt.Errorf("failed to export introspection: %w", err)
	}

	// Request the bus name
	flags := dbus.NameFlagDoNotQueue
	if s.cfg.Replace {
		flags |= dbus.NameFlagReplaceExisting
	}

	reply, err := s.conn.RequestName(dbtypes.ServiceName, flags)
	if err != nil {
		return fmt.Errorf("failed to request name: %w", err)
	}

	if reply != dbus.RequestNameReplyPrimaryOwner {
		return fmt.Errorf("name %s already taken", dbtypes.ServiceName)
	}

	log.Printf("Acquired D-Bus name: %s", dbtypes.ServiceName)

	// Export existing collections
	if err := s.collections.ExportAll(); err != nil {
		log.Printf("Warning: failed to export existing collections: %v", err)
	}

	// Refresh collections property
	s.refreshCollections()

	// Ensure default collection exists
	if err := s.ensureDefaultCollection(); err != nil {
		log.Printf("Warning: failed to ensure default collection: %v", err)
	}

	return nil
}

// Stop stops the service
func (s *Service) Stop() error {
	s.sessions.CloseAll()
	s.prompts.CloseAll()

	if _, err := s.conn.ReleaseName(dbtypes.ServiceName); err != nil {
		return err
	}

	return s.conn.Close()
}

// OpenSession implements org.freedesktop.Secret.Service.OpenSession
func (s *Service) OpenSession(algorithm string, input dbus.Variant) (dbus.Variant, dbus.ObjectPath, *dbus.Error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get client ID from input parameters (for plain, it's empty)
	var inputBytes []byte
	if v, ok := input.Value().([]byte); ok {
		inputBytes = v
	}

	session, output, err := s.sessions.CreateSession(algorithm, inputBytes, "")
	if err != nil {
		return dbus.MakeVariant([]byte{}), "/", ErrUnsupported(err.Error())
	}

	return dbus.MakeVariant(output), session.Path(), nil
}

// CreateCollection implements org.freedesktop.Secret.Service.CreateCollection
func (s *Service) CreateCollection(properties map[string]dbus.Variant, alias string) (dbus.ObjectPath, dbus.ObjectPath, *dbus.Error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Extract label from properties
	label := ""
	if v, ok := properties["org.freedesktop.Secret.Collection.Label"]; ok {
		if l, ok := v.Value().(string); ok {
			label = l
		}
	}

	// Use alias as name if provided, otherwise use label
	name := alias
	if name == "" {
		name = label
	}
	if name == "" {
		name = "collection"
	}
	name = store.SanitizeName(name)

	// Check if collection exists
	if _, ok := s.collections.Get(name); ok {
		return "/", "/", ErrExists("collection already exists")
	}

	// Create collection in store
	if err := s.store.CreateCollection(name, label); err != nil {
		return "/", "/", ErrUnsupported(err.Error())
	}

	// Create and export collection object
	coll, err := s.collections.GetOrCreate(name)
	if err != nil {
		return "/", "/", ErrUnsupported(err.Error())
	}

	// Set alias if provided
	if alias != "" {
		if err := s.store.SetAlias(alias, name); err != nil {
			log.Printf("Warning: failed to set alias %s: %v", alias, err)
		}
	}

	// Emit CollectionCreated signal
	s.emitCollectionCreated(coll.Path())

	// Refresh collections property
	s.refreshCollections()

	return coll.Path(), "/", nil // "/" means no prompt needed
}

// SearchItems implements org.freedesktop.Secret.Service.SearchItems
func (s *Service) SearchItems(attributes map[string]string) ([]dbus.ObjectPath, []dbus.ObjectPath, *dbus.Error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	results, err := s.store.SearchAllItems(attributes)
	if err != nil {
		return nil, nil, ErrObjectNotFound(err.Error())
	}

	var unlocked, locked []dbus.ObjectPath
	for collName, items := range results {
		collData, _ := s.store.GetCollection(collName)
		isLocked := collData != nil && collData.Locked

		for _, item := range items {
			path := dbtypes.ItemPath(collName, item.ID)
			if isLocked {
				locked = append(locked, path)
			} else {
				unlocked = append(unlocked, path)
			}
		}
	}

	return unlocked, locked, nil
}

// Unlock implements org.freedesktop.Secret.Service.Unlock
func (s *Service) Unlock(objects []dbus.ObjectPath) ([]dbus.ObjectPath, dbus.ObjectPath, *dbus.Error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var unlocked []dbus.ObjectPath

	for _, path := range objects {
		if dbtypes.IsCollectionPath(path) {
			name, err := dbtypes.ParseCollectionPath(path)
			if err != nil {
				continue
			}
			if err := s.store.UnlockCollection(name); err != nil {
				continue
			}
			unlocked = append(unlocked, path)
		}
	}

	// No prompt needed for this implementation
	return unlocked, "/", nil
}

// Lock implements org.freedesktop.Secret.Service.Lock
func (s *Service) Lock(objects []dbus.ObjectPath) ([]dbus.ObjectPath, dbus.ObjectPath, *dbus.Error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var locked []dbus.ObjectPath

	for _, path := range objects {
		if dbtypes.IsCollectionPath(path) {
			name, err := dbtypes.ParseCollectionPath(path)
			if err != nil {
				continue
			}
			if err := s.store.LockCollection(name); err != nil {
				continue
			}
			locked = append(locked, path)
		}
	}

	// No prompt needed for this implementation
	return locked, "/", nil
}

// GetSecrets implements org.freedesktop.Secret.Service.GetSecrets
func (s *Service) GetSecrets(items []dbus.ObjectPath, session dbus.ObjectPath) (map[dbus.ObjectPath]dbtypes.Secret, *dbus.Error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sess, ok := s.sessions.GetSession(session)
	if !ok {
		return nil, ErrSessionNotFound("session not found")
	}

	secrets := make(map[dbus.ObjectPath]dbtypes.Secret)

	for _, path := range items {
		collection, id, err := dbtypes.ParseItemPath(path)
		if err != nil {
			continue
		}

		item, err := s.store.GetItem(collection, id)
		if err != nil {
			continue
		}

		params, ciphertext, err := sess.Encrypt(item.Secret)
		if err != nil {
			continue
		}

		secrets[path] = dbtypes.Secret{
			Session:     session,
			Parameters:  params,
			Value:       ciphertext,
			ContentType: item.ContentType,
		}
	}

	return secrets, nil
}

// ReadAlias implements org.freedesktop.Secret.Service.ReadAlias
func (s *Service) ReadAlias(name string) (dbus.ObjectPath, *dbus.Error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	collName, err := s.store.GetAlias(name)
	if err != nil {
		return "/", nil // Return "/" for unknown alias (not an error per spec)
	}

	return dbtypes.CollectionPath(collName), nil
}

// SetAlias implements org.freedesktop.Secret.Service.SetAlias
func (s *Service) SetAlias(name string, collection dbus.ObjectPath) *dbus.Error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if collection == "/" {
		// Remove alias
		if err := s.store.SetAlias(name, ""); err != nil {
			return ErrUnsupported(err.Error())
		}
		return nil
	}

	collName, err := dbtypes.ParseCollectionPath(collection)
	if err != nil {
		return ErrObjectNotFound(err.Error())
	}

	if err := s.store.SetAlias(name, collName); err != nil {
		return ErrUnsupported(err.Error())
	}

	return nil
}

// Signal emission helpers

func (s *Service) emitCollectionCreated(path dbus.ObjectPath) {
	s.conn.Emit(dbtypes.ServicePath, dbtypes.SecretServiceInterface+".CollectionCreated", path)
}

func (s *Service) emitCollectionDeleted(path dbus.ObjectPath) {
	s.conn.Emit(dbtypes.ServicePath, dbtypes.SecretServiceInterface+".CollectionDeleted", path)
}

func (s *Service) emitCollectionChanged(path dbus.ObjectPath) {
	s.conn.Emit(dbtypes.ServicePath, dbtypes.SecretServiceInterface+".CollectionChanged", path)
}

func (s *Service) emitItemCreated(collection string, path dbus.ObjectPath) {
	collPath := dbtypes.CollectionPath(collection)
	s.conn.Emit(collPath, dbtypes.CollectionInterface+".ItemCreated", path)
}

func (s *Service) emitItemDeleted(collection string, path dbus.ObjectPath) {
	collPath := dbtypes.CollectionPath(collection)
	s.conn.Emit(collPath, dbtypes.CollectionInterface+".ItemDeleted", path)
}

func (s *Service) emitItemChanged(collection string, path dbus.ObjectPath) {
	collPath := dbtypes.CollectionPath(collection)
	s.conn.Emit(collPath, dbtypes.CollectionInterface+".ItemChanged", path)
}

func (s *Service) refreshCollections() {
	if s.props != nil {
		s.props.SetMust(dbtypes.SecretServiceInterface, "Collections", s.collections.GetPaths())
	}
}

func (s *Service) ensureDefaultCollection() error {
	// Check if default collection exists
	collName, err := s.store.GetAlias("default")
	if err == nil {
		// Check if the collection actually exists
		if _, err := s.store.GetCollection(collName); err == nil {
			// Export alias for existing collection
			coll, ok := s.collections.Get(collName)
			if !ok {
				// Collection exists in store but not exported yet, export it
				log.Printf("Collection %s exists but not exported, exporting now", collName)
				coll, err = s.collections.GetOrCreate(collName)
				if err != nil {
					return fmt.Errorf("failed to export collection %s: %w", collName, err)
				}
			}
			s.exportAlias("default", coll)
			log.Printf("Exported default alias for collection %s", collName)
			return nil
		}
	}

	// Create default collection
	defaultName := s.cfg.DefaultCollection
	if err := s.store.CreateCollection(defaultName, "Default"); err != nil {
		return err
	}

	// Set alias
	if err := s.store.SetAlias("default", defaultName); err != nil {
		return err
	}

	// Export the collection
	coll, err := s.collections.GetOrCreate(defaultName)
	if err != nil {
		return err
	}

	// Export alias
	s.exportAlias("default", coll)

	s.refreshCollections()
	return nil
}

// exportAlias exports a collection at an alias path
func (s *Service) exportAlias(alias string, coll *Collection) {
	if err := coll.ExportAtPath(dbtypes.AliasPath(alias)); err != nil {
		log.Printf("Warning: failed to export alias %s: %v", alias, err)
	}
}

func (s *Service) introspectionXML() string {
	return `<node>
  <interface name="org.freedesktop.Secret.Service">
    <method name="OpenSession">
      <arg name="algorithm" type="s" direction="in"/>
      <arg name="input" type="v" direction="in"/>
      <arg name="output" type="v" direction="out"/>
      <arg name="result" type="o" direction="out"/>
    </method>
    <method name="CreateCollection">
      <arg name="properties" type="a{sv}" direction="in"/>
      <arg name="alias" type="s" direction="in"/>
      <arg name="collection" type="o" direction="out"/>
      <arg name="prompt" type="o" direction="out"/>
    </method>
    <method name="SearchItems">
      <arg name="attributes" type="a{ss}" direction="in"/>
      <arg name="unlocked" type="ao" direction="out"/>
      <arg name="locked" type="ao" direction="out"/>
    </method>
    <method name="Unlock">
      <arg name="objects" type="ao" direction="in"/>
      <arg name="unlocked" type="ao" direction="out"/>
      <arg name="prompt" type="o" direction="out"/>
    </method>
    <method name="Lock">
      <arg name="objects" type="ao" direction="in"/>
      <arg name="locked" type="ao" direction="out"/>
      <arg name="prompt" type="o" direction="out"/>
    </method>
    <method name="GetSecrets">
      <arg name="items" type="ao" direction="in"/>
      <arg name="session" type="o" direction="in"/>
      <arg name="secrets" type="a{o(oayays)}" direction="out"/>
    </method>
    <method name="ReadAlias">
      <arg name="name" type="s" direction="in"/>
      <arg name="collection" type="o" direction="out"/>
    </method>
    <method name="SetAlias">
      <arg name="name" type="s" direction="in"/>
      <arg name="collection" type="o" direction="in"/>
    </method>
    <signal name="CollectionCreated">
      <arg name="collection" type="o"/>
    </signal>
    <signal name="CollectionDeleted">
      <arg name="collection" type="o"/>
    </signal>
    <signal name="CollectionChanged">
      <arg name="collection" type="o"/>
    </signal>
    <property name="Collections" type="ao" access="read"/>
  </interface>
</node>`
}
