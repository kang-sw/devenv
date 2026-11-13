---
title: "A requested compaction runs before the user's queued follow-up"
---

# A requested compaction runs before the user's queued follow-up

## Background

Investigation by the MAIN lead, 2026-11-13 (owner answers (512)=b, (513)=ok on
MAIN's board).

When a lead calls the compaction lever (`*__request_compaction`) and the user
has queued a follow-up (not a steer), the follow-up is delivered at turn end
before the compaction starts, so the follow-up turn runs first and the
compaction is pushed back.

- claude-mod: `agents-plugin/hooks/claude-mod/compaction.ts` waits
  `SETTLE_MS = 1000` (`:78`) and runs only when no main-thread turn is
  running, otherwise reschedules at the next turn end. Queued board messages
  are released at turn end by `turnCompleted` -> `releaseBoard`
  (`watcher.ts:1225`, `:1270`, `releaseBoard` at `:703`).
- Hosted (daemon): `wsagent/runner.go` `kick()` (`:1734`) delivers queued
  input right at turn end. `claude.go` parses the stream-json, and the fixture
  `agents-plugin-tool/internal/wsagent/testdata/claude-lead-compaction.jsonl`
  already contains a `*__request_compaction` call.
- In hosted mode (`register.ts:114`, `WS_HOSTED_SESSION*`) the mod's input
  delivery is disabled, so the mod change applies only to non-hosted
  sessions.

## Decisions

1. **Scope: hosted sessions and claude-mod sessions' dashboard-delivered
   (board) messages** ((512)=b, owner 2026-11-13). Pi has no compaction lever
   and is out of scope.
2. **Queued follow-ups are held at most 5 s** ((513)=ok, owner 2026-11-13):
   after a successful lever request in the turn, queued follow-ups wait until
   the compaction starts or 5 s pass, whichever comes first.
3. **Hosted mechanism** (MAIN design, technical): the daemon detects a
   successful `*__request_compaction` tool_use / tool_result pair in the
   turn's stream-json, and `kick()` holds the queued follow-ups until the
   compaction starts or the 5 s cap. A compaction is its own turn, so `kick()`
   already waits through it. With `continue`, the lever's resume prompt is
   cancelled when another turn starts within `CONTINUATION_WINDOW_MS = 1500`
   (`compaction.ts:71`), so no duplicate turn results. Rejected: delivering
   the follow-up first (today's behaviour; the compaction the lead asked for
   is delayed by a whole turn).
4. **Mod mechanism** (MAIN design, technical): in `watcher.ts`, while a
   lever request is pending, `turnCompleted` does not release held board
   messages; the existing `compactStarted` / `compactEnded` path releases
   them after `continuation()`. The same 5 s cap applies.
5. **Compatibility tier: T0** (announced by MAIN): daemon-internal delivery
   timing and mod hook behavior; no tool, argument, store, protocol, or
   tracker event change.

## Constraints

- A steer (input that interrupts the running turn) is not held; only queued
  follow-ups are.
- Mirror the mod change into `agents-plugin-wsflow/` per
  `ai-docs/manuals/wsflow-mirroring.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Phases

### Phase 1: Hold queued follow-ups behind a requested compaction

- Hosted: lever detection in the stream parser and the hold in `kick()` per
  Decision 3.
- Mod: the pending-lever hold in `watcher.ts` per Decision 4, mirrored to
  wsflow.

Verification: a hosted runner test driven by `claude-lead-compaction.jsonl`
where a follow-up queued during the lever turn is held until the compaction
starts, and another where it is released at the 5 s cap when no compaction
starts; a turn without a lever call delivers queued input at turn end as
today; a `watcher.ts` test that a pending lever request holds the turn-end
release and `compactStarted` / `compactEnded` releases it, plus the 5 s cap;
the claude-mod and wsflow mirror test suites pass.
