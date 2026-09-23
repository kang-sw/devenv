---
title: Investigate Windows mailbox store read sharing violation
---

# Investigate Windows mailbox store read sharing violation

## Background

Windows dogfooding reported a Pi warning from `ws-mcp mailbox wait`: `read mailbox store ...\mailbox.json: open ... The process cannot access the file because it is being used by another process.` The exact holder and native error code have not been captured. This is a read/open failure, not evidence that the separate mailbox writer lock timed out. The user subsequently observed the waiter retrying.

`agents-plugin-tool/internal/wsmailbox/store.go` reads the store without a retry or the writer lock. `wait.go` terminates an individual wait invocation on a peek error; `cmd/ws-mcp/mailbox.go` surfaces that error. In Pi, `agents-plugin-pi/src/mailbox-waiter.ts` supervises the subprocess and restarts it after the error backoff (default 5 seconds), consistent with the observed retry. This is not a retry inside the store read; a transient error can recover on the next invocation while still emitting a warning and causing delay. The writer lock serializes cooperating writers, not all readers or external processes. Existing mailbox tests do not exercise a transient sharing violation on a read. The Windows replacement helper under `internal/wsstate/` addresses a different write-side failure.

## Phases

### Phase 1: Isolate transient store-read failure and restore reliable waiting

Reproduce or inject a Windows store-open sharing violation, distinguish it from parse corruption and nontransient I/O errors, and assess whether the existing Pi subprocess restart/backoff is sufficient or a narrower read-side recovery is warranted for `mailbox wait` and other affected entry points. Verify that a transient failure can recover without losing an arrival or spamming warnings, while persistent and unrelated failures remain visible. Investigate write-side replacement contention separately rather than assuming it caused this read failure.
