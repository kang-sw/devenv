---
title: "Make ws-claude timeout resume handles internally consistent"
---

# Make ws-claude timeout resume handles internally consistent

## Background

During Pi dogfooding, a `ws-claude` Opus consult timed out and returned the identifier `amber-amber-amber`. An immediate follow-up passed that exact identifier through `resume`, but the tool rejected it with `unknown_resume`. The tool contract advertises continuation of returned handles, so returning an unusable handle leaves the caller unable to tell whether the timed-out request can be resumed or must be restarted.

Observed sequence:

1. A fresh `ws-claude` consult returned `status: error`, `code: timeout`, and `id: amber-amber-amber`.
2. The next call used `resume: amber-amber-amber`.
3. The call returned `code: unknown_resume` in the same Pi session.

## Phases

### Phase 1: Reconcile timeout handle creation and resume behavior

Reproduce the timeout path and trace where request identifiers become resumable handles. Make the returned shape and continuation behavior internally consistent without weakening isolation between sessions. Cover successful continuation, timeout, unknown-handle, and non-resumable error cases in tests. Verification must show that callers can determine from the first response whether retrying through `resume` is supported.
