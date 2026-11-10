---
status: todo
tags: [unsmell]
---
# The live-properties comment is repeated six times

## Goal
"Properties are read from the store on every access, not cached" is stated six times. The type comments run 9 and 7 lines and tell the design history (prop.Export, SetMust).

Where: internal/service/collection.go:18-24, internal/service/collection.go:41-47, internal/service/collection.go:152-153, internal/service/collection.go:506-508, internal/service/item.go:37-38, internal/service/item.go:150. Found by the /unsmell run of 2026-11-10 (duplicated knowledge).

## Next
the fact lives once on collectionPropsHandler in two lines; the other sites drop it or point there
