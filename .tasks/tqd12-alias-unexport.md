---
status: todo
tags: [bug, unsmell]
---
# Collection.Unexport leaves its alias paths exported

## Goal
Unexport removes only c.path. Alias exports made by ExportAtPath (service.go:539, for default and session) stay bound after the collection is deleted, so /aliases/default keeps serving a collection that no longer exists. Collection keeps no record of its alias paths.

Where: internal/service/collection.go:217-225, internal/service/collection.go:228-296. Found by the /unsmell run of 2026-11-10 (bug).

## Next
Collection records the alias paths it is exported at, and Unexport unexports each of them.
