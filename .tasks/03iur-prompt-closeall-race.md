---
status: todo
tags: [bug, unsmell]
---
# PromptManager.CloseAll writes m.prompts after unlocking

## Goal
PromptManager.CloseAll assigns m.prompts again after m.mu.Unlock at :127. The write is not guarded and races with CreatePrompt and onComplete. It is also redundant, because :120 already reset the map under the lock.

Where: internal/service/prompt.go:127. Found by the /unsmell run of 2026-11-10 (oversized lock).

## Next
Delete prompt.go:127.
