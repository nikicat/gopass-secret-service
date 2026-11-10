---
status: todo
tags: [unsmell]
---
# Both CloseAll methods copy and reset their map by hand

## Goal
Both CloseAll methods copy the map's values under the lock, reset the map, unlock, and act on each value.

Where: internal/service/session.go:114-129, internal/service/prompt.go:112-128. Found by the /unsmell run of 2026-11-10 (repeated derivation).

## Next
Each becomes "take := slices.Collect(maps.Values(m.x)); clear(m.x)" under the lock, then a loop over take. Or use a small generic drain helper if more managers appear.
