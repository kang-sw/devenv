---
title: "Open Decision Queue: ticket-held state, standard response format, promotion gate"
related:
  260730-feat-odq-batch-interview: amended - its "tooling holds state" is reversed and its full-text-every-response rule is narrowed to first presentation
  260726-bug-open-decision-queue-ledger-illegible: kept - self-describing item text and response-body conveyance stay; its one-line roll-up of the rest is narrowed to this round's settlements
  260611-chore-lead-discussion-gap-discipline: amended - its visible task-list primitive for the queue is removed
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: c25fbf49ff7ac72a
sage-review-completeness-reviewed: c25fbf49ff7ac72a
---

# Open Decision Queue: ticket-held state, standard response format, promotion gate

## Background

The Open Decision Queue (ODQ) guidance - the `lead-ticket` playbook section
plus its included task-list guidance (`task-list.md` / `task-list.codex.md`) -
specifies only the harness task-list surface: one item per decision,
self-describing item text, status tags, and "keep recommendations out of the
list". For the response body the task-list guidance says only that "the
decisions themselves travel through the response body", and the playbook
section adds only that the whole queue is asked in one response, "each item
restated in full, your recommendation for it in the response body rather than
in the item text" (`agents-plugin/rsrc/lead-ticket/lead-ticket.md#L43-L49`).
Where within the response the recommendation goes, how items are numbered,
and how later rounds re-present items are left to each response, so the layout drifts between rounds and confuses the user answering
it. Re-printing every item in full each round costs output tokens for no new
information.

The task list itself has shown little practical value: the same queue is
carried by the task list and the response body, and the task list is the
weaker channel (truncation, harness-dependent rendering). Its one real job -
holding queue state across the lead's own compaction - is better served by a
file.

## Decisions

### Queue state lives in the target ticket

- **Temporary ticket section.** Queue state is held in a temporary
  `## Open Decision Queue` section of the target ticket. If the ticket does
  not exist yet, create it first with `tickets.create_empty`. Each item
  records its ID, the one-line decision, its context, the lead's
  recommendation, and its status. Ticket text stays English (the ticket-wide
  rule), whatever the conversation language. The section header records the
  last ID used, so the "next unused number" survives compaction after settled
  items have left the section.
- **Multi-ticket items.** An item that affects several tickets (a batch
  coherence `missing` issue, a gap spanning members) is recorded in the
  section of every affected ticket, under the same ID, so each affected
  member is gated until it settles. IDs are unique per conversation: the
  next ID is one past the highest ID already used across every affected
  section and earlier rounds of the conversation, so a section re-created
  after deletion (for a reviewer `missing` issue) continues the numbering
  instead of restarting at `(1)`.
- **Non-authoritative until moved out.** The section is explicitly
  non-authoritative: it is an exception to "persist only confirmed
  decisions", in the same spirit as research tickets' Proposals / Open
  Questions. Research tickets do not use this section; they keep their
  Outcome Ledger.
- **Settled items leave the section.** A confirmed item moves into
  `## Decisions`. A rejected item moves into the relevant decision's
  "Rejected:" text when it is a meaningful alternative, and is otherwise
  dropped. A deferred item moves into `## Constraints` as out of scope. The
  lead deletes the section only after the final confirmation is approved, so
  an item reopened by a correction at that step still has its home in the
  section (it moves back from `## Decisions`). Rejected: keeping
  one-line settled records inside the section - conflicts with the
  section-exists gate below.
- **Working edits vs. persisting.** Moving items within the ticket file is
  uncommitted working state. "Persisting" means the commit and any `ready/`
  move; the final confirmation below gates that. The playbook's `## Write`
  rules ("persist only decisions the user confirmed", "never write a draft
  decision for later correction") gain this one exception as a separate
  sentence: the temporary section is non-authoritative working state, not a
  persisted decision. The first `## Write` sentence ("Persist only decisions
  the user confirmed. Research tickets may preserve ...") is pinned verbatim
  by `agents-plugin-tool/internal/mcp/research_outcome_test.go` (b833d898)
  and stays unchanged; it remains true under this definition of persisting.
  The capture and intent checklists are satisfied against the text after the
  section is deleted; their confirmed-only wording (b833d898) stays.
- **Harness task list removed.** The ODQ no longer uses a harness task list.
  Delete `task-list.md` and `task-list.codex.md` (canonical, wsflow, and pi
  trees), remove `includes: - task-list` and the "visible task list" wording
  from `lead-ticket.md`, and fold the surviving state rules (stable IDs,
  self-describing decision text, status tags) into its `## Open Decision
  Queue` section. Rejected: keeping the task list as the state record - a
  weaker duplicate of the response body, and a file survives compaction
  better.

### Response format

- **First presentation: one full block per item.** An item's first
  appearance, including an item added mid-settlement (a fact populator
  `decision_gaps:` entry, a reviewer `missing` issue), is a block:

  ```text
  # Open Decision Queue

  (1) <one-line decision>
  - <context bullet>
  - <alternative: ...>
  > <recommendation and why>
  ```

  The recommendation lives only in the trailing `>` line of the item's
  block. Rejected: a table (`| ID | decision | recommendation | status |`) -
  context does not fit in cells and wraps badly in a terminal; a
  recommendation section separate from the items - the placement drift this
  ticket removes.
- **Later rounds: no full re-print.** An item still awaiting an answer is
  re-presented as one line, `(n) [open] <one-line decision>`; its full
  context is re-printed only when the user asks, and the lead may point at
  the ticket section instead. Items settled in this round are announced in
  one line (for example `(1), (2) confirmed · (4) deferred`). Items settled
  in earlier rounds are not re-printed. Rejected: collapsing every settled
  item to a one-line entry each round - still grows with queue size; the
  goal is output cost reduction.
- **Stable parenthesized-number IDs.** Items are numbered `(1)`, `(2)`,
  `(3)`, ...; an ID never changes across rounds, and a new item takes the
  next unused number. The parentheses keep them distinct from phase or list
  numbering. Rejected: letter IDs (`A`, `B`, ...) - switching input
  language to type a Latin letter is friction for users who answer in a
  non-Latin-script language.
- **Language.** The `# Open Decision Queue` heading and status tags
  (`[open]`, `[confirmed]`, `[rejected]`, `[deferred]`) stay in English; item
  content in the response follows the user's conversation language.

### Final confirmation

- **Blocking confirmation before persisting.** Before the commit and any
  `ready/` move, show the confirmed items in full once (rejected and deferred
  items in one line) and end the turn; persist only after the user approves.
  It runs at queue settlement - after every item is settled, before the
  section is deleted, and so before `sage_gate` and the reviewers - and only
  for an invocation that had queue items; commits with no queue (drops,
  stamp-only commits) do not add this turn.
- **Correction reopens.** A correction at that step returns the item to
  `[open]` under its existing ID, and it is reconciled like any other open
  item. Rejected: showing the list and persisting in the same response - the
  check would come after the fact.

### Promotion gate (tooling)

- **Section-exists refusal.** `tickets.move(to: "ready")` and
  `tickets.sage_gate` (both the `ready` landing and the epic `todo` landing)
  refuse when the ticket contains a `## Open Decision Queue` section, whatever
  its item statuses. Detection matches an exact level-2 line
  `## Open Decision Queue` (trailing whitespace allowed) and ignores fenced
  code blocks, so example text such as this ticket's own is not a false
  positive. Both refusals run before any write (a genuine no-op, per
  2383bddf); in `sage_gate` the check runs after landing validation and the
  `idea` skip, applies only to the two gated landings (so the research and
  workset `todo` exemptions keep their behavior), and precedes the
  route-facts, posture-skip, and freshness branches, including the retained
  epic-at-ready branch. It is reported as a new structured stop action
  (e.g. `stop_open_decision_queue`) consistent with
  `stop_missing_route_facts`, which needs a case in the MCP layer's
  `sageGateNextInstruction` carrying the "settle the queue and delete the
  section" guidance and an entry in the `sage_gate` tool description's
  action list (`agents-plugin-tool/internal/mcp/server.go`). Rejected:
  refusing only
  when an `[open]` item remains - depends on parsing item status and lets a
  section of stale records reach a worker; enforcing by playbook rule only -
  an unsettled recommendation reaching `ready/` is read by the worker as a
  decision, and a rule alone leaks; also gating an epic's
  `tickets.move(to: "todo")` - an epic's `todo/` is its ordinary accepted
  board state, ungated like an actionable `todo/`, so a pending section there
  is allowed; the epic's design review (the `sage_gate` `todo` landing) is
  the gated point, and children derive only from authoritative decisions.
- **Review loop unchanged.** A reviewer `missing` issue goes back into the
  section as a new item; after it settles and the section is deleted, the
  existing fix-then-re-stamp path applies.

### Thought experiment before applying a change

- **Trace every incoming change through the ticket before writing it.**
  Before any content enters the ticket mid-settlement - a user correction,
  pushback, or change of direction; a new proposal of the lead's own; an item
  brought in by the fact populator or a reviewer - the lead runs a thought
  experiment: apply the change to the ticket as written and trace its
  consequences through the ticket's `## Decisions`, `## Constraints`,
  phases, verification expectations, and `## Prior Decisions`, and through
  the rules of the workflow document the ticket changes (for a ticket that
  edits a playbook, that playbook's rules). Contradictions are resolved
  before the change is reflected; every new decision the resolution needs is
  raised in the same response as a new item, not discovered one reviewer
  round later. The trace runs before the change is recorded anywhere in the
  ticket, including recording a new item in the temporary section; the
  initial draft, written before any queue exists, is covered by the same
  instruction.
- **Pending knock-ons hold the triggering change.** While knock-on items the
  trace raised are open, the triggering change stays in the temporary
  section (confirmed, not yet moved to `## Decisions`) until they settle. A
  resolution that would alter an already-confirmed decision is raised as a
  reopened item, never applied by the lead on its own.
- **Behavior, not mechanism.** This is a reasoning instruction in the
  playbook's ODQ section. Rejected: a new checklist item, tool step, or
  gate - the observed failure (a settlement that chains through several
  review rounds because each change's knock-on contradictions surface late)
  is one of the lead not thinking a change through, which a mechanism does
  not supply. Rejected: triggering only on user pushback - in the motivating
  session about half of the late contradictions came from the lead's own
  proposals.

### Unchanged and amended semantics

- **Unchanged.** Queue membership, reconcile item by item, stating the
  lead's reading of an answer whose reach is unclear, proceeding only when
  every item is confirmed/rejected/deferred, and only confirmed items being
  authoritative.
- **Amended prior decisions.** 260611-chore-lead-discussion-gap-discipline's
  "the Open Decision Queue uses a visible task-list primitive when the harness
  exposes one" is reversed by the task-list removal above.
  260730-feat-odq-batch-interview's "tooling
  holds state, the transcript carries conveyance" is reversed for its state
  half: state moves from the task list to the ticket section. Its "one
  response carries the full text of every open item" is narrowed to the first
  presentation. Its "recommendations live in prose only, never in the
  ledger" is reversed for the ticket section: each item records the lead's
  recommendation there, since a file has no task-list truncation.
  260726-bug-open-decision-queue-ledger-illegible's "one-line status roll-up
  of the rest" is narrowed to announcing only this round's settlements.
  260726-bug-open-decision-queue-ledger-illegible's
  self-describing item text and response-body conveyance are kept: the
  one-line `(n) [open]` re-ask is the decision text itself, printed in the
  response body. The `lead-ticket` sentences "each item restated in full" and
  "re-ask what the answer did not reach as one batch" are reworded to match.

## Constraints

- Phase 2 edit targets: the `## Write` and `## Open Decision Queue` sections
  and frontmatter of `agents-plugin/rsrc/lead-ticket/lead-ticket.md` (plus
  any promotion/review step text that must order the final confirmation and
  section deletion before `sage_gate`); deletion of
  `agents-plugin/rsrc/lead-ticket/task-list.md` and `task-list.codex.md`; the
  same changes in the wsflow mirror `agents-plugin-wsflow/rsrc/lead-ticket/`
  and the byte-identical pi mirror `agents-plugin-pi/rsrc/lead-ticket/`
  (guarded by `TestPiMirrorUpToDate`); and regenerating
  `agents-plugin/rsrc/manifest.json`
  (`ai-docs/manuals/wsflow-mirroring.md#L269-L278`,
  `agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go#L20-L30`).
- The include mechanism itself (`includes:` with harness-specific variants)
  stays; `agents-plugin-tool/internal/wsrsrc/wsrsrc_test.go` exercises it with
  synthetic fixtures, not these files.
- Shipped text: follow `ai-docs/manuals/skill-authoring.md`,
  `ai-docs/manuals/wsflow-mirroring.md`, and
  `ai-docs/manuals/shipped-surface-boundary.md`. The example strings above are
  illustrative; shipped text must not depend on any one conversation
  language.
- Out of scope: ODQ admission criteria
  (260726-feat-doc-organization-autonomy-odq-admission-filter), and any change
  to the research Outcome Ledger.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/`)

## Prior Decisions

- 2383bddf (2026-09-13, commit): "Promotion closure is deliberately status-only (ready/ or .done/): tickets.move is batch-unaware... Placed before any write so the refusal is a genuine no-op." — bearing: constrains
- b444a501 (2026-09-11, commit): "SageGate's epic-at-ready branch is retained, not deleted: tickets.move and sage_gate are decoupled, so barring the move does not bar the direct gate call" — bearing: supports
- b833d898 (2026-09-11, commit): "Scope the Open Decision Queue exception to non-authoritative research entries so general settlement instructions cannot negate the approved research capture contract." — bearing: constrains
- 260919-feat-mailbox-arm-wait-harness-include (2026-09-19, Result): "Registered both new files in `manifest.json` and resynced the `agents-plugin-pi` and `agents-plugin-wsflow` rsrc mirrors via manual `rsync` (not `bump-ws-version.sh`, which is release-scoped...)" — bearing: constrains
- 260730-feat-odq-batch-interview (2026-07-30, Decisions): "Tooling holds state, the transcript carries conveyance. The visible list keeps the item set and each item's open/confirmed/rejected/deferred status — that is what survives the lead's own compaction" — bearing: constrains
- 260726-bug-open-decision-queue-ledger-illegible (2026-07-26, Decisions): "Restating each item in the response body is the documented default, not a recovery. This is the load-bearing half: it is harness-independent, so it cannot silently degrade." — bearing: constrains
- 260726-feat-doc-organization-autonomy-odq-admission-filter (2026-07-26, Decisions): "The queue mechanism is not being weakened or removed. Only its admission filter is in question." — bearing: supports
- 260611-chore-lead-discussion-gap-discipline (2026-06-11, Decisions): "The Open Decision Queue uses a visible task-list primitive when the harness exposes one. Codex uses its plan/task-list surface" — bearing: contradiction-candidate

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdoc/tickets_mutate.go, agents-plugin-tool/internal/wsdoc/tickets_sage.go, agents-plugin/rsrc/lead-ticket/lead-ticket.md, agents-plugin/rsrc/lead-ticket/task-list.md, agents-plugin/rsrc/lead-ticket/task-list.codex.md, agents-plugin-wsflow/rsrc/lead-ticket/, agents-plugin-pi/rsrc/lead-ticket/, agents-plugin/rsrc/manifest.json, agents-plugin-tool/internal/mcp/server.go (sageGateNextInstruction case, sage_gate description action list), agents-plugin-tool/internal/mcp/playbook_tools_test.go |
| scope.surface | public-interface | new refusal on the ws MCP tools tickets.move and tickets.sage_gate plus shipped lead-ticket playbook text in three packages |
| scope.new_public_symbol | no | none; the gate is a precondition inside TicketsMove and SageGate |
| scope.new_type_contract | yes | new sage_gate structured stop action value (e.g. stop_open_decision_queue) on the caller-visible action enum; tickets.move refusal is an error return |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdoc/tickets_mutate_test.go, agents-plugin-tool/internal/wsdoc/tickets_sage_test.go, agents-plugin-tool/internal/mcp/playbook_tools_test.go TestPlaybookPrintGoldenLeadTicket, agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go, agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go, agents-plugin-tool/internal/mcp/research_outcome_test.go (pins the first ## Write sentence verbatim) |
| complexity.reuse_points | confirmed | pre-write refusal slot in TicketsMove to-ready block tickets_mutate.go#L193-L213 and SageGate landing branches tickets_sage.go#L127-L210; no fence-aware heading helper exists, blockedHeadings in tickets.go#L681-L694 is prefix-only |
| complexity.side_effect_risk | moderate | the refusal sits on every ready promotion and epic todo landing and must stay a pre-write no-op; deleting the include files changes the rendered lead-ticket in all three packages |
| risk.correctness | moderate | fenced-code exclusion, both sage_gate landings, and refusal before sage-review frontmatter writes must all hold |
| risk.fit | moderate | reverses recorded ODQ task-list decisions and rewrites shipped playbook prose mirrored byte-identically across three trees |
| risk.test | moderate | golden test must be rewritten for removed include and new pins, plus new gate tests including the fenced false-positive case |
| risk.security_or_contract | moderate | adds a caller-visible refusal to the tickets.move and tickets.sage_gate MCP contracts |

## Phases

### Phase 1: Promotion gate on a pending ODQ section

Implement `## Decisions` -> "Promotion gate (tooling)" in `tickets.move` and
`tickets.sage_gate`. The refusal names the section and tells the lead to
settle the queue and delete the section first.

Verification expectations:

- `tickets.move(to: "ready")` refuses a ticket containing a
  `## Open Decision Queue` section and accepts it once the section is
  removed.
- `tickets.sage_gate` refuses on both the `ready` landing and the epic
  `todo` landing while the section exists.
- A `## Open Decision Queue` heading inside a fenced code block does not
  trigger the refusal; tests pin the exact heading-match rule.
- After a refusal, the ticket file's bytes and location are unchanged (no
  sage-review frontmatter write, no move).
- Existing gate tests keep passing.

### Phase 2: lead-ticket playbook rewrite and task-list removal

Depends on Phase 1 (the playbook states the gate as existing behavior).
Apply the rest of `## Decisions` to the Phase 2 edit targets in
`## Constraints`.

Verification expectations:

- Playbook render/package tests pass, including wsflow and pi mirror drift
  checks; `TestPlaybookPrintGoldenLeadTicket` is updated for the removed
  include and new text rather than merely re-run.
- Rendered `lead-ticket` (Claude and Codex harness variants) contains no
  task-list guidance, and pins by test the first-presentation block
  template, the `(n) [open]` one-line re-ask, the ticket-section storage and
  settlement moves, the blocking final confirmation, and the
  thought-experiment instruction.

## Sage Review Round 1 (2026-09-24)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Unnamed reversal of 260730's 'Recommendations live in prose only, never in the ledger' (and 260726 roll-up narrowing) | important | missing |
| 2 | Phase 2 edit targets leave lead-ticket ## Write confirmed-only rules contradicting the ticket-held queue | important | autonomous |
| 3 | Order of final confirmation, section deletion, and sage_gate unspecified; no-queue commits scope | minor | autonomous |
| 4 | Stable-ID counter is not held in the file | minor | autonomous |
| 5 | Queue items spanning several tickets have no stated home | minor | autonomous |
| 6 | Shape and placement of the sage_gate refusal | minor | autonomous |

### Completeness Reviewer — concern

| # | Title | Severity |
|---|-------|----------|
| 1 | Section deletion vs. final-confirmation correction ordering | important |
| 2 | Final confirmation trigger scope | minor |
| 3 | Pre-write no-op not in Phase 1 verification | minor |
| 4 | Heading-match rule unspecified | minor |
| 5 | Amended ticket 260611 missing from related frontmatter | minor |
