---
title: Investigate Windows mailbox store read sharing violation
---

# Investigate Windows mailbox store read sharing violation

## Background

Windows dogfooding reported that `ws-mcp mailbox wait` stopped with `read mailbox store ...\mailbox.json: open ... The process cannot access the file because it is being used by another process.` The exact holder and native error code have not been captured. This is a read/open failure, not evidence that the separate mailbox writer lock timed out.

`agents-plugin-tool/internal/wsmailbox/store.go` reads the store without a retry or the writer lock. `wait.go` terminates a wait on any peek error; `cmd/ws-mcp/mailbox.go` surfaces that error. The writer lock serializes cooperating writers, not all readers or external processes. Existing mailbox tests do not exercise a transient sharing violation on a read. The Windows replacement helper under `internal/wsstate/` addresses a different write-side failure.

## Phases

### Phase 1: Isolate transient store-read failure and restore reliable waiting

Reproduce or inject a Windows store-open sharing violation, distinguish it from parse corruption and nontransient I/O errors, and determine a bounded recovery policy for `mailbox wait` and any other affected read entry point. Verify that a transient failure can recover without losing an arrival, while persistent and unrelated failures remain visible. Investigate write-side replacement contention separately rather than assuming it caused this read failure.
