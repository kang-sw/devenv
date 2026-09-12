---
title: todo.add accepts a call that omits the required title and silently stores an empty item
---

# todo.add accepts a call that omits the required title and silently stores an empty item

## Background

Dogfood surprise (2026-09-08, Claude Code host, ws-mcp on the Pi track).
The advertised `todo.add` schema lists `title` as required and has no `text`
field. A lead called `todo.add(session_key, key, text: "...")` — wrong field
name, no `title` — six times; every call returned `todo added: <key>` and the
items were created with an empty title (`todo.read` → `{"title": "",
"status": "pending", "instruction": null}`; `todo.list` rendered
`- [ ] {odq-01} ` with nothing after the key). No validation error was
raised for the missing required field or the unknown one, so the mistake
surfaced only later, when the list was re-read and the queue items turned out
to be unreadable.

## Phases

### Phase 1: Reject todo.add calls that violate the advertised schema

Investigate why the `todo.add` handler does not enforce its own schema
(`title` required; unknown top-level fields). Make a missing or empty `title`
a fail-loud error and decide whether unknown fields are rejected or reported,
consistent with how other ws tools treat schema violations. Check the
sibling `todo.*` mutation tools for the same gap.
