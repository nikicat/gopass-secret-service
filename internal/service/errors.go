package service

import (
	"github.com/godbus/dbus/v5"
)

// D-Bus error names for the Secret Service API
const (
	ErrIsLocked         = "org.freedesktop.Secret.Error.IsLocked"
	ErrNoSession        = "org.freedesktop.Secret.Error.NoSession"
	ErrNoSuchObject     = "org.freedesktop.Secret.Error.NoSuchObject"
	ErrAlreadyExists    = "org.freedesktop.Secret.Error.AlreadyExists"
	ErrNotSupported     = "org.freedesktop.Secret.Error.NotSupported"
	ErrInvalidProperty  = "org.freedesktop.DBus.Error.InvalidProperty"
	ErrUnknownInterface = "org.freedesktop.DBus.Error.UnknownInterface"
)

// NewDBusError creates a new D-Bus error
func NewDBusError(name, message string) *dbus.Error {
	return &dbus.Error{
		Name: name,
		Body: []interface{}{message},
	}
}

// ErrLocked returns an IsLocked error
func ErrLocked(msg string) *dbus.Error {
	return NewDBusError(ErrIsLocked, msg)
}

// ErrSessionNotFound returns a NoSession error
func ErrSessionNotFound(msg string) *dbus.Error {
	return NewDBusError(ErrNoSession, msg)
}

// ErrObjectNotFound returns a NoSuchObject error
func ErrObjectNotFound(msg string) *dbus.Error {
	return NewDBusError(ErrNoSuchObject, msg)
}

// ErrExists returns an AlreadyExists error
func ErrExists(msg string) *dbus.Error {
	return NewDBusError(ErrAlreadyExists, msg)
}

// ErrUnsupported returns a NotSupported error
func ErrUnsupported(msg string) *dbus.Error {
	return NewDBusError(ErrNotSupported, msg)
}
