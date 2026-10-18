---
title: Inject the ws workflow manual and dedupe playbook re-reads from the Claude mod
blocked-by: 261018-feat-claude-mod-mcp-reconnect-lever
related:
  261018-research-claude-mod-ws-manual-injection: source research; its Verified Findings are this ticket's evidence, and its Proposals became the Decisions below through the user's 2026-10-18 approval
  261017-feat-claude-mod-lead-compaction-timing: the single Claude mod this feature joins; its compaction observer and lever landing reset the snapshot flag, and its ws-only package gate is reused
  261018-feat-claude-mod-mcp-reconnect-lever: sibling feature in the same mod; runs first (both edit claude-mod/register.ts)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 18ee2e9c52e05bbd
sage-review-completeness-reviewed: 18ee2e9c52e05bbd
completed: 2026-10-18
---

# Inject the ws workflow manual and dedupe playbook re-reads from the Claude mod

## Background

pi gives a ws lead three things the Claude side lacks: the static workflow
manual sits in the system prompt (`agents-plugin-pi/src/lead-bootstrap.ts`
`staticManualPart` / `buildWsBlock`), a `workflow_manual` call returns only
the session state behind a fixed line (`agents-plugin-pi/src/bridge.ts`
`prependWorkflowStateLine` / `cutStaticBody`, with a `workflow_state`
fallback), and repeat reads whose text is unchanged collapse to a pointer
(`agents-plugin-pi/src/playbook-read-dedupe.ts`). On Claude every lead skill
re-reads its playbook and the manual in full, and the manual re-enters the
context on every `workflow_manual` call.

The research `261018-research-claude-mod-ws-manual-injection` verified on
macOS with Claude Code 2.1.302 (probe runs cited there):

- `prompt.compose` answers reach the API only at a conversation's first
  request and after manual `/compact`, a plugin `$.session.compact()`, or
  `/clear`; later answers are discarded, and a `--resume`d session replays its
  recorded snapshot.
- Named subagents run no compose and cannot see a compose section; forks
  inherit the parent's snapshot.
- An un-awaited first-compose fetch can miss the snapshot; awaiting it inside
  compose within the 10 s hook budget fixes it.
- `tool.call` can replace a ws MCP result with `{ result: string | text[] }`
  without spreading the core result (spreading keeps `ref`, and the original
  text wins); `agentId` marks subagent calls.
- `$.session.root()` is the launch directory; `$.session.repo().root` and
  `$.fs.ancestors` find the repository root from a subdirectory.

Session-key omission (auto-filling `session_key`) was dropped from scope by
the user.

## Decisions

- **Mint a lead key at session start, as pi does.** Like pi's one root key per
  session (`agents-plugin-pi/src/bridge.ts`), the mod mints a lead session key
  for the conversation (`workflow_manual` with the fresh-bootstrap sentinel
  `obsidian-latch` (`freshBootstrapKey` in ws-mcp's `workflow_manual.go`)
  and `root` set to the repository root, or `ferrule` followed by keyed reads;
  the worker picks the path from the observed responses) and fetches the
  keyed manual: the `workflow_manual(key)` response, whose part before the
  `## Session Key` line is the injected manual (pi's `staticManualPart`), and
  `playbook_read` `lead-workflow-manual` with the key, the static body whose
  first line is the cut's start anchor (pi's `staticBodySnapshotRef`). The
  key never fills a `session_key` argument: the lead reads it from the system
  prompt and passes it itself, so the dropped session-key omission stays out.
  Rejected: injecting the key-less `playbook_read` render (seed defaults: its
  `### User preferences` and other override points differ from the keyed
  render, checked live 2026-10-18, so a cut would delete key-specific text the
  prompt does not carry); remembering a key from the lead's first
  `workflow_manual` call per session or per repository root (the first window
  or a new worktree conversation would still miss). (user, 2026-10-18)
- **One key per conversation.** The key is stored in `$.store` under the
  session id and reused across compaction, hot reload, and `--resume`
  (re-login as pi's `reloginLeadKey` does); `/clear` starts a new
  conversation (a new session id) and mints a new key. The `$.store` entry
  therefore outlives `session.end` (unlike the mailbox binding's), so a later
  `--resume` of that session can reuse it; the implementer prunes stale
  entries (for example by age at `session.start`) and records the rule. Every Claude session in a ws project mints
  one even if the workflow goes unused (pi parity; the first snapshot rules out
  lazy minting); the worker records in the Result whether ws-mcp
  garbage-collects session records. (user, 2026-10-18)
- **Inject through `prompt.compose`, awaited.** Minting and both fetches start
  at `session.start`; the first compose after a reset awaits them for at most
  about 5 s (below the 10 s hook budget). When nothing is cached (no
  `session.start` follows `/clear`, or a hot reload dropped module state), that
  first compose starts them itself under the same bound. Later composes never
  wait (their answers are discarded); they append the section only when it is
  already cached, so an unhealthy ws-mcp costs one bounded wait per reset, not
  one per turn. The server name comes from the reconnect lever's cached
  resolution (261018-feat-claude-mod-mcp-reconnect-lever), not a second
  resolve path. On failure or timeout compose appends nothing (today's
  behavior). The cache and the snapshot flag are cleared on `session.end`, so a
  `/clear` picks up an updated manual. Rejected: an un-awaited fetch (misses
  the snapshot for the whole conversation); keeping the cache across `/clear`.
  (user, 2026-10-18)
- **Deterministic section text.** A `session`-scope section with id
  `ws:workflow-manual`, pinned below: pi's session-state pointer line, the key
  line, then the injected manual. The keyed render already strips the
  fresh-only bootstrap block. An updated manual reaches a conversation only
  after compaction or `/clear` (accepted; see Constraints). (user, 2026-10-18)
- **State-only `workflow_manual` only when the snapshot carries the manual.**
  A flag records whether the conversation's snapshot carries the section: set
  from the first compose after a reset that refreshes the snapshot: a start
  that minted a fresh key (a new session, or after `/clear`), or a landed
  main-conversation compaction, including the mod's own lever compaction,
  which the engine routes past the mod's own `session.compact` hook. A start
  that reuses a key from `$.store` means a replayed snapshot (`--resume`) or a
  hot reload, so the flag stays false there (session start input carries no
  resume marker; the reused key is the signal). If the dogfood finds that
  native auto compaction does not refresh the snapshot, a `trigger: auto`
  landing leaves the flag unchanged. Only when it is set is a
  main-thread `workflow_manual` result rewritten to the session state;
  otherwise it passes through in full, so the manual is never in neither
  place. Rejected: tracking every compose answer (only the first reaches the
  API; a false flag only costs one full result). (user, 2026-10-18)
- **The rewrite follows pi's contract, behind an equality guard.** Cut from
  the static body's first line up to (not including) the `## Session Key`
  line, prepend pi's fixed line (pinned below), and when the anchors are not
  found fall back to calling `workflow_state` with the same key; when the
  response has no manual body at all (the no-restorable-state notice) pass it
  through unchanged. The rewrite applies only when the span the cut removes
  equals the same span of the injected manual; otherwise (config changed
  mid-conversation, or the lead continues another key, for example through
  `ws:lead-revive`) the result passes through in full. Rejected: mapping every
  call to `workflow_state` up front (loses what ws-mcp prepends and the
  fail-loud shape); a host-specific cut contract. (user, 2026-10-18)
- **Dedupe judged by the conversation, with a memory hint.** On main-thread
  calls only, a `playbook_read` (keyed by `name` plus `context`, the outer
  `session_key` excluded) or `workflow_manual` (keyed by `session_key`) result
  identical to an earlier same-key result still present in the conversation
  collapses to a pointer, compared on the text the model would otherwise see
  (after the state-only rewrite). A main-thread `playbook_read` of
  `lead-workflow-manual` follows the same rule; it is not special-cased.
  Module memory keeps (family, key, text hash)
  as a hint only: `$.session.messages({ as: "api" })` is read only when the
  fresh result matches a hint, to confirm the earlier result is still in the
  conversation. After a pointer, the next identical read returns the full text
  again (pi's count rule). So compaction, `/clear`, a rewind, or a hot reload
  needs no dedupe reset wiring: an earlier result gone from the conversation is
  not a repeat, and a lost hint only costs one missed dedupe. This matches
  pi's stateless, active-context contract
  (261002-feat-agenda-size-nudge-and-pi-manual-state-dedupe). Rejected: a
  module-memory map as the source of truth, reset on each compaction path
  (misses a rewind, and needs the lever-path reset); reading the conversation
  on every read (no hint; one transcript read per call). If Phase 1 finds
  `$.session.messages` unusable for this, the worker stops and reports rather
  than falling back to the rejected map. (user, 2026-10-18)
- **No subagent filter in compose; main thread only in `tool.call`.** Compose
  needs no filter (named subagents run none, forks should match the parent);
  the rewrite and dedupe run only when `agentId` is absent. (user, 2026-10-18)
- **ws-only, gated on a ws project.** The feature runs only in the `ws`
  package (the compaction feature's `compactionEnabledFor` gate) and only when
  `ai-docs/WORKFLOW.md` exists at the repository root from
  `$.session.repo()` (falling back to `$.fs.ancestors`), evaluated at
  `session.start` and again in compose while unset; otherwise it mints
  nothing, appends no section, and rewrites nothing. Rejected:
  `$.session.root()` (the launch directory, false from a subdirectory);
  evaluating per compose (the root does not change within a session).
  (user, 2026-10-18)
- **Prose pinned.** The section framing, the key line, the state-only line,
  and both pointers are pinned below and asserted verbatim (the compaction
  feature's precedent); pi's pointer prose is ported without its provenance
  comment and tool-call distance, which only a transcript-scanning pointer
  reader uses. (user, 2026-10-18)

## Constraints

- Known limitations, accepted (user, 2026-10-18): a plugin update reaches the
  injected manual only after compaction or `/clear`; whether native auto
  compaction refreshes the snapshot is checked in the dogfood, not assumed;
  teammate (`teammate` trait) sessions, agent teams, and Windows are not
  covered.
- Session-key omission (filling `session_key` arguments for the lead) is out
  of scope; minting a key the lead passes itself is in. (user, 2026-10-18)
- Auto-binding mailbox push with the minted key is out of scope (follow-up
  idea ticket). (user, 2026-10-18)
- The 261017 compaction and mailbox Decisions stay in force for the existing
  features of the mod.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/hooks/, agents-plugin-wsflow/)

## Pinned Prose

Asserted verbatim by tests. `{manual}` is the injected manual (the keyed
`workflow_manual` response before `## Session Key`), `{key}` the minted key,
`{tool}` the called tool (`playbook_read` or `workflow_manual`), `{headings}`
the result's `#`-`###` headings joined by `; ` (or `(no headings)`).

Section text:

```text
Your session state (agenda, todos, notes) is not in this prompt. When no `workflow_manual` result is visible in your context — a fresh session, or after compaction — call `workflow_manual` before acting.

Your lead session key for this conversation is `{key}`; pass it as `session_key`. Continuing another track (for example `ws:lead-revive` with a preserved key) is still allowed.

{manual}
```

State-only line (prepended to a rewritten `workflow_manual` result, then a
blank line):

```text
Workflow manual is in your system prompt; this is your current session state.
```

Pointer for `playbook_read`:

```text
The result is unchanged. Continue using the full result of your earlier `{tool}` call in this conversation. Do not read it again (headings: {headings}).
```

Pointer for `workflow_manual`:

```text
The result is unchanged. Continue using the full result of your earlier `{tool}` call in this conversation as your current session state; call workflow_manual again after you change agenda, todos, or notes. (headings: {headings})
```

## Prior Decisions

- 261017-feat-claude-mod-lead-compaction-timing (2026-10-18, Result): "The engine routes a plugin's `$.session.compact` past that plugin's own `session.compact` hook (an engine test confirmed it), so the lever step applies the landing reset itself; the hook ignores a landing while the lever's compaction runs" — bearing: constrains
- 42ab0938 (2026-10-18, commit): "Claude Code accepts one hooks module per plugin and refuses an event registered twice without a matcher, so session.start and session.end are registered once and start each feature in isolation" — bearing: constrains
- 1ed6398b (2026-10-18, commit): "Range review finding C2: if the session.end time bound cut the awaited delete short, compaction.end never ran and a pending request or latch carried into the next conversation after /clear or resume." — bearing: constrains
- 261002-feat-agenda-size-nudge-and-pi-manual-state-dedupe (2026-10-02, Decisions): "Reuse the stateless contract of `260908-feat-ws-pi-playbook-read-dedupe-in-window`: active-context only, fresh backend read and byte equality as the source of truth, no adapter cache" — bearing: supports (the dedupe Decision judges by the conversation; memory is only a hint)
- 260905-pi-workflow-manual-state-mapping (2026-09-09, 5ebebfaa): "The substring cut was always going to miss on any real ws-mcp render: every warning/manuals/notes injection prepends or appends around the manual body" — bearing: supports
- 260904-feat-ws-pi-lead-bootstrap-system-prompt (2026-09-04, Decisions): "On the `workflow_state` fallback, `root` and other `workflow_manual`-only arguments are dropped; `workflow_state`'s own error path for an unresolvable key is surfaced unchanged (fail-loud, never minting)." — bearing: supports
- 261002-feat-agenda-size-nudge-and-pi-manual-state-dedupe (2026-10-02, 1362fbf3): "staticManualPart uses the same whole-line `## Session Key` end anchor as the bridge's cutStaticBody, so the mapping (unchanged) and the prompt agree on where the static part ends" — bearing: supports
- 260908-feat-ws-pi-playbook-read-dedupe-in-window (2026-09-08, Decisions): "Scope. `ws__playbook_read` and `ws-skill` only. `playbook.render` and the workflow-manual snapshot (`lead-bootstrap.ts`, already served from the system prompt) are untouched." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/claude-mod/register.ts, a new feature module and tests beside compaction.ts, the byte-identical agents-plugin-wsflow/hooks/claude-mod/ copy |
| scope.surface | cross-module | new prompt.compose and tool.call hooks, key minting at session.start, and a snapshot-flag reset from the compaction feature's lever path (compaction.ts#L43-L66) and session.end; changes model-visible system prompt text and tool results in ws |
| scope.new_public_symbol | no | no new tool registered; the phase names no exported symbol |
| scope.new_type_contract | yes | the compaction lever path needs a landing notification for the snapshot flag (Compaction and CompactionEdges types in agents-plugin/hooks/claude-mod/compaction.ts#L43-L66), plus the pinned prose |
| scope.test_surface | new-files | engine tests for the new feature beside agents-plugin/hooks/claude-mod/compaction-engine.test.ts; existing engine.test.ts and compaction tests must keep passing |
| complexity.reuse_points | confirmed | compactionEnabledFor, mcpToolName and the session.start/session.end isolation in register.ts; pi cut and dedupe logic in agents-plugin-pi/src/bridge.ts#L361-L400 and agents-plugin-pi/src/playbook-read-dedupe.ts (read) |
| complexity.side_effect_risk | high | rewrites every ws lead conversation's system prompt and workflow_manual results; a wrong cut or stale flag leaves the manual in neither place |
| risk.correctness | high | snapshot-once compose semantics, the first-compose race, key minting and re-login, the cut guard, and transcript-confirmed dedupe are only partly testable in the engine kit |
| risk.fit | moderate | mirrors pi behavior (per-session root key, cut contract, stateless dedupe) on a different host API |
| risk.test | high | the compose snapshot, auto-compaction refresh, and opus treatment of the section are live-dogfood only (Phase 1 verification) |
| risk.security_or_contract | moderate | mints ws session keys and injects fetched ws-mcp text into the system prompt and alters tool results; no credential or permission surface, but a model-facing contract |

## Phases

### Phase 1: Manual injection, state-only workflow_manual, and re-read dedupe

Add the feature to `claude-mod/` as a feature beside the mod's existing
ones: setup (gate, minting, fetches) in the `session.start` hook, isolated
like the others (extend the existing `session.start` / `session.end` /
`session.compact` registrations rather than adding second unmatched ones,
which the host refuses; give new `tool.call` hooks matchers; bind engine edges
as closures, since the validator forbids storing `$`); a `prompt.compose`
hook; `tool.call` handling for the ws server's `playbook_read` and
`workflow_manual`; the snapshot-flag reset on `session.compact`,
`session.end`, and the compaction lever's own landing. Reword, in the same
landing, the texts that list the mod's features: the ws `hooks.json`
description, the `register.ts` header comment, and the Claude Mod paragraph of
`ai-docs/manuals/wsflow-mirroring.md` (as 261017 did when it added
compaction). Resync the wsflow copy byte-identically (the feature stays inert
there). The phase is one behavior slice; the worker may land it as several
commits (for example injection and rewrite, then dedupe), verified together.

Verification:

- `claude plugin validate` and `claude plugin test` pass for both packages,
  with engine tests for: the section appended only in `ws` and only with
  `ai-docs/WORKFLOW.md` at the repository root (including from a
  subdirectory); minting at start, the key reused from `$.store` after a hot
  reload and on resume, a new key after `/clear`, and stale-entry pruning; compose awaiting the
  pending work and appending nothing on timeout or failure; the lazy start
  after `session.end`; the snapshot flag from the first compose after each
  refreshing reset (the lever's own landing included) and false when the key
  was reused; later composes not waiting;
  the state-only rewrite with the cut, the guard passing a differing span
  through, the fallback to `workflow_state`, and the no-body pass-through;
  subagent calls untouched; dedupe pointers verbatim, the conversation read
  only on a hint match, no pointer when the earlier result is gone from the
  conversation, and the full text again after a pointer; the pinned prose
  verbatim; and the existing mailbox, compaction, and reconnect tests
  passing. Under wsflow: no key, no section, no rewrite.
- `python3 -m unittest discover` passes for both packages' tests, and
  `go test -race ./internal/wsrsrc/ -run TestWsflowHooksMirrorUpToDate`
  passes under `agents-plugin-tool/`.
- Live dogfood on opus in this repository: the first request's system prompt
  carries the section with the minted key (proxy or `/context`), the lead uses
  that key and follows the manual as system prompt, `workflow_manual` returns
  state only behind the pinned line, a repeated `playbook_read` collapses to
  the pointer, after a compaction the same read returns in full, and whether a
  native auto compaction refreshes the snapshot is recorded. Record in the
  Result the time one `$.session.messages({ as: "api" })` read takes in a
  large conversation (about 200K tokens or more), and whether ws-mcp
  garbage-collects minted session records.

### Result (8d7382f) - 2026-10-18

Implemented in `agents-plugin/hooks/claude-mod/manual.ts` (state machine
behind injected edges), wired in `register.ts` (`prompt.compose`, the
`workflow_manual` / `playbook_read` `tool.call` hooks, `session.start`,
`session.compact`, `session.end`); `compaction.ts` gained an optional
`leverLanded` edge because the lever's own landing bypasses the mod's
`session.compact` hook. Mirrored byte-identically to wsflow, inert there.
Tests: `manual.test.ts` (pure helpers, pinned prose, state machine) and
`manual-engine.test.ts` (engine wiring per package), plus `compaction.test.ts`
assertions for the new edge; 197 tests pass in each package.

Choices within the decisions:
- Minting uses `ferrule` (`format: "json"`); re-login passes the stored key as
  `relogin_session_key`. The key binds to `$.session.root()` (ws-mcp
  canonicalizes to the worktree toplevel), not `$.session.repo().root`, which
  names the main working tree for a linked worktree; the gate reads `repo()`.
- A refused re-login mints a new key but still counts as reused (the snapshot
  is the replayed one), so the flag stays false.
- `/context` composes (`analysis` trait) never wait and do not take the first
  compose; a compose before any `session.start` skips only that
  conversation's first-compose decision.
- Stored entries carry `at`; other sessions' entries older than 30 days (the
  ws-mcp key retention) or without `at` are pruned at `session.start`.

Live dogfood (opus, this repository, scratch plugin copy via `--plugin-dir`,
logging proxy on `ANTHROPIC_BASE_URL`):
- The first request's system prompt carried the section with the minted key;
  the first compose waited about 230 ms for the setup. The lead used that key.
- `workflow_manual` returned the state behind the pinned line; a repeated
  `playbook_read` collapsed to the pointer; after `/compact` the same read
  returned in full and `workflow_manual` was still state-only; a repeat
  `workflow_manual` whose earlier result survived compaction got its pointer.
- `/compact` refreshes the snapshot (the next request carried a new compose
  answer). Native auto compaction composes nothing: across six auto
  compactions the requests kept the snapshot's answer, and after one the
  requests carried the latest non-measuring compose answer already given. So
  auto landings leave the flag unchanged, as implemented.
- `--resume --fork-session` replays the recorded snapshot (no section in the
  first resumed request) while the fork's new session id reads as fresh, so
  the mod mints a key and sets the flag. Harmless: the span guard binds the cut
  to the minted key, which only the injected section reveals. Noted at the
  site; same for a resume of a session that never stored a key.
- One `$.session.messages({ as: "api" })` read in a resumed conversation of
  about 170K tokens (142 API messages, 763K JSON characters) took 5 ms; small
  conversations took 1 ms.
- ws-mcp does garbage-collect minted session records: the bootstrap
  (`ferrule`) path runs a prune at most once per 24 h that deletes key records
  idle 30 days (mtime; touched on use, guarded to once per hour).
- Found while dogfooding: a wrapped line in the lead-run playbook starts with
  `### ` and shows up in the pointer's heading list; captured as
  `261018-bug-lead-run-wrapped-line-reads-as-heading`.
