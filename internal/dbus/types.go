package dbus

import (
	"github.com/godbus/dbus/v5"
)

// Secret represents a secret as transferred over D-Bus.
// Format: (oayays) - session path, parameters, value, content-type
type Secret struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

// SecretServiceInterface is the D-Bus interface name for the Secret Service
const SecretServiceInterface = "org.freedesktop.Secret.Service"

// CollectionInterface is the D-Bus interface name for collections
const CollectionInterface = "org.freedesktop.Secret.Collection"

// ItemInterface is the D-Bus interface name for items
const ItemInterface = "org.freedesktop.Secret.Item"

// SessionInterface is the D-Bus interface name for sessions
const SessionInterface = "org.freedesktop.Secret.Session"

// PromptInterface is the D-Bus interface name for prompts
const PromptInterface = "org.freedesktop.Secret.Prompt"

// ServiceName is the well-known D-Bus name for the Secret Service
const ServiceName = "org.freedesktop.secrets"

// ServicePath is the object path for the Secret Service
const ServicePath = dbus.ObjectPath("/org/freedesktop/secrets")

// CollectionBasePath is the base path for collections
const CollectionBasePath = "/org/freedesktop/secrets/collection"

// SessionBasePath is the base path for sessions
const SessionBasePath = "/org/freedesktop/secrets/session"

// PromptBasePath is the base path for prompts
const PromptBasePath = "/org/freedesktop/secrets/prompt"

// AliasBasePath is the base path for collection aliases
const AliasBasePath = "/org/freedesktop/secrets/aliases"

// Algorithm names
const (
	AlgorithmPlain = "plain"
)
