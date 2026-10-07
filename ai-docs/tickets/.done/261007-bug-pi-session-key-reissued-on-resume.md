---
title: Pi lead default session key is reissued on resume and ignores the revived key
related:
  261007-bug-pi-adapter-messages-read-as-user-text: sibling; why the key-change line rides the tool result rather than a custom message
sage-review-completeness: completed
sage-review-completeness-reviewed: 334afb51b553243a
sage-review-design: completed
sage-review-design-reviewed: e93543b47318b9f6
completed: 2026-10-07
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
  through a `relogin_session_key` argument left out of the advertised
  `tools/list` schema. When
  that key is a parent-less lead-capability key bound to the same canonical
  root as the call's `root`, `ferrule` mints nothing, rebinds the mailbox
  owner to that key under the existing rules (including never stealing from
  another live process), and returns the same key. Any other key (unknown,
  parent-carrying, delegate or leaf, another root) is refused with an error.
  Rejected: an advertised parameter - an agent that forgets its key mints a
  new one by convention, and an advertised re-login invites re-logging with
  another session's key to take its mailbox; Pi's bridge is the only intended
  caller. Rejected: naming it `session_key` - the bridge's
  `resolveSessionKey` fills `session_key` into every bridged call that omits
  it, `ferrule` included, so every agent `ferrule` would become a re-login
  with the default key; `existing_session_key` - names the input rather than
  the action. Rejected: making `workflow_manual` rebind the owner as a side
  effect - it puts a write behind a read tool.
- **Persist and reuse the key through the Pi session file, scoped by session
  id.** After the bootstrap settles a lead key, the adapter records it as a
  custom session entry (`pi.appendEntry`, customType `ws-pi-lead-key`, named
  after the fork path's `ws-pi-fork-keys`) carrying the key and the current
  `sessionManager.getSessionId()`; nothing is appended when the newest
  matching entry already holds the same key. At every lead bootstrap (no spawn
  policy),
  the adapter reads the newest such entry whose `sessionId` equals the current
  session id from `ctx.sessionManager.getEntries()`; when one exists it
  re-logs in with that key, and on success that key is the default key. When
  none exists, or the re-login is refused, the bootstrap mints a fresh key as
  today; a refusal also raises a UI warning (`notify`) only. The trigger is
  the entry, not `SessionStartEvent.reason`: reopening a session file
  (`pi -c`, `pi -r`, `pi --session`) starts the first runtime with no event,
  which Pi defaults to `reason: "startup"` (`core/agent-session.js`), and
  `/reload` re-runs the bootstrap in the same session; `reason: "resume"`
  fires only on an in-process session switch. Pi keeps the header's session
  id when it reopens a file and mints a new one for `/fork`, `pi --fork`, and
  `new` (`core/session-manager.js`), so the session-id match lets a restart
  or reload of the same session reuse its key while a fork, whose file
  inherits the lead's entries, mints its own key, as
  260904-feat-ws-pi-side-thread-fork-question-surface decided; this is the
  same filter `restoreForkKeys` applies to `ws-pi-fork-keys`. Rejected:
  gating reuse on `reason: "resume"` - it misses the restart and reload cases
  this ticket is about; reusing the newest entry without the session-id
  match - a fork would re-log in with the lead's key, which the re-login
  accepts (parent-less, same root), and take the lead's mailbox. Rejected:
  recording only the problem and deferring the mechanism - the session file
  is exactly the resume unit, and the fork path already persists its keys the
  same way.
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
  `[system message from ws-pi-plugin] Default session key changed: <prev> -> <now>. Calls that omit session_key and compaction summaries now use <now>.`
  Nothing is appended when the revived key already is the default key. The
  prefix is the shared `ADAPTER_MESSAGE_LABEL` constant, imported rather than
  restated, so the two cannot drift.
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
  unchanged.
- A child process (spawn policy present) keeps its policy key; this change is
  for the lead's own bootstrap, resume, and revive.
- `/import` of a copied session file keeps the file's session id, so the
  copy re-logs in with the original's key; the re-login's rule against
  taking a mailbox from a live process keeps that from becoming a takeover.
  Record this at the re-login handler comment.
- A legacy session whose original key is no longer in any summary is not
  recovered automatically; it converges once the human supplies the key and
  the lead revives with it.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 260913-research-cross-session-mailbox (2026-09-13, Confirmed Decisions): "owner binding: rebind owner on each ferrule call where WS_MAILBOX is set and parent_session_key == null; worker/delegate ferrule calls (parent present) never bind. ... Trigger is parent-less ferrule" — bearing: constrains
- 2cc292b7 (2026-10-07, commit): "Finding: skipping ferrule on resume would leave the mailbox owner empty, because a new MCP process re-registers presence with no Owner and only a parent-less lead ferrule sets it" — bearing: supports
- 963ef404 (2026-10-07, commit): "Finding: the bridge default key is set only at bootstrap, so a lead-revive with another key never moves it and the compaction summary keeps naming the freshly minted key" — bearing: supports
- 260926-bug-pi-execute-worker-steals-mailbox-owner (2026-09-26, Decisions): "A (pi): the execute-worker spawn carries the lead's session key. ws-execute's spawn passes a parentPolicy with sessionKey: bridge.defaultSessionKeyRef.current, matching the other lead spawn sites" — bearing: constrains
- 260904-feat-ws-pi-side-thread-fork-question-surface (2026-09-04, Decisions): "ws session_key: the fork gets its own lead-scope key, never the lead's. Every Pi process's bridge already mints its own key at start (ferrule, bridge.ts default-fill)" — bearing: constrains
- 260904-feat-ws-pi-lead-bootstrap-system-prompt (2026-09-04, Decisions): "Degraded path: if the session_start ferrule bootstrap or the manual fetch failed (own key unset or no snapshot), §2's sentinel rewrite and this mapping are both disabled for the session" — bearing: constrains
- 2121cd5a (2026-09-02, 260902-feat-ws-pi-native-mvp commit): "a blanket auto-login per subagent mints an unrelated key and breaks ws-mcp lineage; and hiding the key at lead scope breaks the common case where the lead orchestrates multiple implementation tracks" — bearing: constrains
- 260605-epic-ws-playbook-factory-pivot (2026-09-04, a2e42f9c commit): "D-C keep session_key across auto-resume (agent_id->{pi session, ws key} mapping; ws-agent-stop halts but retains the mapping so a child is dormant/resumable)" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/server.go ferrule handler, agents-plugin-tool/internal/mcp/mailbox_runtime.go, agents-plugin-pi/src/bridge.ts, agents-plugin-pi/src/index.ts, agents-plugin-pi/src/lead-compaction.ts |
| scope.surface | cross-module | ws-mcp ferrule handler semantics plus Pi adapter bridge bootstrap and session entries |
| scope.new_public_symbol | no | none; the re-login argument is deliberately absent from the tools/list schema |
| scope.new_type_contract | yes | hidden ferrule re-login argument `relogin_session_key`, and a new Pi custom session entry type for the lead key |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/session_auth_test.go, mailbox_runtime_test.go, server_test.go; agents-plugin-pi/test/bridge.test.ts, fork-context.test.ts, lead-compaction.test.ts |
| complexity.reuse_points | confirmed | rebindMailboxOwnerAtFerrule at agents-plugin-tool/internal/mcp/mailbox_runtime.go#L468 and the ws-pi-fork-keys appendEntry pattern at agents-plugin-pi/src/index.ts#L1045 |
| complexity.side_effect_risk | high | changes mailbox owner rebinding and the lead default key used by every omitted-session_key call |
| risk.correctness | high | wrong key acceptance or adoption could steal another session's mailbox or bind the lead to foreign state |
| risk.fit | moderate | hidden schema argument departs from the advertised tool surface and must match ferrule rebind rules |
| risk.test | moderate | resume, refusal, legacy, sentinel, and child-policy paths each need dedicated cases across Go and TS suites |
| risk.security_or_contract | high | re-login with an existing key is an authentication path; refusal rules for parent, capability, and root guard mailbox takeover |

## Phases

### Phase 1: Hidden existing-key re-login on ferrule (ws MCP)

Add the re-login argument to the `ferrule` handler. Test: the same key comes
back and no key is minted; the mailbox owner moves to it; an unknown,
parent-carrying, delegate or leaf, or other-root key is refused without a
mint or owner change; a live other process's presence is not stolen; the
argument is absent from `tools/list`.

### Result (e08398238) - 2026-10-07

Landed in `agents-plugin-tool/internal/mcp/server.go`: `handleLeadLogin`
branches to `handleLeadRelogin` when `relogin_session_key` is set. The key
must be known, parent-less, lead-scope, and bound to the call's canonical
root; it is then passed to `rebindMailboxOwnerAtFerrule` and returned through
the shared `leadLoginResponse` tail (same text/JSON shape and bootstrap alarm
as a mint). The handler comment records why the argument is hidden and the
`/import` copied-file case. The `tools/list` schema is untouched.

Decisions: a re-login that also carries `parent_session_key` or a non-lead
`capability` is refused rather than ignored (those arguments describe a key
the re-login could never return). A re-login whose owner rebind is refused
by the live-holder rule still returns the key; the refusal is the rebind's,
not the call's.

Verification: `go test ./...` in `agents-plugin-tool`: all packages ok (mcp
279s). New tests: `TestFerruleReloginReturnsSameKeyWithoutMint`,
`TestFerruleReloginRefusesForeignKeys` (unknown, parent-carrying, delegate,
leaf, other-root, combined parent, combined capability; no mint),
`TestFerruleSchemaOmitsReloginArgument`,
`TestMailboxReloginRebindsOwnerToExistingKey` (owner moves; refused keys
leave it), `TestMailboxReloginRefusesLiveDifferentPIDHolder`.

### Phase 2: Persist, reuse, and adopt the lead key (Pi)

Builds on Phase 1. The bootstrap in `startBridge` (`src/bridge.ts`) gains
access to the session's entries and id. Record the key with its session id
after bootstrap, re-log in with the matching entry at every lead bootstrap,
fall back to minting with a UI warning on refusal, adopt a revived key through
re-login, and append the pinned change line. Test: reuse with no mint and an
owned mailbox when the session file holds an entry for its own session id,
across `startup` (reopened file), `reload`, and `resume`; minting when no
entry matches - a `new` session, and a fork (`/fork` or `pi --fork`) whose
file carries the lead's entry under the lead's session id; the refused-key
fallback; revive adoption (default key, session entry, pinned line, and the
next compaction summary's key line); no adoption on a refused re-login, a
failed call, the sentinel, or another tool's explicit key; no line when the
key is unchanged; the legacy session converging on its first revive; and the
child policy path unchanged.

### Result (dc698ea04) - 2026-10-07

Landed in `agents-plugin-pi/src/bridge.ts` and `src/index.ts`:
`BridgeOptions.sessionId` (passed from `ctx.sessionManager.getSessionId()`),
`LEAD_KEY_ENTRY = "ws-pi-lead-key"`, `restoreLeadKey`, `revivedKeyToAdopt`,
`buildDefaultKeyChangedLine`, and `reloginLeadKey`. The bootstrap re-logs in
with the newest entry for the session id before minting; a refusal notifies
a warning and mints. The settled key is recorded unless the newest entry
already holds it. A lead-role `workflow_manual` (mapped and verbatim paths)
that succeeds with an explicit non-sentinel key other than the default
re-logs in with it; on success the default key, `knownKeys`, and the session
entry follow, and one line prefixed with the imported `ADAPTER_MESSAGE_LABEL`
is appended after the dedupe so a pointer result still carries it. The
compaction summary reads the live default key ref, so it names the adopted
key without further change.

Decisions: the ownership gate is `!policy && readSpawnRole() === undefined &&
sessionId` (every spawned child, forks included, carries a delegation
policy, and a fork's readiness check treats its previous own key as stale).
A degraded bootstrap (no default key) still adopts a revived key, with
`(unset)` as the previous key. `reloginLeadKey` treats a returned key other
than the requested one as a refusal. Tests live in a new
`test/lead-key-resume.test.ts` that drives the production `startBridge`
against a journaling fake ws-mcp, rather than growing `bridge.test.ts`.

Verification: `npm test` in `agents-plugin-pi`: 2073 tests, 2070 pass, 0
fail, 3 skipped. The new file covers reuse with no mint across the three
start reasons (the trigger is the entry, which none of them changes), the
newest entry winning, minting for a new session and for a fork carrying the
lead's entry, the refused-key fallback with its warning, no session id, the
child policy paths, revive adoption (default key, entry, pinned line, filled
omitted key, compaction summary line) on both dispatch paths, no adoption on
a refused re-login, a failed call, the sentinel, or another tool's explicit
key, no line when unchanged, and the legacy session converging on its first
revive and reusing that key on the next restart.

Review (lite, one pass): no Critical or Important findings; two Minor
findings (owner-unchanged coverage for parent-carrying and other-root
refusals; a JSDoc displaced by the insertion) fixed in the follow-up commit.

## Sage Review Round 1 (2026-10-07)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | The resume reason misses the restart case the ticket is about: pi -c / -r / --session start with reason startup, and /reload re-runs the bootstrap; resume fires only on in-process session switches | important | missing |
| 2 | Lead-key entry needs session scoping: pi --fork and Pi /fork copy entries, so a startup-triggered reuse would let a fork re-log in with the lead key and take its mailbox | important | missing |
| 3 | startBridge needs the start reason and the session entries | minor | autonomous |
