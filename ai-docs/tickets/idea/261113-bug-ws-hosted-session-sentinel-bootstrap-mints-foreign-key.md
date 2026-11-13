---
title: "Hosted session sentinel bootstrap mints a foreign key"
---

# Hosted session sentinel bootstrap mints a foreign key

## Background

Peer InspectTGV lead report, 2026-11-13 (wsflow 0.49.0): the lead-discuss
preload instructs a fresh session to call `workflow_manual` with sentinel key
`obsidian-latch` and a root path. In a hosted session with
`WS_HOSTED_SESSION_KEY` set, this minted a new key without warning. Calls with
that key to `note.query`, `mailbox.lookup_peers`/`recv`, and
`whiteboard.query`/`update` were accepted; the update wrote a separate board
under `ai-docs/whiteboards/`, leaving the user's dashboard unchanged, and the
lead gave the user the wrong mailbox reply-id. A later `workflow_manual` call
with the minted key refused it and instructed use of the fixed hosted key.

## Idea

Have sentinel bootstrap return the hosted key in a hosted session, or have
every tool refuse a non-hosted key as `workflow_manual` does.

## Open

- What compatibility tier applies at promotion?
