---
status: todo
tags: [unsmell]
---
# D-Bus interface names are spelled at every export

## Goal
"org.freedesktop.DBus.Introspectable" is spelled at 11 sites and "org.freedesktop.DBus.Properties" at 8. Every site is a line the changeset rewrote, while internal/dbus/types.go:17-29 already names the Secret interfaces as constants.

Where: internal/service/session.go:82, internal/service/session.go:157, internal/service/session.go:181, internal/service/prompt.go:84, internal/service/prompt.go:181, internal/service/service.go:134. Found by the /unsmell run of 2026-11-10 (duplication).

## Next
Add IntrospectableInterface and PropertiesInterface to internal/dbus/types.go, next to SessionInterface and the other names, and use them at every site.
