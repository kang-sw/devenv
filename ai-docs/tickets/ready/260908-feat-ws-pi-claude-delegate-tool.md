---
title: "`ws-claude`: delegate bounded judgment tasks to Claude Code subagents (fan-out, edit-scoped, resumable)"
sage-review-design: completed
sage-review-completeness: completed
related:
  260908-feat-ws-pi-claude-code-lead-provider: the reverted provider path; its finding is why this tool runs Claude Code as its own harness instead of as a Pi model
  260908-research-ws-pi-claude-code-lead-provider: the spike that established the SDK seam, strictMcpConfig, and cross-process prompt-cache behavior this tool reuses
  260907-feat-ws-pi-lead-tool-profile-and-orchestrator-role: the lead surface that would call this tool; delegation of audit/rewrite/consult is a lead affordance
spec:
  - pi-adapter-runtime
sage-review-design-reviewed: bfdff06e04e9fa40
sage-review-completeness-reviewed: bfdff06e04e9fa40
---

# `ws-claude`: delegate bounded judgment tasks to Claude Code subagents (fan-out, edit-scoped, resumable)

## Background

Claude's judgment on workflow artifacts (ticket audit, ticket rewrite, design
review) is materially better than the Pi-native tiers, and the owner wants that
quality available mid-workflow without leaving the Pi lead.

The provider path (`260908-feat-ws-pi-claude-code-lead-provider`) tried to make
Claude a Pi *model* and was reverted: routing Pi's built-in harness system
prompt through `claude -p` made the API classify every request as third-party
harness usage and refuse it with `400 out of extra usage` on a subscription with
no extra-usage balance. The finding there is the pivot for this ticket.

This tool flips prompt ownership. Instead of Claude serving Pi's harness, a
`ws-claude` call runs Claude Code **as its own harness** on a bounded subtask:
its own system prompt (a small task frame appended to the `claude_code` preset),
its own tools, a task string, a result returned as the tool output. That is
ordinary `claude -p` / Agent SDK usage, intended to use the subscription's
normal usage path. That expectation is not yet verified for this exact task
frame: the Phase 1 live probe must establish viability before implementation.
No Pi harness prompt crosses the wire; this avoids the known failed provider
shape without claiming that a different prompt guarantees billing treatment.

Scope note: this deliberately does **not** replace Pi's native subagent spawn.
Making spawn polymorphic (some subagents Pi, some Claude) would push a branch
through the most complex part of the adapter, lose Pi tier control and
sub-spawning, and a Claude subagent selected via a tier would carry Pi's
subagent prompt and hit the same 400. `ws-claude` is a distinct leaf tool for a
narrow, high-value class of tasks.

## Decisions

- **Tool shape: array in, index-aligned array out (fan-out + synchronous join).**
  ```
  ws-claude([
    { preset, request, edit-targets?, paths?, model?, resume? },
    ...
  ]) -> [ { id, status, output, changed?, usage, error? }, ... ]
  ```
  `Promise.allSettled` so one item failing does not sink the batch; each item
  reports its own error. A single task is still an array of one. The join is
  synchronous: the call blocks until every item settles.
- **Concurrency cap.** Fan-out runs through a pool with a small default cap
  (3-4). A subscription has rate/concurrency limits; an uncapped fan-out of
  many `claude` processes invites 429s. Overflow serializes.
- **Bounded settlement and cancellation.** Every running item has a finite
  timeout and honors the enclosing Pi tool's cancellation signal. Timeout or
  SDK/process failure yields that item's ordinary error result without losing
  completed siblings; cancel queued work and terminate active children on tool
  cancellation. Clean up listeners/processes in all outcomes. The implementation
  must expose/document its finite timeout policy and test it with a never-settling
  fake SDK child; numeric tuning is implementation configuration, not a promise
  of unbounded subscription work.
- **Permission is the `edit-targets` whitelist, not a flag.** The presence of a
  non-empty `edit-targets` array is what grants write access, scoped to exactly
  those paths; absent or empty means read-only. No separate `allow-edit` key. A
  `canUseTool` hook denies any Edit/Write outside the declared list, so a
  fanned-out editing agent cannot stray into another item's files. Resolve
  each target against the track worktree, canonicalize existing ancestors and
  symlinks, and require the resulting file to stay inside that worktree and
  match an exact authorized target. For a new file, canonicalize its existing
  parent before appending the leaf name. Reject out-of-root paths, traversal
  escapes and symlink aliases that escape the whitelist; revalidate at each
  write so a changed symlink does not retain old authorization. Reject
  overlapping canonical edit targets across concurrently scheduled items.
- **Tool profile.** Open: `Read`, `Grep`, `Glob`, `WebSearch`, `WebFetch`, and
  `Edit`/`Write` restricted to `edit-targets`. Closed: `Bash`, `git`, any exec,
  and account connector MCP servers (`strictMcpConfig: true`, which the spike
  showed is required or the claude.ai Drive/Gmail/Calendar servers attach).
- **No commit boundary to police, because git is closed.** Editing agents leave
  changes unstaged; the lead reviews and commits through the normal ws
  `git.commit` path, preserving the `## AI Context` and ticket-verify
  invariants and the develop -> main review gates. `changed` lists the paths an
  item touched.
- **Presets select the role prompt; four of them.**
  | preset | permission | output | prompt source |
  |---|---|---|---|
  | `audit` | read-only | findings on an artifact | embedded |
  | `consult` | read-only | reasoned answer to a posed question | embedded |
  | `rewrite` | requires `edit-targets` | improvements applied + change summary | embedded |
  | `design-review` | read-only | ticket design findings | `ticket-reviewer-design` |
  A generic passthrough preset is deliberately omitted: Claude is not better at
  arbitrary work, only at these judgment tasks. `consult` is kept separate from
  `audit` (rather than folded in) because they share the read-only permission
  family but have different output contracts — audit critiques an artifact,
  consult answers a specific assumption question the lead surfaced from its user
  conversation — and prompt specialization is where this tool's value lives.
  `preset` and `edit-targets` are orthogonal, but nonsensical combinations are
  guarded: `rewrite` without `edit-targets` is rejected, and a read-only preset
  with `edit-targets` is rejected.
- **Embedded prompts, except design-review.** Because the tool is embedded in
  the Pi extension there is no need to route the simple role prompts through the
  playbook system; `audit`/`consult`/`rewrite` carry embedded task frames.
  `design-review` is specifically ticket design review using the canonical
  `ticket-reviewer-design` playbook; general code/architecture review is outside
  this preset. The request supplies a target ticket path and Relations context
  (explicitly `none` when absent). Resolve the canonical text through the adapter's
  parent-side `playbook.read` access, then apply the context adaptation below.
  Do not pass a key-bearing `playbook.render` artifact to the Claude child:
  lead-key rendering mints and embeds a child ws credential. Keep all ws session
  credentials out of the child prompt and context. Keep the Claude tool profile closed: supply relevant
  ticket/spec/mental-model lookup results as a read-only context bundle and adapt
  unavailable ws lookup instructions to those supplied artifacts. Do not expose
  lead session keys or attach ws/account MCP servers merely to satisfy the prompt.
  Missing essential review context returns a clear per-item error or an explicit
  incomplete review, never a claimed complete verdict. Preserve the canonical
  design criteria and verdict format; this is a host adapter, not a new shared
  review playbook. Embedded prompts are prompt
  authoring and follow `ai-docs/manuals/skill-authoring.md`.
- **Resume by extension-assigned 3-word stem.** Each spawned agent gets a
  collision-free 3-word stem as its public handle (e.g. `durable-miss-posture`),
  returned as `id`. The extension keeps a `stem -> claude session id` map and a
  later call with `{ resume: <stem>, request }` continues that agent via Claude
  Code's native `--resume`. The public handle is the stem so callers never touch
  raw session ids and names cannot collide. Because resume starts a fresh
  process against Claude's own session store, no process is held open between
  calls (no MCP-park timeout), and the spike's cross-process prompt-cache hit
  makes the replay cheap.

## Constraints

- **Never carry Pi's harness prompt.** The system prompt is always the
  `claude_code` preset plus a small natural task frame. This preserves the
  intended Claude-owned harness boundary. The exact subscription behavior
  remains subject to the pre-build live probe; do not claim guaranteed billing
  classification based only on prompt shape.
- **Subscription usage still accrues,** and if the paused Agent-SDK credit split
  resumes it moves to that credit. This is less gray than the provider (Claude
  used as Claude Code), but it is not free; the concurrency cap also bounds
  burst usage.
- **Prompt drift risk on `rewrite`.** An embedded rewrite prompt can drift from
  the actual ticket/spec conventions. Mitigation: the rewrite frame has Claude
  read the bundled conventions first (or the convention text is injected as
  context) rather than trusting the embedded prompt to encode them.
- **Run inside the track worktree** so any edit damage is bounded to a branch.

## Spec Impact

Target spec area: `pi-adapter-runtime`. Adds a new adapter-registered Pi tool
`ws-claude` to the runtime surface: its parameter array shape, the four presets
and their permission/output contracts, the `edit-targets` whitelist permission
model and `canUseTool` enforcement, the closed-tool list, the fan-out/join and
concurrency-cap behavior, and the resume-by-stem handle contract. Caller-visible
change: the lead gains a synchronous delegation tool that returns per-item
results and durable 3-word agent handles.

## Phases

### Phase 1: read-only delegation with fan-out and stable handles

Register `ws-claude` with the `audit` and `consult` presets (read-only), the
array-in/array-out fan-out with `Promise.allSettled` and the concurrency cap,
the closed-tool profile (`strictMcpConfig`, no git/Bash), and 3-word stem
handles returned per item. Embedded prompts for both presets, authored per
`skill-authoring.md`.

Gate before building anything else: a throwaway SDK probe confirming that the
`claude_code` preset plus a small natural task-frame append does **not** trigger
the `400 out of extra usage` classification (the provider finding showed the
preset-plus-*Pi-prompt* append does; a small natural append is expected to pass
but must be confirmed).

Verification: fake-SDK tests for fan-out result alignment, per-item error
isolation, never-settling child timeout and cancellation/process cleanup; a live gate (owner-run, subscription login) showing an `audit` and a
`consult` item returning against a real ticket; confirmation that closed tools
(git/Bash/connectors) are absent from the spawned agent.

### Phase 2: edit-scoped rewrite

Add the `rewrite` preset and the `edit-targets` whitelist permission model:
presence of a non-empty `edit-targets` grants `Edit`/`Write` scoped to those
paths via a `canUseTool` hook that denies out-of-scope writes; edits land
unstaged and `changed` reports touched paths; the lead commits. Guard the
nonsensical preset/`edit-targets` combinations. Include the convention-read
mitigation for prompt drift.

Verification: `canUseTool` denies a write outside `edit-targets`, including
relative-path traversal, absolute out-of-worktree paths, symlink escape and
new-file parent escape; overlapping canonical targets are rejected; a fanned-out
pair of rewrite items touching disjoint files leaves both edited and unstaged;
`rewrite` without `edit-targets` and a read-only preset with `edit-targets` are
both rejected.

### Phase 3: resume and the playbook-backed design-review preset

Add resume: capture each agent's Claude session id, map it to its 3-word stem,
and continue via native `--resume` on a `{ resume: <stem>, request }` call.
Session-local map first (resume within a Pi session); persisting the map across
Pi restarts is a later edition if wanted. Add the `design-review` preset whose
prompt uses `ticket-reviewer-design` and the confirmed read-only context bundle.

Verification: a resumed agent continues with prior context (a fact stated in the
first call is available in the follow-up); an unknown stem errors cleanly; the
`design-review` preserves the canonical criteria/verdict format while resolving
its ticket/Relations inputs and lookup context without unavailable tool calls;
missing context is reported explicitly, lead credentials are absent, and no
additional MCP or execution tools become available.

## Pre-build live gate retained (2026-09-09)

The owner's backlog review refers to this subprocess-as-Claude-Code direction,
not to restoring the reverted Claude-as-Pi-model provider. This ticket was
already in `ready/`, but its recommended review posture and pre-build gate were
unresolved; folder placement alone did not make it dispatchable.

The Phase 1 throwaway SDK probe with the normal `claude_code` preset plus a
small natural task frame remains a hard pre-build gate. No probe was executed
by this ticket-authoring session, and no subscription-path success is claimed.
The separate post-build live audit/consult and tool-profile checks also remain.
The owner confirmed `ticket-reviewer-design` on 2026-09-09. Fresh independent
design and completeness reviews resolved the authoring block after the bounded
settlement, canonical edit containment, ticket/Relations inputs and closed-profile
context adaptation were captured. These verdicts do not clear the live probe gate.
