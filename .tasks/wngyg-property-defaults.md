---
status: todo
tags: [unsmell]
---
# Get and GetAll derive property values separately

## Goal
Get and GetAll each derive the property values and their missing-data defaults separately, in both handlers (collection.go:68-86 vs 97-106, item.go:52-60 vs 91-97).

Where: internal/service/collection.go:52-114, internal/service/item.go:43-116. Found by the /unsmell run of 2026-11-10 (repeated derivation).

## Next
GetAll builds the map once, and Get indexes it
