---
title: Cross-session mailbox wake path & usage — CLI wait, harness hook adapters, lead-use-mailbox skill
related:
  260913-research-cross-session-mailbox: design-source — carries the settled wake decisions (6, 7, 9, wake half of 14) and the 2026-09-13 probe results
  260913-feat-cross-session-mailbox-core: prerequisite — the wake path drains and arms over the core's name-keyed queue, reply-id queue, and piggyback spine
  260909-epic-ws-worker-interpreter-refoundation: constraint — no blocking in a tool call, no polling; harness owns waiting (Cross-Child Decision 16)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 3fd30e7661a29b94
sage-review-completeness-reviewed: 3fd30e7661a29b94
---

# Cross-session mailbox wake path & usage — CLI wait, harness hook adapters, lead-use-mailbox skill

## Background

The core ticket (`260913-feat-cross-session-mailbox-core`) delivers peer
discovery, universal send, explicit `recv`, and the unread piggyback badge —
enough for a session that is already taking turns to *notice* mail. It does not
wake a **truly idle** executor: the motivating scenario is a Claude discussion
session remote-controlling an otherwise-idle Codex `lead-run` executor by mail
("run ticket X"), which needs something that blocks until mail arrives and then
re-invokes the agent.

This ticket adds that wake path and the usage layer on top of the core:
a host-neutral blocking CLI, the Codex/Claude hook adapters that arm and
re-invoke it, and a `lead-use-mailbox` skill that walks the lead/user through
the end-to-end flow. The harness-hook behavior this rests on was **already
empirically verified** — see `## Probe results` in the research ticket
(Codex 0.154.0, 2026-09-13) — so this ticket is no longer probe-gated; it
carries the probe findings as constraints.

## Decisions

Carried in intent from the research ticket's Confirmed Decisions 6, 7, 9 and the
wake half of 14, plus the 2026-09-13 probe results.

- **Wake lives in a CLI, never an MCP tool (6):** blocking belongs to a CLI
  process `ws-mcp mailbox wait --timeout`, launched by the model as a harness
  background task whose exit re-invokes the agent — the one non-implicit model
  obligation. A blocking MCP tool would freeze the single multiplexed stdio
  channel shared by the lead and every native subagent and re-cross refoundation
  Decision 16. The wait is **level-triggered, not edge-triggered**: on startup it
  synchronously checks the durable queues (the owner's name-keyed inbox and the
  caller's own reply-id queue) and returns immediately if any unread exists,
  blocking only when empty. Because the store is durable, mail deposited in the
  arm/drain gap is read on startup — no lost wakeup. A listening marker records
  that a wait is armed.
- **Env-less best-effort wake (wake half of 14):** a session with no
  `WS_MAILBOX`/`WS_MAILBOX_AUTO` has no durable inbox and no hook-driven wake;
  its wake is best-effort — the piggyback badge on its own ws calls plus a
  background CLI `wait` registration arranged at piggyback time, waiting on its
  own reply-id queue. Durable, hook-backed wake requires a slug (env).
- **Harness adapters are best-effort, never the contract (7):** the Codex/Claude
  hook wiring and pi native push are adapter/fallback surfaces per
  `shipped-surface-boundary.md`. The host-neutral contract is the CLI + listening
  marker of Phase 1; each host's arm/re-invoke mechanism is adapter behavior.
- **Codex adapter (probe-verified, 0.154.0):** the turn-boundary wake is a `Stop`
  hook returning `{"decision":"block","reason":"<drain/act instruction>"}`, which
  re-invokes the model with the reason as an instruction; the re-entry `Stop`
  carries `stop_hook_active: true` as a loop guard. `PostToolUse` output **never
  reaches the Codex model** (verified: not `exit 2`+stderr, not `decision:block`,
  not `additionalContext`), so `PostToolUse` is **side-effect/marker only**;
  mid-turn awareness rides the core's piggyback spine, not hooks. Codex `Stop`
  carries **no agent classifier**, so owner-context gating must **bake owner
  identity into the hook command args**. Codex 0.154 gates hooks behind persisted
  **hook trust** (`--dangerously-bypass-hook-trust` bypasses for automation), so
  a real adapter must persist hook trust — a deployment step.
- **Claude adapter (probe-verified from docs):** a single `Stop` hook gates on
  `hook_event_name == "Stop"` && `agent_id` absent → root lead turn (subagent
  context carries `agent_id`; `SubagentStop` always does), so the arm-reminder
  fires only in the owner/root turn. `run_in_background` → task-notification
  re-invoke is the wake (observed live; a completion can surface mid-turn).
- **Hook-layer misfire guard (9):** the arm-reminder fires only in the
  owner/root-turn context — on Claude via the `Stop`-not-`SubagentStop`
  distinction above; on Codex, since there is no payload classifier, via owner
  identity baked into the hook command args. This is best-effort and is the
  hook-layer counterpart of the core's server-layer `caller == owner` gate, which
  remains the real misfire guard.

## Constraints

- `agents-plugin-tool/` (the `ws-mcp mailbox wait` CLI subcommand and any MCP
  surface it reads) → read `ai-docs/manuals/ws-mcp.md` before editing.
- Hook-adapter text and the skill are **shipped surfaces** → read
  `ai-docs/manuals/shipped-surface-boundary.md`; the host-neutral contract is the
  CLI + marker, and Codex/Claude/pi specifics are adapter behavior that must not
  leak into the host-neutral layer.
- Phase 4 edits skills under `agents-plugin/skills/` → read
  `ai-docs/manuals/skill-authoring.md` and `ai-docs/manuals/wsflow-mirroring.md`
  (the skill must mirror into the agentless `agents-plugin-wsflow/` package).
- Codex behavior specifics come from `ai-docs/manuals/codex-integration.md`
  (re-probed 2026-09-13, 0.154.0); honor the hook-trust gating and the
  `PostToolUse`-output-never-reaches-model finding.
- Refoundation Cross-Child Decision 16: no blocking in a tool call, no polling —
  the wait blocks in the CLI process, and the level-triggered check is a durable
  read on startup, not a poll loop.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Art

- `260504-feat-ws-mcp-hook-driven-interrupt` (retired) — the hook-driven delivery
  shape (Codex `Stop`/`PostToolUse`, a `WS_AGENT_OUTBOX`-style env, hook-triggered
  drain) to mirror; its ws-owns-the-subprocess premise is dead, the delivery
  mechanism is not.
- `260913-feat-cross-session-mailbox-core` — the name-keyed queue, the reply-id
  queue, the listening/presence store, and the piggyback spine that this wait
  drains and this adapter arms.
- `ai-docs/manuals/codex-integration.md` — the 2026-09-13 hook re-probe: `Stop`
  `decision:block` keep-alive, `PostToolUse` output not reaching the model,
  hook-trust gating, useful isolation flags.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/ (mailbox wait CLI subcommand and the MCP surface it reads), agents-plugin/.codex-plugin/plugin.json + agents-plugin/.claude-plugin/plugin.json (Phase 2/3 hook wiring), agents-plugin/skills/ (Phase 4 lead-use-mailbox skill), agents-plugin-wsflow/ (Phase 4 mirror target) |
| scope.surface | public-interface | new ws-mcp mailbox wait CLI subcommand, Codex/Claude hook adapter wiring, and the lead-use-mailbox skill are all harness- or user-facing surfaces |
| scope.new_public_symbol | yes | the ws-mcp mailbox wait CLI subcommand (Phase 1) and the lead-use-mailbox skill (Phase 4) |
| scope.new_type_contract | yes | the wait CLI's exit/timeout contract and the listening marker format (Phase 1) |
| scope.test_surface | existing | agents-plugin-tool/cmd/ws-mcp/main_test.go covers existing CLI subcommands, agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go and wsflow_mirror_test.go plus agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py cover skill-shim/mirroring drift, matching Phase 4's own verification bullet |
| complexity.reuse_points | confirmed | the CLI subcommand dispatch pattern used by existing git/tickets/config subcommands, agents-plugin-tool/cmd/ws-mcp/main.go#L25-L53, is the pattern a new mailbox subcommand follows; Phase 4 reuses the skill-authoring/wsflow-mirroring pattern. Phases 2-3's hook wiring have no surviving code to reuse: grep for hooks in agents-plugin/.codex-plugin/plugin.json and agents-plugin/.claude-plugin/plugin.json returns nothing, and 260504-feat-ws-mcp-hook-driven-interrupt's hook code is fully retired (research ticket's grep for agents.interrupt/mercenary in current Go source returns nothing) — only its design shape is prior art |
| complexity.side_effect_risk | moderate | a Stop hook can block/re-invoke the harness turn loop and a background wait process interacts with harness lifecycle, though it is gated to opted-in WS_MAILBOX/WS_MAILBOX_AUTO sessions |
| risk.correctness | moderate | level-triggered lost-wakeup avoidance, the Stop re-entry loop guard (stop_hook_active), and owner-context misfire gating are load-bearing but build on the core ticket's already-Confirmed durable-queue design (260913-feat-cross-session-mailbox-core, pending) |
| risk.fit | low | Decisions 6, 7, 9, and the wake half of 14 match 260913-research-cross-session-mailbox's Confirmed Decisions section verbatim, and the CLI-dispatch and skill-mirroring reuse points exist in the tree as described |
| risk.test | moderate | Phase 1 is unit-testable host-neutrally, but Phases 2-3's verification bullets need live Codex 0.154.0 / Claude harness behavior (hook trust, Stop re-invoke, run_in_background notification) that automated tests cannot fully cover |
| risk.security_or_contract | moderate | the hook-layer misfire guard is explicitly best-effort (Decision 9); the real gate is the core ticket's server-layer caller-equals-owner check, which this ticket depends on (pending 260913-feat-cross-session-mailbox-core) but does not itself implement |

## Phases

### Phase 1: Host-neutral wake CLI + env-less registration

Implement `ws-mcp mailbox wait --timeout` as a blocking CLI (never an MCP tool):
on startup it synchronously checks the durable queues the caller is entitled to
drain (the owner's name-keyed inbox when a slug is set, and always the caller's
own reply-id queue) and returns immediately if any unread exists; otherwise it
blocks until arrival or the timeout, then exits. It writes and clears a listening
marker so an armed wait is discoverable. The host-neutral deliverable for the
env-less path (wake half of 14) is only that the `wait` CLI can target the
caller's **own reply-id queue without a slug**; actually *arming* it as a harness
background task at piggyback time is a host mechanism owned by Phases 2-3 (and, per
Decision 6, the model's non-implicit obligation triggered by the core's piggyback
badge). This phase is host-neutral — the CLI, its exit semantics, and the marker
are the contract; how each host launches it and re-invokes on exit is Phases 2-3.

Verification (host-neutral, no harness hook needed):

- `wait` returns immediately (exit 0, reports the unread) when the durable queue
  already holds unread mail at startup — level-triggered, no lost wakeup across
  the arm/drain gap.
- `wait` on an empty queue blocks, then returns promptly when mail is deposited.
- `wait --timeout` returns cleanly (distinguishable exit/stdout) on timeout with
  no mail.
- The listening marker is written while armed and cleared on exit.
- An env-less caller's `wait` targets its own reply-id queue and returns on a
  reply addressed to that reply-id.

### Result (588dda97) - 2026-09-13

Implemented `ws-mcp mailbox wait --timeout [--slug] [--session-key] [--format]`
as a blocking CLI subcommand (never an MCP tool), landed across two commits:
`e3c50663` (initial implementation) and `588dda97` (round-1 review fixes).

- **Host-neutral wait primitive** (`internal/wsmailbox/wait.go`): `Wait()`
  performs one up-front resolution (`resolveWaitTarget`: slug parse/scope-path
  resolution, reply-id derivation) and then loops over a cheap `peek()`
  (`Load`/`LoadReplyStore` only) at `DefaultWaitPoll` (500ms) increments,
  level-triggered (immediate return if either queue already has unread mail)
  with an injectable clock/sleep for deterministic tests. `NamedInboxStatus()`
  is a one-time diagnostic reporting whether an explicit `--slug` currently
  has a presence record and is owned by the caller.
- **Listening marker** (`internal/wsmailbox/listening.go`): a discoverable
  "armed wait" marker (session_key, slug, PID, started/timeout-at), written
  before blocking and cleared on every exit path (explicit clears, not
  `defer`, since `os.Exit` skips deferred functions). `session_key` is
  validated against `^[a-z0-9-]{1,128}$` (mirroring
  `internal/mcp/session_auth.go`'s pattern, duplicated rather than imported to
  avoid a Go import cycle) before it is ever interpolated into a filename.
- **CLI layer** (`cmd/ws-mcp/mailbox.go`, wired into `main.go`): validates
  `--session-key` (required) and `--timeout` (must be `>= 0`), emits a
  one-time stderr warning when an explicit `--slug` cannot currently be
  reached (no presence yet, or owned by a different session), handles
  SIGINT/SIGTERM via `signal.NotifyContext` (clearing the marker before exit),
  and reports outcomes with distinguishable exit codes: 0 (mail found), 3
  (`mailboxWaitExitTimeout`), 130 (interrupted), 1 (any other error). Text and
  `--format json` output both normalize empty queues to `[]`, never `null`.

Verification: `go build ./...`, `go vet ./...`, and `go test ./...` all pass
(`agents-plugin-tool`, all packages). CLI-level tests in
`cmd/ws-mcp/mailbox_test.go` cover the required-session-key/negative-timeout
error paths, immediate-return-on-unread-mail (both reply-id and owned
`--slug`), the unowned/absent-`--slug` warning, the JSON output shape, the
listening-marker write/clear lifecycle, a real-subprocess block-then-arrival
case (mail deposited while genuinely blocked, confirmed to return well before
its timeout), the timeout exit code, and the exit-code-1 error path. Package
tests in `internal/wsmailbox/{wait,listening}_test.go` cover the primitive
and marker directly, including path-traversal rejection and all four
present/owned combinations for `NamedInboxStatus`.

Two-round independent review (correctness: opus, test: sonnet, partitioned
per route verdict): round 1 found one Critical (session_key path traversal
into the listening marker path — fixed) and two Important correctness
findings (unbounded per-tick `PathForScope` re-resolution shelling out to git
on a worktree/clone `--slug` — fixed by resolving once up front; silent
degradation on an unreachable/unowned `--slug` — fixed via
`NamedInboxStatus` + a startup stderr warning), two Minor correctness
findings (negative `--timeout` silently accepted; JSON `null` vs `[]` for
empty queues — both fixed), and three Important test-coverage gaps (no
CLI-level `--slug` end-to-end test, no real-subprocess block-then-arrival
test, no exit-code-1 test — all three closed with new tests). Round 2
(fresh reviewers, fix-verification only) confirmed all Critical/Important
findings fixed with supporting code evidence and a clean
build/vet/test run; no new Critical/Important issues were raised.

**Decisions and deferred items:**

- `--slug` is an explicit, caller-supplied flag; `Wait` never re-derives a
  slug from `WS_MAILBOX`/`WS_MAILBOX_AUTO` itself, because a fresh CLI
  process re-reading `WS_MAILBOX_AUTO` would mint a different random stem
  than the already-registered server process.
- `"mailbox.wait"` was deliberately **not** added to
  `runtimeCapabilityCommandNames()` / `agents-plugin/runtime.json`'s
  "commands" map: that map is a pinned launcher contract
  (`TestRuntimeCapabilitiesCommandReportsLauncherContractSurface`), and a
  CLI-only wait primitive with no MCP-tool surface does not belong in it.
- **Deferred (Minor, accepted risk):** stale marker / PID-liveness / two
  waits sharing a session_key silently overwriting each other's marker is
  unfixed. Both reviewers agreed this is a landmine rather than a live
  defect in Phase 1, since nothing reads the marker yet (Phase 2/3
  territory) — revisit when a hook adapter starts consuming it.
- **Deferred (non-blocking observation from round 2):** the CLI's startup
  `NamedInboxStatus` diagnostic and `Wait`'s own `resolveWaitTarget` each
  independently resolve an explicit `--slug`'s scope path, doing the
  work (and, for a worktree/clone scope, the `git` shell-out) twice at
  startup instead of once. One-time only, so it does not reintroduce the
  round-1 per-tick-resolution finding; left as a minor efficiency
  opportunity rather than a fix-now item.

Phases 2-4 remain open; this ticket stays in `ready/`.

### Phase 2: Codex hook adapter

Wire the Codex adapter over Phase 1 (depends on the Phase 1 CLI + marker).
A `Stop` hook returns `{"decision":"block","reason":...}` to drain/act and
re-invoke at the turn boundary, guarding re-entry on `stop_hook_active`. The
causal trigger: the `Stop` hook fires at each turn-conclude (the hook event
itself), and on each firing does a **level read** of the durable queue /
listening marker, returning `decision:block`+`reason` **only when unread mail is
pending** — a per-turn-boundary level check, not a poll loop, so it honors
Decision 16's no-polling (contrast Claude's `run_in_background` completion →
task-notification chain in Phase 3). Bake the
owner identity into the hook command args (no payload classifier exists). Use
`PostToolUse` only as a side-effect marker (its output never reaches the model).
Persist hook trust as a documented deployment step (0.154 gates hooks behind
trust). Keep all Codex-specific wiring in the adapter layer, not the host-neutral
contract.

Verification (Codex CLI 0.154.0):

- A `Stop` hook `decision:block`+`reason` re-invokes the model, which drains and
  acts on queued mail; the re-entry `Stop` carrying `stop_hook_active` does not
  loop.
- The `Stop` hook returns `decision:block` only when the level read finds unread
  mail pending; a turn-conclude with an empty queue concludes normally (no
  per-turn poll loop).
- Owner-context: the arm-reminder/drain fires for the owner turn (identity from
  hook args) and the server-layer `caller == owner` gate still blocks a non-owner
  key regardless of hook firing.
- With hook trust persisted, the adapter's hooks run without an interactive trust
  prompt.

### Result (67478735) - 2026-09-13

Implemented the Codex Stop-hook adapter over Phase 1's storage primitives
(never Phase 1's blocking `wait` CLI — Codex's mechanism is per-turn
event-driven, so it only ever needs one non-blocking read per `Stop`
firing, not a background block-until-arrival process). This phase went
through one round of independent review (correctness + test partitions),
which found 2 Critical and 5 Important issues in the initial pass; all are
fixed below and confirmed by a second, fix-verification-only review round.

- **Bounded, ownership-agnostic notify check**
  (`internal/wsmailbox/hook_peek.go`): `PeekNamedInboxUnread(slug, root)`
  reports a named inbox's queue length regardless of current
  `Presence.Owner` — a bare hook subprocess has no ws session_key to assert
  ownership with (Decision 9), so this deliberately skips the owner-match
  check Phase 1's `Wait`/`peek` perform; the real `caller == owner` gate
  stays server-side in `mailbox.recv`. On top of that,
  `ShouldNotifyNamedInboxUnread(slug, root)` adds a persisted per-name
  watermark (`<store-dir>/mailbox-codex-stop-notified/<name>.json`, a
  sibling directory of the mailbox store file itself, atomic temp+rename
  writes) so `decision:block` fires at most once per queue-length *change*,
  clearing on drain so a later refill at any count notifies again.
  **Round-1 Critical finding:** without this bound, a named inbox whose
  owner is unset, rebound, or simply never drained would get
  `decision:block` on every single `Stop` firing forever — worse than a
  missed wake, and exactly what Decision 7 warns an adapter must not
  become. **Round-2 Important finding (fix-verification round):** the
  round-1 fix's first version keyed the watermark path by `(scope, name)`
  under a fixed machine-global cache root, not by the actual resolved store
  directory. For `worktree`/`clone` scope, `PathForScope` resolves a
  *different* physical store per caller-supplied root, so two different
  worktree roots sharing the same mailbox name (e.g. `lead@worktree`, the
  exact per-role naming pattern this scope exists to support) would collide
  on one watermark file despite tracking two independent queues — reopening
  the same unbounded-re-block failure mode, just across roots instead of
  across owners. Fixed by deriving the watermark path from
  `filepath.Dir` of the same resolved store path
  `ShouldNotifyNamedInboxUnread` already reads, rather than a separate
  `wsstate.CacheRoot()` call: the store's own directory is already
  root-scoped by construction, so the watermark inherits that scoping with
  no extra bookkeeping (and no longer needs scope in its filename).
  Regression-tested by
  `TestShouldNotifyNamedInboxUnreadIsolatesWatermarkPerRoot`, which seeds
  two separate git worktree fixtures with the same name and the same
  static unread count and asserts both notify independently.
- **CLI adapter subcommand** (`cmd/ws-mcp/mailbox.go`,
  `mailboxCodexStopHook`): `ws-mcp mailbox codex-stop-hook` reads the mailbox
  slug from its own inherited `WS_MAILBOX` environment variable at fire time
  (`--slug` remains as an override for direct invocation/testing only — the
  shipped hook never passes it), reads Codex's `Stop` JSON payload from
  stdin (`stop_hook_active` for the loop guard, `cwd` as a root fallback for
  a worktree/clone-scope slug), and prints `{"decision":"block","reason":...}`
  to stdout (exit 0) only when `ShouldNotifyNamedInboxUnread` says to; every
  other path (no `WS_MAILBOX`/`--slug`, malformed/empty stdin, an
  unresolvable slug, an empty or already-notified-at-this-count queue) is
  silent and exits 0 — fail-open, per Decision 7. Stdin is always drained
  before any exit path, including the no-op ones, so the harness's payload
  write is never left with an unread pipe on this end.
- **Plugin wiring** (`agents-plugin/.codex-plugin/hooks.json`, referenced
  from `agents-plugin/.codex-plugin/plugin.json` via
  `"hooks": "./.codex-plugin/hooks.json"`): a `Stop` hook whose POSIX
  `command` (plus a `commandWindows` counterpart) shells out to the existing
  `bin/ws-mcp-launcher.py` with `mailbox codex-stop-hook || true` — no
  `--slug` interpolated into the command string at all; the Go binary reads
  `WS_MAILBOX` from its own environment instead, closing what would
  otherwise be a command-substitution injection surface. `timeout: 30`
  (raised from an initial 10 — see deferred items below).
  **Round-1 Critical finding:** this file originally lived at
  `agents-plugin/hooks/hooks.json`. Claude Code auto-discovers
  `<plugin-root>/hooks/hooks.json` by directory-name convention alone, with
  no manifest key required (unlike Codex, which needs an explicit `"hooks"`
  path in its manifest) — so that path would have silently also activated
  this Codex-only hook in Claude, where `$PLUGIN_ROOT` is undefined (Claude
  uses `${CLAUDE_PLUGIN_ROOT}`), causing an exit-2 failure that *blocks* the
  Stop in Claude and feeds stderr to the model on every turn — a broken
  per-turn loop that `install.sh`'s `rsync -a --delete` would have shipped
  before Phase 3 (the Claude adapter) even exists. Fixed by relocating the
  file into `agents-plugin/.codex-plugin/` (a directory Claude never scans)
  and updating `plugin.json`'s `"hooks"` pointer to match.
- **Doc update** (`ai-docs/manuals/codex-integration.md`): added the
  plugin-bundled `hooks.json` schema (`{"hooks": {"<Event>": [{"type",
  "command", "timeout", ...}]}}`, one shell-string `command`, `PLUGIN_ROOT`/
  `PLUGIN_DATA` path env vars) sourced from the official docs site,
  explicitly flagged as lower-confidence than the manual's hands-on CLI
  re-probes above it; a note that no CLI/config hook-trust-persistence
  mechanism was found on 0.154.0; and, after the relocation above, a note
  on why the file moved and on the env-based (not `--slug`-interpolated)
  slug-discovery design.
- **New manifest-validation test**
  (`agents-plugin/tests/test_hooks_manifest.py`): asserts `plugin.json`'s
  `"hooks"` pointer resolves to an existing file, that file parses as JSON
  in the expected `{"hooks": {...}}` shape, every `command` is
  syntactically valid POSIX shell (`sh -n`), and every entry with a
  `command` also carries a non-blank `commandWindows`. Closes a round-1
  Minor finding: nothing previously asserted the shipped hooks.json even
  parses, so a typo would only have surfaced at live Codex install time.

Verification: `go build ./...`, `go vet ./...`, and `go test ./...` all
pass across every `agents-plugin-tool` package (re-run after every fix
round, including the round-2 fix). New/updated Go coverage:
`internal/wsmailbox/hook_peek_test.go` (the watermark's bound/re-notify-on-
increase/reset-after-drain behavior, the pre-existing ownership-agnostic and
malformed-slug cases, and — added after round 2 —
`TestShouldNotifyNamedInboxUnreadIsolatesWatermarkPerRoot`, seeding two
separate git worktree fixtures with the same name and unread count to prove
the watermark no longer collides across roots) and
`cmd/ws-mcp/mailbox_codex_hook_test.go` (rewritten for the env-based
interface: block-on-unread via `WS_MAILBOX`, `--slug` still overriding the
env for direct invocation, silent-on-empty-queue, the `stop_hook_active`
loop guard, a CLI-level repeated-firing-suppression regression test
mirroring the package-level watermark tests, no-op with neither env nor
flag set, fail-open on malformed stdin and on an unresolvable slug,
empty-stdin tolerance; and, added after round 2,
`TestMailboxCodexStopHookHonorsExplicitReason` plus a rewritten
`TestMailboxCodexStopHookUsesPayloadCwdForWorktreeSlug` that deliberately
diverges the OS process cwd from the payload's `cwd` field with a negative
control, rather than the original version's weaker "no warning surfaced"
assertion against the test binary's own already-valid ambient cwd, which
could not actually have caught a regression). `python3 -m unittest` passes
for `test_shipped_surfaces_downstream_neutral.py` (its `TEXT_TREES` now
lists the exact file `agents-plugin/.codex-plugin/hooks.json` rather than
the whole `.codex-plugin` directory — see deferred items below for why),
`test_skill_dispatch_contracts.py`, and the new `test_hooks_manifest.py`.
The CLI subcommand was also driven directly (piped Stop-shaped stdin JSON,
both via `WS_MAILBOX` and via `--slug`, against a real seeded machine-scope
inbox) confirming the `{"decision":"block","reason":...}` shape and the
watermark suppression by hand, matching the automated coverage.

**Review record:** round 1 (correctness + test partitions) found 2
Critical (Claude-leak relocation, unbounded re-block loop — both above),
5 Important (fail-open scoped only to inside the Go binary, not the shell
wrapper; the repair path in `ws-mcp-launcher.py` can exceed a 10s hook
timeout under local-devenv dogfood mode; the `Stop` payload's `cwd` was
parsed but discarded; doc comments inaccurately claimed the slug was baked
into hook args and that `WS_MAILBOX_AUTO` was consulted; no Windows command
variant), and several Minor (command-substitution injection via
shell-interpolated `--slug`; an exit-before-stdin-read EPIPE risk; no
manifest-parse test; no test for `--reason`) findings; all were fixed
before round 2. Round 2 (same two playbooks, fix-verification only)
confirmed 9 of the round-1 fixes clean, but the correctness partition
caught one genuine remaining gap in the Critical-2 fix itself (the
per-root watermark collision above, classified Important since the
mechanism was present and only its root-scoping was wrong) — fixed and
regression-tested as described above. The test partition returned "clean
with 1 minor remaining" (the `--reason` gap, since fixed) plus a
non-gating Observation that the cwd test was too weak to catch a
regression (also since fixed, by rewriting rather than only noting it).
No third independent review round was spawned for these fixes: both are
narrow, mechanically verifiable corrections (a path-derivation change and
two test rewrites) confirmed by the new regression tests and a clean full
`go build`/`go vet`/`go test` pass, and the ws Worker Protocol caps
independent review at two rounds.

**Not independently re-verified this round:** an actual live
`codex exec` round trip of this exact hook command (the sandbox's
"Create Unsafe Agents" guard denied spawning `codex exec
--dangerously-bypass-hook-trust --dangerously-bypass-approvals-and-sandbox`
even in an isolated `--ephemeral --ignore-user-config` probe). The
underlying `Stop`+`decision:block` re-invoke mechanism and the
`stop_hook_active` re-entry guard were already live-probed at the
research stage (`ai-docs/manuals/codex-integration.md`'s 2026-09-13
re-probe, cited in this ticket's Decisions) with a placeholder hook; this
round only re-verified the CLI's own JSON I/O contract and the on-disk
watermark against that already-confirmed mechanism, not a fresh live round
trip of the new command string. A plugin-cache-level (Level 3-style)
verification also needs the human-in-the-loop refresh
`ai-docs/manuals/ws-mcp.md` describes.

**Decisions and deferred items:**

- `WS_MAILBOX` (explicit slug) only, never `WS_MAILBOX_AUTO`: the
  auto-registration path mints its name randomly inside the MCP server
  process and never exports it anywhere a sibling hook subprocess can read
  (confirmed by reading `internal/mcp/mailbox_runtime.go`'s
  `computeMailboxIdentity`). A bare shell hook cannot discover that name, so
  Codex's durable, hook-backed wake is scoped to the explicit-slug case for
  now. Filed as a gap, not fixed here: the fix (e.g. the server writing a
  discoverable self-address record once per process) would touch the
  already-`.done` core ticket's surface and was not asked for by this
  phase.
- **Partially mitigated, not architecturally fixed:** the round-1 Important
  finding that `ws-mcp-launcher.py`'s binary-repair path can exceed a Stop
  hook's timeout under local-devenv dogfood mode. Mitigated by raising
  `hooks.json`'s `timeout` from 10 to 30; a full fix (e.g. bypassing the
  repair path entirely for hook invocations, or a fast-path binary lookup)
  was deliberately deferred as out of proportion for this phase — the
  process-env-visible `WS_MCP_RUNTIME_BINARY` shortcut was checked and ruled
  out (it is set inside the launcher's own exec'd child, not visible to a
  sibling hook subprocess). A future pass can revisit this if the 30s
  timeout still proves insufficient in practice.
- Filed `260913-research-codex-hook-trust-persistence` (idea): no
  CLI subcommand or documented `config.toml` key persists hook trust on
  Codex CLI 0.154.0 — only the always-required
  `--dangerously-bypass-hook-trust` bypass flag was found. This ticket's
  own Decision text assumed a persistable deployment step exists; that
  assumption is now flagged as unconfirmed rather than restated as fact.
- Filed `260913-bug-plugin-json-migration-vocab-uncovered` (idea): widening
  the neutrality scanner's `TEXT_TREES` to cover the relocated
  `hooks.json` surfaced that `agents-plugin/.codex-plugin/` (and thus
  `plugin.json`) had never been scanned at all, and `plugin.json` itself
  already carries pre-existing, unrelated "codex-first" migration-vocabulary
  text that would fail the scan. Scoped this ticket's `TEXT_TREES` entry to
  the exact file `agents-plugin/.codex-plugin/hooks.json` (the scanner's
  matching loop was extended to support an exact-file entry, not only a
  directory prefix) rather than fixing `plugin.json`'s wording under an
  unrelated ticket.
- Round-2 correctness review also logged 5 non-gating Observations (not
  findings — round 2 is fix-verification-only per protocol, so new items it
  notices are recorded rather than actioned): notably that `hooks.json`'s
  `|| true` fail-open is POSIX-only (no equivalent on the `commandWindows`
  side), and that `MaxQueueLen = 200` (`store.go`) means a named inbox that
  fills past that cap silently drops old mail without ever being reflected
  in the watermark's "queue-length change" signal in the one specific case
  where the drop count happens to exactly offset a simultaneous arrival
  count. Recorded here rather than fixed: neither was asked for by this
  phase, and both are pre-existing behavior this phase's own changes did
  not introduce.
- No `PostToolUse` hook was added: per this phase's own Decision text,
  Codex `PostToolUse` output never reaches the model, and mid-turn
  awareness already rides the core ticket's piggyback spine — there is no
  side effect this adapter currently needs a `PostToolUse` hook for.
- The `hooks.json` schema documented in `codex-integration.md` is sourced
  from the official docs site fetched today, not from a hands-on CLI probe
  like the rest of that file's Codex findings — flagged inline at a lower
  confidence tier pending a live plugin-cache dogfood pass.

### Phase 3: Claude hook adapter

Wire the Claude adapter over Phase 1 (depends on the Phase 1 CLI + marker). A
single `Stop` hook gates on `hook_event_name == "Stop"` && `agent_id` absent →
root lead turn, so the arm-reminder fires only in the owner/root context and
never on a subagent stop. Use `run_in_background` on the wait CLI so its
task-notification completion re-invokes the agent.

Verification (Claude):

- The `Stop` hook fires the arm-reminder only in the root turn (`agent_id`
  absent); a subagent stop / `SubagentStop` does not emit it.
- A `run_in_background` wait completing produces a task notification that
  re-invokes the agent to drain.
- Env-less arm-at-badge: an env-less session that surfaces the core's piggyback
  badge arms a `run_in_background` wait on its own reply-id queue, and a reply to
  that reply-id re-invokes it — the at-piggyback-time registration end to end
  (the model-driven arming obligation of Decision 6 / wake half of 14, documented
  in the Phase 4 skill).

### Phase 4: lead-use-mailbox skill

Author a host-neutral guidance skill `lead-use-mailbox` under
`agents-plugin/skills/` (mirrored into `agents-plugin-wsflow/` per
`wsflow-mirroring.md`) that walks the lead/user through the end-to-end flow over
the landed Phases 1-3: launching a session with `WS_MAILBOX` or
`WS_MAILBOX_AUTO`, discovering the self-address (the `lookup_peers` self entry /
workflow ambient block), arming the wait, the send/recv flow, and the
remote-control-executor recipe (a discussion session driving an idle Codex
executor by mailing "run ticket X"). Guidance only — no new MCP tool. Read
`skill-authoring.md` and apply its invariant checklist. New skill: its existence
and scope are user-confirmed (the (A) consolidation choice); depends on Phases
1-3 so the documented flow actually exists.

Verification:

- The skill passes the `skill-authoring.md` invariant checklist and the plugin's
  skill-shim / mirroring drift tests (`agents-plugin` + `agents-plugin-wsflow`).
- The documented flow (launch → self-address → arm → send/recv → remote-control
  recipe) references only shipped, host-neutral surfaces, with Codex/Claude
  specifics called out as adapter behavior.
