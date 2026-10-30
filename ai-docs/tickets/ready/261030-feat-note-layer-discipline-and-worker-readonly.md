---
related:
  261030-bug-workflow-manual-sentinel-mints-lead-key-for-subagents: the worker note read-only gate is bypassable through the sentinel hole (out of scope)
title: Note layer discipline - worktree-first writes, worker read-only notes, bounded ambient Notes block
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: dae665eecf19267a
sage-review-completeness-reviewed: dae665eecf19267a
---

# Note layer discipline - worktree-first writes, worker read-only notes, bounded ambient Notes block

## Background

`note.write` takes a required `layer` enum with no default and no preference
signal (`agents-plugin-tool/internal/mcp/server.go` note.write schema); the only
steering is the workflow manual's "Use the narrowest layer that reaches its
readers" (`agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md`,
`### Notes / durable memory`). The only write-time check is the >=300-byte
oversize nudge (`internal/mcp/note_tools.go`, `noteOversizeChallengeFor`).

Notes accumulate unbounded. On the owner's machine (2026-10-30) one project's
clone store held 121 notes / 182 KB and another 17.6 KB clone + 12.9 KB
worktree. The ambient `# Notes` block (`internal/wsnote/inject.go`, `Compute`)
inlines the top `notesInjectionCap = 20` (`internal/mcp/note_announcement.go`)
and reduces everything else to one count line, so elided notes show not even a
key and are effectively undiscoverable.

Any session scope can write notes today: `roleAllowsTool`
(`server.go`, `func roleAllowsTool`) lets delegate and leaf keys call every
`note.*` tool.

## Decisions

- **Clone/machine writes require a reason.** `note.write` with `layer:
  "clone"` or `layer: "machine"` requires a non-empty `reason` string argument
  stating why the note must reach beyond this worktree; the call is rejected
  without it. The reason is not stored on the record and is not echoed into the
  Notes block. Worktree and repo writes take no reason.
  Rejected: a post-write soft nudge only (does not force the decision at write
  time); storing the reason (grows the very notes this ticket bounds).
- **Worktree-first wording.** The `note.write` and `note.query` schema layer
  descriptions present `worktree` first, and the `clone`/`machine` entries
  state their use criterion: write there only when a session in another
  worktree (clone) or another project (machine) would fail without the fact.
  The workflow manual's `### Notes / durable memory` sentence is aligned with
  the same rule, and the paragraph also says that a spawned worker puts such
  a fact in its final report instead of writing a note, so the manual, the
  footer, and the denial message agree.
- **Workers are note read-only.** For every session key that is not a
  top-level lead, `note.query` stays allowed, and `note.write`, `note.erase`,
  `note.mute`, and `note.unmute` are denied. "Not a top-level lead" means
  delegate and leaf keys, plus lead-scoped keys that carry a parent, such as
  ticket-workers and Pi forks (`session_auth.go`, `isTopLevelLead`). The
  check therefore sits beside the role gate rather than in `roleAllowsTool`
  alone. The owner's intent covers every spawned worker, and a ticket-worker
  key is minted lead-scoped with the lead as parent (`playbook_tools.go`,
  the `"worker"` role mapping). Update the `childRoleForPlaybookRole` comment
  ("trusted at lead level by design") and the `isTopLevelLead` doc comment
  in the same change. Only spawner-launched forks carry a parent; a
  human-started Pi `/fork` gets a fresh top-level key and stays a lead, so
  the fork test builds a spawned-fork-shaped key (lead scope, non-empty
  parent). The denial for
  these four tools carries a note-specific message telling the caller to put
  the note content in its final report so the lead can write it, instead of
  the generic `tool not available in current ws MCP profile` text. Mute and
  unmute count as writes because they change state every session sees.
  Because the Notes footer tells every reader to erase stale notes, the
  denial message says that erasing is the lead's job, so the footer and the
  denial agree for workers. Rejected: a new elevation tool (workers already
  report to the lead).
- **Machine notes are pinned.** In the ambient `# Notes` block, visible
  machine-layer notes always render first and are exempt from every cap and
  from the counts below. This supersedes the shared priority-ordered cap
  across layers recorded in 260810-feat-repo-tracked-note-layer's Result. Muted machine notes stay hidden, as muted notes do
  today.
- **Bounded non-machine display.** Non-machine notes (worktree, clone, repo),
  ordered by the existing `CompareRecords` comparator:
  - the first 20 render with their body, as today;
  - the next 30 render key-only, each key layer-tagged, on one line;
  - anything beyond 50 renders as a `+N hidden` line, on every render; it
    replaces today's elided-count line.
  Machine notes keep `CompareRecords` order among themselves. A `reason`
  that is empty after trimming counts as missing.
- **Hidden-notes cleanup nudge.** When the `+N hidden` line is present, a
  strong cleanup warning is shown as well. It gives the hidden count with its
  per-layer breakdown and the remediation: `note.query` the hidden notes,
  `note.erase` the stale ones, and move still-needed detail to a ticket or
  manual (not mute). The warning is throttled to at most once per day per
  clone, with a throttle file in the git common dir, following the worktree
  pool nudge (`internal/mcp/worktree_nudge.go`, `worktreeNudgeThrottleFile` /
  `worktreeNudgeInterval`).
  - Placement: the warning is not part of the `# Notes` block. It is
    prepended to the `workflow_manual` response through
    `injectBootstrapStalenessWarning` (the banner region). The claude-mod's
    `manualPart` keeps that region in the system prompt, because it cuts at
    `## Session Key`.
  - Who consumes the slot: only a `workflow_manual` call by a top-level lead
    (`isTopLevelLead`: lead scope, empty parent) renders the warning and
    consumes the daily slot. This holds in both the CONTINUE branch and the
    fresh-sentinel-with-root branch, which mints a top-level lead. Other keys
    never render or consume it: ticket-workers, Pi forks, delegates, and
    leaves. Neither does `workflow_state`. Workers, who are note read-only,
    therefore cannot spend it. Accepted residual: a claude-mod session-start
    setup that finishes after the compose wait spends the slot without its
    section reaching the snapshot. The next day self-heals this; note it in a
    comment at the throttle site. Known bypass: until
    261030-bug-workflow-manual-sentinel-mints-lead-key-for-subagents is
    fixed, a subagent's fresh-sentinel call mints a top-level lead and can
    consume the slot.
  - The `+N hidden` line stays in the `# Notes` block on every render. The
    block therefore stays byte-identical between `workflow_manual` and
    `workflow_state` (the invariant at `workflow_manual.go`,
    `handleWorkflowState`).
  - Rejected: once per session key per 24h, because it repeats on every new
    session, the condition is a store property, and one session's cleanup
    resolves it. Rejected: placing the warning inside the Notes block,
    because the mod's session-start call would consume the slot while
    discarding the block, and a worker could consume it without being able
    to act. The lead decided the placement on 2026-10-31 under the owner's
    overnight delegation of minor policy calls.
- **Stale-note guidance.** The block's closing line (`inject.go`, the
  "_Post-it reminders, ..._" footer) is rewritten as one line that keeps the
  "Post-it reminders" lead-in and the "higher priority number first"
  direction, and says to erase stale notes immediately on sight with
  `note.erase`, e.g. `_Post-it reminders, higher priority number first:
  erase stale ones immediately with note.erase._`. ba6328df put the priority
  direction there after downstream agents flipped it, and Pi tests match the
  "Post-it reminders" substring. The empty-state
  text is unchanged. Rejected: a new sentence in the manual body (three
  mirrored copies; the footer renders next to the notes on every call).

## Constraints

- The shipped v0047 migration text in
  `agents-plugin/skills/lead-bootstrap/AGENTS.template.md` (its clone
  `note.write` example) gains the `reason` argument in the same change.

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin-wsflow/rsrc/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Out of scope: a total-size (10 KB) warning — deferred by the owner.
- Out of scope: `agents-plugin/rsrc/executor-wrapup.md` (and its wsflow/pi
  copies) still instructs a `note.write(layer: "repo")`, but no playbook loads
  it (only `rsrc/manifest.json` and `playbook_tools_test.go` reference it);
  its cleanup is a separate decision.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` (every MCP string
  here ships), `ai-docs/manuals/ws-mcp.md` (`internal/mcp/`),
  `ai-docs/manuals/skill-authoring.md` and
  `ai-docs/manuals/wsflow-mirroring.md` (the manual sentence and its
  wsflow/pi mirrors).

## Prior Decisions

- 260823-feat-notes-postit-discipline (2026-08-23, Decisions): "Non-empty (trailing, one line under the notes list): _Post-it reminders: `note.write` to pin, `note.erase` when done." — bearing: constrains
- ba6328df (2026-10-20, commit): "Put the order in the existing trailing post-it line rather than the "# Notes" header, since Pi tests match "\n# Notes\n" and "Post-it reminders" substrings." — bearing: constrains
- 260811-feat-note-visibility-mute (2026-08-11, Decisions): "Muted notes leave the injection entirely, including the cap budget. A muted note is excluded from the `# Notes` block and does not consume a `notesInjectionCap` slot" — bearing: supports
- 260810-feat-repo-tracked-note-layer (2026-08-12, Result): "the `# Notes` block renders `repo`-layer records alongside `machine`/`worktree` under the same priority-ordered cap." — bearing: constrains
- 261002-feat-ws-config-adapter-schema-extension (2026-10-02, 1ff2e25a): "config.get is exempt from the config.* delegate/leaf gate (read-only) so a spawned child reads the session value its lead tuned" — bearing: supports
- 260814-bug-wsflow-runtime-contract-missing-note-mute-unmute (2026-08-14, fbec365f): "Chose option (A) — passive primitives-reference prose only, no Go-runtime ambient nudge — to keep blast radius to doc" — bearing: constrains
- 260823-feat-notes-postit-discipline (2026-08-23, Decisions): "Three render states, not two. ... Key the empty-state post-it hint off the existing empty-skip predicate (no notes on any layer)" — bearing: constrains
- 774a4930 (2026-10-30, commit): "Worker note read-only rides the note-discipline ticket: same note.* surface, and its denial text cites the same layer rules." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/server.go, internal/mcp/note_tools.go, internal/mcp/note_announcement.go, internal/wsnote/inject.go, agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md plus wsflow and pi mirrors |
| scope.surface | public-interface | note.write schema gains a conditional reason argument; role gate denies note.write/erase/mute/unmute to non-top-level keys (delegate, leaf, parented lead) |
| scope.new_public_symbol | yes | new note.write reason argument (MCP tool surface); no new exported Go symbol identified |
| scope.new_type_contract | yes | note.write input contract: reason required for layer clone or machine |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/note_tools_test.go, internal/mcp/note_workflow_manual_test.go, internal/wsnote/inject_test.go |
| complexity.reuse_points | confirmed | CompareRecords (wsnote/search.go), worktreeNudgeThrottleFile and worktreeNudgeInterval (mcp/worktree_nudge.go), roleAllowsTool (mcp/server.go) |
| complexity.side_effect_risk | moderate | ambient Notes block is rendered in every workflow_manual call and pinned by Pi fixtures and substring tests |
| risk.correctness | moderate | tiered display, machine pinning, muted exemption, and per-clone throttle interact in Compute |
| risk.fit | moderate | footer and manual sentence are mirrored across ws, wsflow, and pi copies; manifest hashes must regenerate |
| risk.test | moderate | throttle file in git common dir needs a temp-repo fixture; Pi fixtures embed the old manual sentence and footer |
| risk.security_or_contract | moderate | denies existing tool access to delegate, leaf, and parented lead keys and adds a required argument to a public MCP tool |

## Phases

### Phase 1: Note layer discipline

Implement every decision above in `agents-plugin-tool/internal/mcp/` and
`internal/wsnote/`, plus the manual sentence and its mirrors.

Verification: Go tests covering the `reason` rejection per layer, the
non-top-level denial for delegate, leaf, and ticket-worker-shaped keys
(write/erase/mute/unmute denied, query allowed, message text), the warning
throttle with a ticket-worker-shaped key (no render, no consumption), machine pinning and cap exemption including muted machine notes, the
20/30/`+N hidden` boundaries, the once-per-day-per-clone throttle, and the new
footer; the existing note and workflow_manual test suites, the playbook
manifest and mirror drift tests, and the pi test suite (its fixtures carry
the manual sentence) pass.
