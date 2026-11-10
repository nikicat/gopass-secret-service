---
status: doing
tags: [bug, upstream]
---
# godbus reads its object table without a lock

## Goal
godbus (v5.1.0 to master) reads the table of exported objects without its lock in `PathExists`, `introspectPath` and `Conn.export`. Concurrent exports, or one export while a client introspects, kill gopass-secret with `concurrent map read and map write` (seen 2026-11-10). Done means gopass-secret-service builds against a godbus with the table locked and no `replace` to a local path.

## Next
Fixed in nikicat/dbus `fix/object-table-races` (b3713a7), upstream PR https://github.com/godbus/dbus/pull/461. go.mod replaces godbus with that commit, which is godbus master plus the fix. When a godbus release has the fix: drop the replace and require that release.

## Decisions
- Fix godbus itself rather than lock around exports in this repo (2026-11-10). The lock-around-exports version is in `.git/unsmell/20261110-145218/fixes/exportmu-wrapper.patch`; it cannot cover introspection, which runs on godbus's dispatch goroutine.
