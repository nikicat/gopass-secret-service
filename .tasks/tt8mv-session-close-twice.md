---
status: todo
tags: [unsmell]
---
# Session.close and closeWithoutCallback repeat one body

## Goal
close and closeWithoutCallback repeat the same body (the closed check, closed=true, the two unexport lines the changeset rewrote at :156-157 and :180-181, and crypto.Close). They differ only in onClose and in which one takes s.mu.

Where: internal/service/session.go:149-185. Found by the /unsmell run of 2026-11-10 (duplication).

## Next
Add one release() that holds the shared body and is called with s.mu held. close() is release() and then onClose. closeWithoutCallback locks and calls release().
