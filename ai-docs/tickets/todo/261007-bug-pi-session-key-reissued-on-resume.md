---
title: Pi lead session key is reissued when a session is resumed
---

# Pi lead session key is reissued when a session is resumed

## Background

Restarting Pi and resuming the same session mints a new ws session key. The
bridge's default-fill bootstrap (`src/bridge.ts`, "Default-fill key
bootstrap") calls `ferrule` on every `session_start`; only a child whose spawn
policy carries `policy.sessionKey` reuses a key. The resumed lead then holds a
key unrelated to its agenda, todos, and notes. Dogfood evidence: a downstream
lead carried two keys (a fresh "DEFAULT" and the "ORIGINAL" one holding its
state) through its compaction summary and revived both by hand to work
around this.

## Decisions

- **Persist and reuse the key through the Pi session file.** After the
  bootstrap settles a lead key, the adapter records it as a custom session
  entry (`pi.appendEntry`). On `session_start` with `reason: "resume"`
  (`SessionStartEvent.reason` in `pi-coding-agent`), the adapter reads the
  newest such entry from `ctx.sessionManager.getEntries()`; when the key still
  resolves in the ws store it becomes the default key and `ferrule` is
  skipped, and otherwise the bootstrap mints a fresh key as today. Rejected:
  recording only the problem and deferring the mechanism - the session file is
  exactly the resume unit, and the fork path already persists its keys the
  same way.

## Prior Art

- `src/index.ts` appends `ws-pi-fork-keys` (`current` / `previous` keys) and
  `ws-pi-fork-context` entries, and `restoreForkContext` reads entries back on
  `session_start`; the lead key follows the same pattern.

## Constraints

- A child process (spawn policy present) keeps its policy key; this change is
  for the lead's own bootstrap.
- How "still resolves" is checked is not settled yet; it is decided at
  `ready/` promotion from the ws tools available then.

## Phases

### Phase 1: Persist the lead key and reuse it on resume

Record the key after bootstrap, reuse it on resume when it resolves, and fall
back to minting otherwise; test resume reuse, the unresolvable-key fallback,
`startup`/`new` still minting, and the child policy path unchanged.
