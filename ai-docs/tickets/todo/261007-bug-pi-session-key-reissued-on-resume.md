---
title: Pi lead default session key is reissued on resume and ignores the revived key
---

# Pi lead default session key is reissued on resume and ignores the revived key

## Background

Restarting Pi and resuming the same session mints a new ws session key. The
bridge's default-fill bootstrap (`agents-plugin-pi/src/bridge.ts`,
"Default-fill key bootstrap") calls `ferrule` on every `session_start`; only a
child whose spawn policy carries `policy.sessionKey` reuses a key. The resumed
lead then holds a key unrelated to its agenda, todos, and notes. Dogfood
evidence: a downstream lead carried two keys (a fresh "DEFAULT" and the
"ORIGINAL" one holding its state) through its compaction summary and revived
both by hand to work around this.

A second cause compounds it: the bridge's default key (`defaultKeyRef`) is
set only by that bootstrap. When the lead revives with another key through
`lead-revive` / `workflow_manual`, the default key does not follow, and the
compaction summary's `ws session key:` line (`buildLeadCompactionSummary`,
fed from the default key) keeps naming the freshly minted key instead of the
one holding the lead's state.

Simply skipping `ferrule` on resume is not enough, because of the mailbox
owner. The mailbox name comes from `WS_MAILBOX`; which session key may drain
it is the presence record's `Owner`. A new MCP process re-registers the
mailbox with an empty `Owner` (`agents-plugin-tool/internal/mcp/
mailbox_runtime.go`, `ensureMailboxRegistered` / `ownedMailboxPresence`), and
only a parent-less lead `ferrule` sets it (`rebindMailboxOwnerAtFerrule`).
Today every restart's bootstrap `ferrule` refills it; a resume that skips
`ferrule` would leave the mailbox drainable by no key.

## Decisions

- **Hidden re-login on `ferrule` (ws MCP).** `ferrule` accepts an existing key
  through an argument left out of the advertised `tools/list` schema. When
  that key is a parent-less lead-capability key bound to the same canonical
  root as the call's `root`, `ferrule` mints nothing, rebinds the mailbox
  owner to that key under the existing rules (including never stealing from
  another live process), and returns the same key. Any other key (unknown,
  parent-carrying, delegate or leaf, another root) is refused with an error.
  Rejected: an advertised parameter - an agent that forgets its key mints a
  new one by convention, and an advertised re-login invites re-logging with
  another session's key to take its mailbox; Pi's bridge is the only intended
  caller. Rejected: making `workflow_manual` rebind the owner as a side
  effect - it puts a write behind a read tool.
- **Persist and reuse the key through the Pi session file.** After the
  bootstrap settles a lead key, the adapter records it as a custom session
  entry (`pi.appendEntry`). On `session_start` with `reason: "resume"`
  (`SessionStartEvent.reason` in `pi-coding-agent`), the adapter reads the
  newest such entry from `ctx.sessionManager.getEntries()` and re-logs in
  with it; on success it is the default key, and on refusal the bootstrap
  mints a fresh key as today and raises a UI warning (`notify`) only.
  Rejected: recording only the problem and deferring the mechanism - the
  session file is exactly the resume unit, and the fork path already persists
  its keys the same way.
- **Adopt the key the lead revives with.** When a lead-role
  `workflow_manual` call with an explicit `session_key` succeeds and that key
  differs from the default key, the adapter re-logs in with it and the Pi
  session's root; on success the key becomes the default key and is recorded
  in the session entry above, so later resumes and compaction summaries carry
  it. A refused re-login, a failed `workflow_manual`, or the bootstrap
  sentinel (`obsidian-latch`) adopts nothing, and an explicit key passed to
  any other tool (for example a child's key) is never adopted. The re-login's
  root and parent-less conditions are what keep another worktree's track key,
  or a parent-carrying track key, from replacing the default key. This is also
  the legacy path: a session file written before this change has no key
  entry, so its first resume mints a key as today, and the first revive with
  the original key (from the summary or the human) adopts it, records it, and
  takes the mailbox back; from then on the session holds one key.
  Rejected: recovering a legacy key by parsing the `ws session key:` line of
  adapter-authored compaction entries - in legacy sessions that line names
  the freshly minted default key, so it would restore the wrong key.
- **Tell the lead when its default key changes.** An adoption that changes the
  default key appends one line to that same `workflow_manual` tool result,
  pinned:
  `[ws-pi-plugin] Default session key changed: <prev> -> <now>. Calls that omit session_key and compaction summaries now use <now>.`
  Nothing is appended when the revived key already is the default key.
  Rejected: a separate custom message - Pi delivers it as user-role text,
  indistinguishable from the human (`261007-bug-pi-adapter-messages-read-as-user-text`).

## Prior Art

- `agents-plugin-pi/src/index.ts` appends `ws-pi-fork-keys` (`current` /
  `previous` keys) and `ws-pi-fork-context` entries, and `restoreForkContext`
  reads entries back on `session_start`; the lead key follows the same
  pattern.
- `rebindMailboxOwnerAtFerrule` already holds the owner-rebind rules the
  re-login reuses.

## Constraints

- The re-login argument is absent from the `tools/list` schema; a comment at
  the handler records why, and the agent-visible `ferrule` surface is
  unchanged. Its argument name is chosen at `ready/` promotion.
- A child process (spawn policy present) keeps its policy key; this change is
  for the lead's own bootstrap, resume, and revive.
- A legacy session whose original key is no longer in any summary is not
  recovered automatically; it converges once the human supplies the key and
  the lead revives with it.
- Manuals: `ai-docs/manuals/ws-mcp.md` and
  `ai-docs/manuals/shipped-surface-boundary.md` for the
  `agents-plugin-tool/internal/mcp/` change.

## Phases

### Phase 1: Hidden existing-key re-login on ferrule (ws MCP)

Add the re-login argument to the `ferrule` handler. Test: the same key comes
back and no key is minted; the mailbox owner moves to it; an unknown,
parent-carrying, delegate or leaf, or other-root key is refused without a
mint or owner change; a live other process's presence is not stolen; the
argument is absent from `tools/list`.

### Phase 2: Persist, resume, and adopt the lead key (Pi)

Builds on Phase 1. Record the key after bootstrap, re-log in with it on
resume, fall back to minting with a UI warning on refusal, adopt a revived
key through re-login, and append the pinned change line. Test: resume reuse
(no mint, mailbox owned), the refused-key fallback, `startup`/`new` still
minting, revive adoption (default key, session entry, pinned line, and the
next compaction summary's key line), no adoption on a refused re-login, a
failed call, the sentinel, or another tool's explicit key, no line when the
key is unchanged, the legacy session converging on its first revive, and the
child policy path unchanged.
