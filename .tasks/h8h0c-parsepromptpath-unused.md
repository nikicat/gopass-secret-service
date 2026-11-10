---
status: todo
tags: [unsmell]
---
# service.ParsePromptPath is an unused pass-through

## Goal
service.ParsePromptPath only calls dbtypes.ParsePromptPath, and nothing in the repo calls it.

Where: internal/service/prompt.go:107-110. Found by the /unsmell run of 2026-11-10 (pass-through).

## Next
Delete it.
