---
title: lead-revive should offer in-memory session-key candidates on lost key
---

# lead-revive should offer in-memory session-key candidates on lost key

## Context

Found during a v0.31.1 dogfooding pass. Session state (agenda, todo,
session-tree) is bound per session key, not per working root: an agenda set
under one key was invisible when reloading `workflow_manual` under a second
key minted for the same root. If a lead loses its session key (e.g. after a
compaction that drops the summary line), re-minting via `ferrule` silently
returns a clean, empty session — no error, no hint that other state exists for
that root. The lost key strands all of its agenda/todo/session-tree state with
no recovery path short of guessing or grepping transcripts.

This is a reasonable-expectation surprise: a caller who loses a key expects
either an error/warning or some way to reconnect to prior state for the same
root, not silent success on an empty session.

## Suggestion

In `lead-revive` (or the underlying MCP primitive it calls), if the MCP
process still holds transient in-memory session keys bound to the same
working root, surface a few of them as recovery candidates. This requires no
persistent-storage access and no cross-session intrusion risk — only keys
already live in this process's memory are considered, so it cannot leak state
across process restarts or unrelated roots.

This is a complement to, not a replacement for, the existing "preserve the key
verbatim in the compaction summary" discipline — that remains the primary
path; this is a fallback when the summary path fails.
