---
status: todo
tags: [unsmell]
---
# attributesMatch and a prefix test redo maps.Equal and strings.HasPrefix

## Goal
attributesMatch is maps.Equal, and the prefix test in RemoveCollection is strings.HasPrefix

Where: internal/service/collection.go:479-489, internal/service/item.go:383. Found by the /unsmell run of 2026-11-10 (inline generic machinery).

## Next
use maps.Equal and strings.HasPrefix, and delete attributesMatch
