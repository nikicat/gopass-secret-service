---
status: todo
tags: [unsmell]
---
# Introspection XML is written by hand

## Goal
Hand-written introspection XML repeats the Go method sets and the Properties interface block in both files, so it drifts whenever a method changes. godbus ships introspect.Methods, introspect.IntrospectDataString and prop.IntrospectData for this.

Where: internal/service/collection.go:160-207, internal/service/item.go:158-192. Found by the /unsmell run of 2026-11-10 (duplication).

## Next
build the XML from introspect.Node with introspect.Methods(v) and prop.IntrospectData
