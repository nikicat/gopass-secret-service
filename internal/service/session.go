package service

import (
	"fmt"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/google/uuid"

	"github.com/nblogist/gopass-secret-service/internal/crypto"
	dbtypes "github.com/nblogist/gopass-secret-service/internal/dbus"
)

// Session represents a D-Bus session for encrypted communication
type Session struct {
	path      dbus.ObjectPath
	id        string
	crypto    crypto.Session
	clientID  string
	conn      *dbus.Conn
	mu        sync.RWMutex
	closed    bool
	onClose   func()
}

// SessionManager manages active sessions
type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
	conn     *dbus.Conn
}

// NewSessionManager creates a new session manager
func NewSessionManager(conn *dbus.Conn) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
		conn:     conn,
	}
}

// CreateSession creates a new session with the given algorithm
func (m *SessionManager) CreateSession(algorithm string, input []byte, clientID string) (*Session, []byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cryptoSession, output, err := crypto.NewSession(algorithm, input)
	if err != nil {
		return nil, nil, err
	}

	// D-Bus path elements cannot contain hyphens, use hex string without dashes
	rawID := uuid.New()
	id := fmt.Sprintf("s%x", rawID[:])
	session := &Session{
		path:     dbtypes.SessionPath(id),
		id:       id,
		crypto:   cryptoSession,
		clientID: clientID,
		conn:     m.conn,
	}

	session.onClose = func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.sessions, id)
	}

	m.sessions[id] = session

	// Export session object
	if err := m.conn.Export(session, session.path, dbtypes.SessionInterface); err != nil {
		delete(m.sessions, id)
		return nil, nil, err
	}

	// Export introspection
	introXML := `<node>
  <interface name="org.freedesktop.Secret.Session">
    <method name="Close"/>
  </interface>
</node>`
	if err := m.conn.Export(introspect(introXML), session.path, "org.freedesktop.DBus.Introspectable"); err != nil {
		m.conn.Export(nil, session.path, dbtypes.SessionInterface)
		delete(m.sessions, id)
		return nil, nil, err
	}

	return session, output, nil
}

// GetSession returns a session by path
func (m *SessionManager) GetSession(path dbus.ObjectPath) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, err := dbtypes.ParseSessionPath(path)
	if err != nil {
		return nil, false
	}

	session, ok := m.sessions[id]
	return session, ok
}

// CloseSession closes a session by path
func (m *SessionManager) CloseSession(path dbus.ObjectPath) error {
	session, ok := m.GetSession(path)
	if !ok {
		return nil
	}
	return session.Close()
}

// CloseAll closes all sessions
func (m *SessionManager) CloseAll() {
	m.mu.Lock()
	// Copy sessions to avoid holding lock during close (prevents deadlock)
	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.sessions = make(map[string]*Session)
	m.mu.Unlock()

	// Close sessions without holding the lock
	for _, session := range sessions {
		session.closeWithoutCallback()
	}
}

// Path returns the session's D-Bus path
func (s *Session) Path() dbus.ObjectPath {
	return s.path
}

// Close implements org.freedesktop.Secret.Session.Close
func (s *Session) Close() *dbus.Error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.close()
	return nil
}

func (s *Session) close() {
	if s.closed {
		return
	}
	s.closed = true

	// Unexport from D-Bus
	s.conn.Export(nil, s.path, dbtypes.SessionInterface)
	s.conn.Export(nil, s.path, "org.freedesktop.DBus.Introspectable")

	// Close crypto session
	s.crypto.Close()

	// Notify manager
	if s.onClose != nil {
		s.onClose()
	}
}

// closeWithoutCallback closes the session without calling onClose
// Used during bulk cleanup to avoid deadlock
func (s *Session) closeWithoutCallback() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}
	s.closed = true

	// Unexport from D-Bus
	s.conn.Export(nil, s.path, dbtypes.SessionInterface)
	s.conn.Export(nil, s.path, "org.freedesktop.DBus.Introspectable")

	// Close crypto session
	s.crypto.Close()
}

// Encrypt encrypts data using this session's crypto
func (s *Session) Encrypt(plaintext []byte) (params, ciphertext []byte, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, nil, ErrSessionNotFound("session is closed")
	}

	return s.crypto.Encrypt(plaintext)
}

// Decrypt decrypts data using this session's crypto
func (s *Session) Decrypt(params, ciphertext []byte) (plaintext []byte, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrSessionNotFound("session is closed")
	}

	return s.crypto.Decrypt(params, ciphertext)
}

// introspect is a simple introspection handler
type introspect string

func (i introspect) Introspect() (string, *dbus.Error) {
	return string(i), nil
}
