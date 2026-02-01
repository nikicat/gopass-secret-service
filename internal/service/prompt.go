package service

import (
	"fmt"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/google/uuid"

	dbtypes "github.com/nikicat/gopass-secret-service/internal/dbus"
)

// PromptAction represents the action to perform after a prompt completes
type PromptAction func() (dbus.Variant, error)

// Prompt represents a D-Bus prompt for operations requiring authentication
type Prompt struct {
	path       dbus.ObjectPath
	id         string
	conn       *dbus.Conn
	action     PromptAction
	dismissed  bool
	completed  bool
	mu         sync.Mutex
	onComplete func()
}

// PromptManager manages active prompts
type PromptManager struct {
	prompts map[string]*Prompt
	mu      sync.RWMutex
	conn    *dbus.Conn
}

// NewPromptManager creates a new prompt manager
func NewPromptManager(conn *dbus.Conn) *PromptManager {
	return &PromptManager{
		prompts: make(map[string]*Prompt),
		conn:    conn,
	}
}

// CreatePrompt creates a new prompt
func (m *PromptManager) CreatePrompt(action PromptAction) (*Prompt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	rawID := uuid.New()
	id := fmt.Sprintf("p%x", rawID[:])
	prompt := &Prompt{
		path:   dbtypes.PromptPath(id),
		id:     id,
		conn:   m.conn,
		action: action,
	}

	prompt.onComplete = func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.prompts, id)
	}

	m.prompts[id] = prompt

	// Export prompt object
	if err := m.conn.Export(prompt, prompt.path, dbtypes.PromptInterface); err != nil {
		delete(m.prompts, id)
		return nil, err
	}

	// Export introspection
	introXML := `<node>
  <interface name="org.freedesktop.Secret.Prompt">
    <method name="Prompt">
      <arg name="window-id" type="s" direction="in"/>
    </method>
    <method name="Dismiss"/>
    <signal name="Completed">
      <arg name="dismissed" type="b"/>
      <arg name="result" type="v"/>
    </signal>
  </interface>
</node>`
	if err := m.conn.Export(introspect(introXML), prompt.path, "org.freedesktop.DBus.Introspectable"); err != nil {
		m.conn.Export(nil, prompt.path, dbtypes.PromptInterface)
		delete(m.prompts, id)
		return nil, err
	}

	return prompt, nil
}

// GetPrompt returns a prompt by path
func (m *PromptManager) GetPrompt(path dbus.ObjectPath) (*Prompt, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	id, err := dbtypes.ParsePromptPath(path)
	if err != nil {
		return nil, false
	}

	prompt, ok := m.prompts[id]
	return prompt, ok
}

// ParsePromptPath extracts the prompt ID from a D-Bus path
func ParsePromptPath(path dbus.ObjectPath) (string, error) {
	return dbtypes.ParsePromptPath(path)
}

// CloseAll closes all prompts
func (m *PromptManager) CloseAll() {
	m.mu.Lock()
	// Copy prompts to avoid holding lock during cleanup
	prompts := make([]*Prompt, 0, len(m.prompts))
	for _, prompt := range m.prompts {
		prompts = append(prompts, prompt)
	}
	m.prompts = make(map[string]*Prompt)
	m.mu.Unlock()

	// Cleanup prompts without holding the lock
	for _, prompt := range prompts {
		prompt.cleanup()
	}
	m.prompts = make(map[string]*Prompt)
}

// Path returns the prompt's D-Bus path
func (p *Prompt) Path() dbus.ObjectPath {
	return p.path
}

// Prompt implements org.freedesktop.Secret.Prompt.Prompt
func (p *Prompt) Prompt(windowID string) *dbus.Error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.completed || p.dismissed {
		return nil
	}

	// Execute the action
	result, err := p.action()
	if err != nil {
		// Emit completed with dismissed=true on error
		p.emitCompleted(true, dbus.MakeVariant(""))
		return nil
	}

	p.completed = true
	p.emitCompleted(false, result)
	return nil
}

// Dismiss implements org.freedesktop.Secret.Prompt.Dismiss
func (p *Prompt) Dismiss() *dbus.Error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.completed || p.dismissed {
		return nil
	}

	p.dismissed = true
	p.emitCompleted(true, dbus.MakeVariant(""))
	return nil
}

func (p *Prompt) emitCompleted(dismissed bool, result dbus.Variant) {
	p.conn.Emit(p.path, dbtypes.PromptInterface+".Completed", dismissed, result)
	p.cleanup()
	if p.onComplete != nil {
		p.onComplete()
	}
}

func (p *Prompt) cleanup() {
	p.conn.Export(nil, p.path, dbtypes.PromptInterface)
	p.conn.Export(nil, p.path, "org.freedesktop.DBus.Introspectable")
}
