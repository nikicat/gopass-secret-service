---
status: todo
tags: [unsmell]
---
# SetSecret, setAttributes and setLabel repeat read, update and notify

## Goal
SetSecret, setAttributes and setLabel each repeat GetItem, mutate, UpdateItem and emitItemChanged. Item.mu is held by SetSecret but not by the other two, so it serializes nothing consistently.

Where: internal/service/item.go:263-328. Found by the /unsmell run of 2026-11-10 (duplication).

## Next
one i.update(func(*store.ItemData)) *dbus.Error that holds i.mu, used by all three
