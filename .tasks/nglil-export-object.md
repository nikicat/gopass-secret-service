---
status: todo
tags: [unsmell]
---
# Five exporters each export, introspect and roll back by hand

## Goal
Five functions (CreateSession, CreatePrompt, Item.Export, Collection.Export and ExportAtPath) each export one or two interfaces and an introspect object at a path, and undo the earlier exports by hand when a later one fails. Each also lists its interfaces again in its unexport.

Where: internal/service/session.go:70-86, internal/service/prompt.go:65-88. Found by the /unsmell run of 2026-11-10 (repeated derivation).

## Next
Add exportObject(conn, path, ifaces map[string]any) error to export.go. It exports all the interfaces under one exportMu hold and unexports the ones it already exported when one fails. Its pair, unexport(conn, path, ifaces...), comes from b1-3.
