---
status: todo
tags: [unsmell]
---
# CreateItem parses D-Bus properties by key and branches inline

## Goal
CreateItem parses raw D-Bus properties by key (collection.go:357-378), finds a duplicate, and runs three create or replace branches inline in one 120-line body.

Where: internal/service/collection.go:340-463. Found by the /unsmell run of 2026-11-10 (long function).

## Next
parse properties into an itemProps struct at the edge, and split out findExisting
