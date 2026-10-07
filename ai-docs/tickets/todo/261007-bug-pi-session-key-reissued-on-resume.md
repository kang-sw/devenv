---
title: Pi lead default session key is reissued on resume and ignores the revived key
---

# Pi lead default session key is reissued on resume and ignores the revived key

## Background

Restarting Pi and resuming the same session mints a new ws session key. The
bridge's default-fill bootstrap (`src/bridge.ts`, "Default-fill key
bootstrap") calls `ferrule` on every `session_start`; only a child whose spawn
policy carries `policy.sessionKey` reuses a key. The resumed lead then holds a
key unrelated to its agenda, todos, and notes. Dogfood evidence: a downstream
lead carried two keys (a fresh "DEFAULT" and the "ORIGINAL" one holding its
state) through its compaction summary and revived both by hand to work
around this.

A second cause compounds it: the bridge's default key (`defaultKeyRef`) is
set only by that bootstrap. When the lead revives with another key through
`lead-revive` / `workflow_manual`, the default key does not follow, and the
compaction summary's `ws session key:` line (`buildLeadCompactionSummary`,
fed from the default key) keeps naming the freshly minted key instead of the
one holding the lead's state.

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

- **Adopt the key the lead revives with.** When a lead-role
  `workflow_manual` call with an explicit `session_key` succeeds and that key
  differs from the default key, the adapter makes it the default key and
  records it in the session entry above, so later resumes and compaction
  summaries carry it. A failed call or the bootstrap sentinel
  (`obsidian-latch`) adopts nothing. Only `workflow_manual` adopts: an explicit
  key passed to any other tool (for example a child's key) does not.
  This is also the legacy path: a session file written before this change has
  no key entry, so its first resume mints a key as today, and the first
  revive with the original key (from the summary or the human) adopts and
  records it; from then on the session holds one key.
  Rejected: recovering a legacy key by parsing the `ws session key:` line of
  adapter-authored compaction entries - in legacy sessions that line names
  the freshly minted default key, so it would restore the wrong key.

## Prior Art

- `src/index.ts` appends `ws-pi-fork-keys` (`current` / `previous` keys) and
  `ws-pi-fork-context` entries, and `restoreForkContext` reads entries back on
  `session_start`; the lead key follows the same pattern.

## Constraints

- A child process (spawn policy present) keeps its policy key; this change is
  for the lead's own bootstrap and revive.
- A legacy session whose original key is no longer in any summary is not
  recovered automatically; it converges once the human supplies the key and
  the lead revives with it.
- How "still resolves" is checked is not settled yet; it is decided at
  `ready/` promotion from the ws tools available then.

## Phases

### Phase 1: Persist the lead key, reuse it on resume, adopt the revived key

Record the key after bootstrap, reuse it on resume when it resolves, fall
back to minting otherwise, and adopt the key of a successful explicit
`workflow_manual` call. Test resume reuse, the unresolvable-key fallback,
`startup`/`new` still minting, revive adoption (default key, session entry,
and the next compaction summary's key line), no adoption on a failed call,
the sentinel, or another tool's explicit key, the legacy session converging
on its first revive, and the child policy path unchanged.
