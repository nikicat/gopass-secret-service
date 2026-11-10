---
status: todo
tags: [unsmell]
---
# Doc comments in collection.go and item.go open with their own name

## Goal
38 doc comments in the two files open with their declaration's name ("NewCollection creates…", "Path returns…"), and no enabled linter requires it. Getter docs such as Path and Name restate the signature.

Where: internal/service/collection.go:33, internal/service/item.go:23. Found by the /unsmell run of 2026-11-10 (tautological comment).

## Next
drop the leading name, and delete the getter docs that only restate the signature
