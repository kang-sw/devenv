---
title: "agent-v2 도그푸드 서프라이즈 및 agy compaction 배지/배경 유예 수정"
parent: 261114-epic-ws-common-session-foundation
related:
  261117-feat-ws-hosted-prompt-assembly: "hosted manual, playbook.render, and steer rendering surfaces"
  261117-feat-ws-hosted-compaction-lifecycle: "hosted /compact lifecycle and compacting state"
  261118-feat-ws-agy-tool-names-and-worker-role: "agy underscore tool aliases and worker role tool exposure"
  261115-feat-ws-hosted-codex-adapter-contract: "hosted Codex adapter and git_commit worktree behavior"
---

# agent-v2 dogfood surprises and agy compaction lifecycle fixes

## Background

Dogfooding the `track/agent-v2` hosted-session stack (agy lead/worker, Codex,
compaction, and ticket-index caching) surfaced fourteen workflow/runtime surprises
(items 1-11 from the original capture, items 12-14 added from the 2026-11-19
child-spawn dogfood) plus two concrete compaction regressions in agy (`CommandEndConfirms /compact`
badge clearing and `beginTurn` ending `bgResumeGrace` early).

This ticket captures all items in `todo/` under
`261114-epic-ws-common-session-foundation` for diagnosis and phased repair.

## Decisions

- **Compatibility tier: T0 (transparent)** for the whole ticket unless an
  item's diagnosis uncovers a persisted-shape, hello, or cross-version protocol
  change, in which case that phase must settle its concrete compatibility tier
  per `ai-docs/manuals/release-compatibility.md` before promotion to `ready/`.
- **Ticket stem category:** Requested as `261119-fix-ws-agent-v2-dogfood-surprises`
  and stored as `261119-bug-ws-agent-v2-dogfood-surprises` because ws ticket
  categories accept `feat, bug, refactor, chore, research, epic`.

## Constraints

- Manuals for the touched paths (`AGENTS.md` Implementation Conventions):
  - `ai-docs/manuals/shipped-surface-boundary.md` (`agents-plugin-tool/`, `agents-plugin/`, `agents-plugin-wsflow/`)
  - `ai-docs/manuals/ws-mcp.md` (`agents-plugin-tool/internal/mcp/`)
  - `ai-docs/manuals/release-compatibility.md` (`agents-plugin-tool/internal/`)
  - `ai-docs/manuals/performance.md` (`agents-plugin-tool/`)
  - `ai-docs/manuals/skill-authoring.md` (if any playbook/skill/convention text is touched)
- All source paths below are relative to `agents-plugin-tool/`. Line numbers in
  Phase 1 are **reported line hints** from the `track/agent-v2` dogfood capture
  (captured from an `ai-docs`-only sparse worktree without reading Go/JS source);
  verify exact lines before editing.
- Items marked **Diagnosis needed** must confirm root cause from source before
  promoting their phase to `ready/`.

## Phases

### Phase 1: agy compaction badge and background resume grace fixes

Compatibility tier: **T0** (in-memory turn/badge state and frontend debounce
timing only; no stored shape or protocol change).

1. **(a) agy compacting badge cleared on `TurnStart` (plus composer mention debounce):**
   - **Observed symptom:** In agy `/compact`, `CommandEndConfirms` applies
     `CompactStart` in `runCommand` (`internal/wsagent/runner_process.go`
     ~L24-28, reported line hint), and then `kick()` applies `KindTurnStart`
     (`internal/wsagent/runner_conv.go` ~L1175, reported line hint), which
     clears `compactStartedAt` (`internal/wslive/state.go` ~L316-319, reported
     line hint) and drops the compacting badge.
     - *Secondary:* Typing `/compact` + `Enter` inside the 60 ms mention
       debounce (`mention.js` ~L157, reported line hint) sends plain text
       instead of dispatching the `/compact` command.
   - **Expected behavior:** The lead remains in `Lead.Phase == compacting`
     across `kick()` / `TurnStart` until compaction completes or fails, and
     pressing `Enter` immediately after `/compact` within the 60 ms mention
     debounce submits the slash command rather than plain text.
   - **Fix & verification direction:** Apply `CompactStart` after `TurnStart` in
     `kick()` when `cmd.Name == "compact"` (or equivalent ordering invariant),
     add a regression test asserting `Lead.Phase == compacting`, and fix the
     60 ms mention debounce submit race in `mention.js` with a frontend test.

2. **(b) `beginTurn` calls `stopBgGrace()` unconditionally during `/compact`:**
   - **Observed symptom:** `beginTurn` (`internal/wsagent/runner_events.go`
     ~L264, reported line hint) calls `c.stopBgGrace()` unconditionally. A
     `/compact` arriving inside the 6 s `bgResumeGrace` window cancels the grace
     timer and publishes a stale held report early.
   - **Expected behavior:** A `/compact` turn inside the 6 s `bgResumeGrace`
     window does not cancel `bgResumeGrace` or flush a stale held report early.
   - **Fix & verification direction:** Guard the call with `if !c.compactCommand`
     around `c.stopBgGrace()` and add a regression test verifying that `/compact`
     inside the 6 s `bgResumeGrace` window does not publish a stale held report
     early.

### Phase 2: Hosted prompt, manual, playbook, and tool-surface parity

1. **Item 3 — Key minted by `playbook.render` refused by a later call *(Diagnosis needed)*:**
   - **Observed symptom:** A session key minted by `playbook.render` was
     subsequently refused when passed to a later ws tool call.
   - **Expected behavior:** Any key minted by `playbook.render` is accepted by
     subsequent ws tool calls within its minted session scope and authority.

2. **Item 4 — Stale `pi.*` tool text in the User preferences manual section *(Diagnosis needed)*:**
   - **Observed symptom:** The `User preferences` section of the workflow
     manual still includes stale `pi.*` tool references.
   - **Expected behavior:** Hosted manual rendering presents current
     `agent.*` / `mailbox.*` guidance without stale `pi.*` text (and diagnoses
     whether the source is user config override `UserPreferenceSection` or a
     shipped fallback).

3. **Item 7 — Legacy `"Reply with pi.send"` prefix still emitted in mail *(Diagnosis needed)*:**
   - **Observed symptom:** Delivered mailbox notifications/envelopes still emit
     a legacy `"Reply with pi.send"` instruction prefix.
   - **Expected behavior:** Emitted mail prefixes direct callers to
     `mailbox.send` (with the peer `id:<reply-id>` address) and never emit
     `"Reply with pi.send"`.

4. **Item 9 — agy `playbook.render` output drifts from the Claude render *(Diagnosis needed)*:**
   - **Observed symptom:** `playbook.render` output on an agy connection drifts
     from the equivalent Claude render beyond expected harness-declared tool
     naming/scheme differences.
   - **Expected behavior:** `playbook.render` produces consistent workflow
     instructions across hosted Claude and agy connections, differing only in
     catalog/descriptor-declared harness attributes.

5. **Item 11 — agy worker at `workdir-write` lacks `convention.read` and `tickets.template` in its tool list *(Diagnosis needed)*:**
   - **Observed symptom:** An agy worker spawned at `workdir-write` authority
     does not receive `convention.read` and `tickets.template` in its MCP
     `tools/list`.
   - **Expected behavior:** A `workdir-write` agy worker receives the expected
     worker toolset consistently across harnesses (or, if either tool is
     intentionally excluded from worker profiles, the worker tool contract and
     playbooks align so callers are not instructed to call absent tools).
   - **Update 2026-11-19:** the same gap was reported for `codex.*` children
     and Claude `workdir-write`/`read-only` children; see Item 12, which
     carries the fix status and the remaining gap.

6. **Item 12 — Child tool surface: `convention.read`, `tickets.template`, Codex approvals, and sage gate/stamp *(partly fixed on develop)*:**
   - **Observed symptom (2026-11-19):** `codex.*` and Claude
     `workdir-write`/`read-only` children lacked `convention.read` and
     `tickets.template`, and Codex children were refused `path.generate` and
     `mailbox.send` by the never-approve policy. Separately, a `claude.large`
     child spawned via `agent.spawn` with authority `workdir-write`,
     worktree-bound, had no `tickets.sage_gate` / `tickets.sage_stamp` in its
     tool list, so a ticket-promotion handler could not stamp and the lead
     stamped instead.
   - **Evidence:** the dogfood report of 2026-11-19 (child tool lists and the
     Codex refusal observed in the child-spawn dogfood); fix for
     the first part is commit `2240464a4` on `develop`.
   - **Status:** first part **fixed on develop** by `2240464a4` (child-safe
     allowlist: Codex per-tool `approval_mode=approve` for workers; Claude
     read-only children restricted to the same set), **pending daemon rebuild**
     before it is observable in a running daemon. **Remaining gap:** the sage
     gate/stamp tools for `workdir-write` children.
   - **Expected behavior:** either `tickets.sage_gate` / `tickets.sage_stamp`
     are exposed to `workdir-write` children carrying the worker key, or the
     ticket-promotion playbook states that the lead stamps and never instructs
     a child to call them.
   - **Verification direction:** after the daemon rebuild, a `workdir-write`
     child of each harness lists `convention.read` and `tickets.template`, and
     a Codex child's `path.generate` / `mailbox.send` succeed; the remaining
     gap is covered by a tool-list test for the chosen option.

7. **Item 13 — Rendered delegate playbook carries a different child session key than the spawn harness appended *(Diagnosis needed)*:**
   - **Observed symptom (2026-11-19):** a rendered delegate playbook embedded
     one child session key while the spawn harness appended another to the
     child's prompt; the worker had to guess and used the appended one.
   - **Evidence:** the dogfood report of 2026-11-19 (the worker saw two
     differing keys and used the harness-appended one); related to Item 3 (keys minted by
     `playbook.render`).
   - **Expected behavior:** exactly one child session key is present in a
     delegated child's prompt, and it is the key the child's tool calls are
     accepted under; diagnose whether render-minted and spawn-minted keys
     should be unified or the spawn append dropped.

### Phase 3: Harness tool-result size limits and output truncation

1. **Item 1 — `workflow_state` output truncated by the agy harness *(Diagnosis needed)*:**
   - **Observed symptom:** `workflow_state` tool output is truncated by the agy
     harness.
   - **Expected behavior:** `workflow_state` output stays bounded within agy
     harness tool-result limits (or pages/summarizes cleanly) so callers receive
     complete, usable state without mid-payload truncation.

2. **Item 5 — `tickets.query` output exceeds the host size limit *(Diagnosis needed)*:**
   - **Observed symptom:** `tickets.query` responses exceed the host's
     tool-result size limit.
   - **Expected behavior:** `tickets.query` bounds response size (e.g., tighter
     default page/field budgets or compact summary mode) so normal discovery
     calls remain within host size limits.

### Phase 4: Daemon swap (`--replace`) and agy turn/child lifecycle

1. **Item 8 — Mail sent after a daemon swap (`--replace`) does not wake an idle agy child *(Diagnosis needed)*:**
   - **Observed symptom:** After a daemon `--replace`, `mailbox.send` to an idle
     agy child does not wake the child.
   - **Expected behavior:** Mail sent after `--replace` wakes an addressable
     idle agy child reliably, or if non-holder-backed children cannot survive
     daemon replacement, their retired/stopped status is surfaced explicitly
     rather than silently dropping wakes.

2. **Item 10 — An agy turn silent for >30 s after a `--replace` ends early *(Diagnosis needed)*:**
   - **Observed symptom:** After a daemon `--replace`, an agy turn that produces
     no output for >30 s ends early.
   - **Expected behavior:** An active agy turn silent for >30 s (for example
     during model thinking or long tool/command work) remains open after
     `--replace` until the harness reports turn completion or failure.

3. **Item 14 — agy child turn ends with only a stream-idle API error as its final report *(fix in progress)*:**
   - **Observed symptom (2026-11-19):** an agy child's turn can end with only
     `⚠ API error (attempt 1): request failed: stream idle timeout (10m0s)`
     as the final report, with no work summary.
   - **Evidence:** the dogfood report of 2026-11-19 (final report text of an agy
     child as received by the lead).
   - **Status:** a watchdog fix is in progress on branch
     `impl/develop/agy-model-wait-watchdog`; this ticket tracks the symptom
     only and takes no implementation from that branch.
   - **Expected behavior:** a stream-idle timeout is retried or reported as a
     turn failure that the lead can distinguish from a completed report, never
     as the final report of a finished turn.
   - **Verification direction:** settle once the watchdog branch lands: a
     simulated stream-idle timeout yields a failed or retried turn state, not
     a final report.

### Phase 5: Codex `git_commit` branch mismatch and ticket-index cache staleness

1. **Item 2 — Ticket-index cache served a 16h-old view *(Diagnosis needed)*:**
   - **Observed symptom:** The local ticket ownership index cache served a
     ~16-hour-old view instead of refreshing from origin.
   - **Expected behavior:** Stale local ticket-index caches refresh within the
     configured freshness window when origin is reachable (and clearly surface
     staleness when offline) rather than serving a 16h-old view.

2. **Item 6 — Codex `git_commit` reports a branch mismatch *(Diagnosis needed)*:**
   - **Observed symptom:** Calling `git_commit` from a Codex session reports a
     branch mismatch error.
   - **Expected behavior:** Codex `git_commit` resolves the target worktree's
     current `HEAD` branch accurately and commits when `expected_branch` matches
     the checked-out branch.
