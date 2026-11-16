---
title: Fill the gaps left by the lead-ticket / ticket-handler split
related:
  261105-feat-lead-ticket-review-auto-ack: knob mechanism this ticket replaces
  261105-feat-whiteboard-knob-harness-and-session-scope: whiteboard knob injection this ticket re-anchors
sage-review-design: required
sage-review-completeness: required
---

# Fill the gaps left by the lead-ticket / ticket-handler split

## Background

Commits 753749802 (split lead-ticket promotion into a `ticket-handler` worker),
39481c64f (route lead-run stop (c) through a handler `revise` operation), and
779292e08 (lead playbooks open as manuals) rewrote
`agents-plugin/rsrc/lead-ticket/lead-ticket.md` into a compressed manual and
moved fact population, review gates, reviewer fixes, stamps, the `ready/` move,
and the commit into `agents-plugin/rsrc/ticket-handler/ticket-handler.md`.
Tests were not run for the first commit. A read-only audit on 2026-11-16 found
these gaps (evidence by inspection; no `go test` was run):

- G1 (behavior-breaking): the `ticket_review_auto_ack` knob is applied only on
  `playbook.read` and only to `lead-ticket`
  (`applyTicketReviewAutoAckVariant` in `internal/mcp/ticket_review_text.go`,
  called from the `playbook.read` case in `internal/mcp/server.go`). All five
  on-texts in `ticketReviewAutoAckVariants` are gone from rsrc, so with the
  knob off `replaceOrAppend` appends the off-texts (about the removed
  acknowledgement flow; the on-texts, not the off-texts, cite the removed
  `### Item classes`: `internal/mcp/ticket_review_text.go#L29`, `#L56`,
  `#L64`) to the end of lead-ticket. The handler, rendered through
  `playbook.render`, always auto-confirms, whatever the knob says.
- G2 (failing test): the whiteboard knob anchor `lineTicketQueueStateEnd`
  (`internal/mcp/whiteboard_text.go`) targets `### Trace a change before
  writing it`, which no longer exists, so the queue bullet is appended after
  lead-ticket's final `Ticket:` output rule; `TestWhiteboardWorkflowTextFollowsKnob`
  asserts the removed `### Queue state` / `### Trace a change...` headings.
- G3 (behavior gap): the handler has no path for `tickets.sage_gate` actions
  `ask` (ask the user whether to run a review), `check_review_required`, and
  `stop_blocked`; it has no user channel and no matching stop.
- G4: tool next-instruction texts still address the old single-lead flow
  (`tickets.move` to ready says to call `sage_gate` after the move; queue
  refusals say "settle the queue with the user"; posture changes say "commit
  before proceeding"; `stop_blocked` names a nonexistent "On: Reviewer Spawn").
  Sources: the sage_gate/move response builders in `internal/mcp/server.go`,
  `openDecisionQueueRefusal` in `internal/wsdoc/tickets_mutate.go`.
- G5: lead-ticket says "render the handler in that worktree" for an occupied
  root and for `revise` without naming `playbook.render`'s `root_override`.
- G6: stale wording elsewhere: the lead-ticket `SKILL.md` description (ws and
  wsflow copies) still says it owns fact population and the design-review gate;
  `ticket-fact-populator.md` and `ticket-reviewer-design.md` say "the lead"
  where the caller is now the handler; `ai-docs/manuals/skill-authoring.md`
  Layouts still says a lead skill opens with Identity, contradicting 779292e08.
- G7: `ticket_review_text.go` claims a test pins every on-text; none does.

## Decisions

- **Procedure knowledge lives in the subagent playbook (user directive,
  2026-11-16).** A lead playbook does not carry "when the handler reports X,
  do Y" rules. The handler playbook carries "in situation X, ask the lead in
  this form". The lead side only relays.
  Rejected: per-status response rules in lead-ticket (the shape the split left
  behind).
- **One generic ask channel from handler to lead.** The handler reports a
  single question status (working name `ask`) whose payload is
  self-describing: what is being asked and why, the user-facing wording, the
  options with a recommendation, the item class, and the exact reply format
  the handler expects back. lead-ticket's whole contract for it is: put the
  items to the user per the workflow manual's `### Asking the user` and the
  whiteboard rules, then send the user's answers back verbatim. Open Decision
  Queue questions, review-raised items that need the user, and the sage_gate
  `ask` action all use this channel; the existing `decisions` status folds
  into it. When the handler uses it is written only in ticket-handler.md.
  The implementer settles the concrete payload shape and final status name;
  the item kind tells an Open Decision Queue item (recorded in the ticket)
  from a one-off gate question (not recorded). The lead returns the user's
  answers verbatim, so the handler parses acknowledgements and per-item
  answers itself (`## Lead replies` changes accordingly); handler wording is
  English and the lead renders it in the user's language. Update any other
  playbook that names the old `decisions` status (check lead-run).
  Rejected: a dedicated `gate_ask` status (one status per cause re-grows
  lead-side rules); mixing gate questions into `decisions` (those items are
  written into the ticket's Open Decision Queue, a one-off gate question must
  not be).
- **The handler decides `check_review_required` itself**: on `revise`, rerun
  design review; decide completeness from the diff. **`stop_blocked`** is
  either fixed by the handler or, when it needs a decision, raised through the
  generic ask; otherwise the handler stops with its existing blocked report.
- **`ticket_review_auto_ack` is kept (user (02)) and reimplemented with
  override-point markers.** Wrap the handler's auto-confirm rule text in
  `<!-- ws:override:<pointId> -->` blocks (seed = knob-on text) and have the
  override lookup (`buildOverrideLookup` in `internal/mcp/playbook_tools.go`)
  return the knob-off text when the resolved knob is off. Markers go in
  ticket-handler.md only; lead-ticket keeps no auto-confirm statement (its
  current unconditional mentions move to the handler or are deleted). The
  markers are resolved in the shared render core, so `playbook.render` of the
  handler honors the knob resolved from the caller's session key. A user prompt
  override on the same pointId takes precedence over the knob: the knob-off
  text is a fallback consulted only after every bucket in
  `harnessBucketReadOrder` found no stored override (the lookup stops at the
  first bucket hit), and the knob is resolved with config defaults, as
  `ticketReviewAutoAckEnabled` does, not the prompt-override defaults the
  lookup's resolver is seeded with. The implementer names the pointIds and
  rewrites the off text for the handler's ask channel (the existing off-texts
  target the removed lead acknowledgement flow). Delete
  `ticketReviewAutoAckVariants`/`applyTicketReviewAutoAckVariant` and its
  `playbook.read` call; update the knob description text (server.go config
  description, `wsconfig/scope.go` comment) to name the handler.
  This reverses 261105-feat-lead-ticket-review-auto-ack's rejection of "rsrc
  variants or template conditionals for knob-off text" (user (06),
  2026-11-16): that choice only followed the code-side substitution precedent
  of the whiteboard knob, and the split showed it desyncs silently.
  Rejected: retiring the knob (deletes functionality users may rely on);
  re-pointing the sentence-substitution pairs at the new text (the same
  silent desync recurs on the next edit, and only a prose-anchor test could
  guard it); a new `{{.Var}}` placeholder (needs session-config plumbing in
  `buildPlaybookVars`, more than markers).
- **G2**: replace the whiteboard knob's lead-ticket sentence anchor with an
  override-point marker at the lead's relay point, using the same lookup
  fallback as the auto-ack knob (same reason as (06): a substring anchor
  desyncs silently). The bullet keeps covering every queue the lead puts to
  the user, its own drafting queue included, not only handler asks. The
  lead-discuss and workflow-manual whiteboard variants are out of scope.
  Delete the heading-index assertions from the test.
  Rejected: re-anchoring on another heading substring (same silent-desync
  shape).
- **G4**: reword the affected tool texts caller-neutral and correct for the
  handler flow (gate before move; "settle the queue" without "with the user";
  "commit with the caller's next commit"; drop every dangling
  "On: Reviewer Spawn", in the sage_gate `run` text as well as `stop_blocked`;
  replace hard-coded `ws/` tool names in these texts, including the stamp
  instructions in `tickets_mutate.go`, with the session's namespace). Delete
  test assertions that pin those sentences.
- **G5**: lead-ticket names `root_override: <worktree>` for the occupied-root
  and `revise` renders.
- **G6**: update both lead-ticket `SKILL.md` descriptions (and regenerate the
  skills manifest), say "the caller" in the populator and design reviewer, and
  change `skill-authoring.md` Layouts so a lead skill opens with what the
  playbook covers, not an Identity line (user (04); worker, reviewer, and
  delegate playbooks keep their identity lines).
- **Prose tests (user directive)**: add no test that pins English prose; in
  the area this ticket touches, delete assertions that pin prose sentences or
  removed headings (the G1 differential test's on/off text checks, the G2
  heading-index assertions, the G4 sentence pins) and the false G7 comment.
  Structural checks stay: manifest hashes, mirror byte-identity, variable
  substitution, code-parsed headings. Prose tests elsewhere are out of scope.
- **Compatibility: T0.** No MCP tool or argument, config key, or store shape
  changes; tool response wording changes; new override pointIds are additive.

## Constraints

- Manuals to read before editing (AGENTS.md Implementation Conventions):
  `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`,
  `ai-docs/manuals/ws-mcp.md`, `ai-docs/manuals/release-compatibility.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Shipped text stays downstream-neutral: no repository-specific names.
- Mirrors: `agents-plugin-wsflow/rsrc` and `agents-plugin-pi/rsrc` stay
  byte-identical to `agents-plugin/rsrc`; regenerate every rsrc `manifest.json`
  and the skills manifest.
- No local `go test` on the Windows host: Go tests run on the remote w5090
  (`D:/sys_lock.sh remote w5090 run ...`).
- Out of scope: lead-run compression; prose tests outside the touched area;
  the duplicated manifest regen env vars (`WSRSRC_REGEN`, `WS_REGEN_MANIFEST`).

## Open Decision Queue
last-id: 07

(07) [open] [policy] Lead-addressed handler asks: the handler's non-user reports (awaiting-confirmation full-read check, done after revise, blocked) become self-describing asks addressed to the lead, and lead-ticket carries one generic rule — an ask addressed to the user is relayed, an ask addressed to the lead is answered by the lead
- Context: design review block. "The lead only relays" would delete the awaiting-confirmation full-read check, but the handler cannot take it over (it does not hold the user's confirmed intent); 753749802/39481c64f keep that check with the lead.
- Alternative (a): keep the full-read/confirm rule in lead-ticket as an explicit exception to the directive.
- Alternative (c): fold awaiting-confirmation into a user-addressed ask (adds a user round the design avoids).
> Recommend (b) above: the handler says what to check and how to reply, so the procedure stays in the handler and the lead keeps only one generic addressing rule.

## Prior Decisions

- 261105-feat-lead-ticket-review-auto-ack (2026-11-05, 8836292a): "Rejected: auto-confirming every late item (128b/128c), a wsflow default of off, rsrc variants or template conditionals for knob-off text, a global-only key." — bearing: contradiction-candidate
- 261105-feat-lead-ticket-review-auto-ack (2026-11-05, Result 1ace37e49): "The knob-off variant is applied in the `playbook.read` handler after `printPlaybook`, not inside it. `printPlaybook` carries no session key... `lead-ticket` reaches the model only through `playbook.read`." — bearing: contradiction-candidate
- 75374980 (2026-11-15, commit): "Known follow-up: ticketReviewAutoAckVariants (mcp/ticket_review_text.go) still targets the old lead-ticket text; it should move to the handler render." — bearing: supports
- 75374980 (2026-11-15, commit): "Corrections after awaiting-confirmation go back to the handler, never edited by the lead, because the sage stamp digests the ticket body." — bearing: constrains
- 75374980 (2026-11-15, commit): "Lead-side playbooks open as a manual, not a role assignment (user view)." — bearing: supports
- 39481c64 (2026-11-16, commit): "lead-run invokes lead-ticket rather than rendering the handler itself, so the relay/full-read/confirm loop stays defined in one skill" — bearing: constrains
- 261105-feat-whiteboard-knob-harness-and-session-scope (2026-11-05, 1ace37e4): "Each variant pair spans a whole sentence or bullet so the replace-or-append fallback appends a self-standing rule; a test pins every shipped anchor to occur exactly once in the rendered body." — bearing: contradiction-candidate
- 261107-feat-ws-whiteboard-xml-commands-topic-content (2026-11-07, 420c93d5): "the variant lines in whiteboard_text.go are a second shipped surface... Tests that pin the variant text move with it." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | rsrc ticket-handler/lead-ticket/populator/design reviewer, SKILL.md shims, skill-authoring.md, internal/mcp, internal/wsdoc, internal/wsconfig, mirrors and manifests |
| scope.surface | cross-module | spans rsrc playbooks, internal/mcp, internal/wsdoc, internal/wsconfig; no exported Go symbol changes |
| scope.new_public_symbol | no | none; new override pointIds and the handler ask status are additive playbook text |
| scope.new_type_contract | no | none; the ask payload is prose protocol between playbooks, no Go type or signature added |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/ticket_review_text_test.go, whiteboard_tools_test.go#L1176-L1246, tickets_sage_test.go#L123 and #L187 edited or deleted; no new test file planned |
| complexity.reuse_points | confirmed | applyOverrideMarkers and buildOverrideLookup read (internal/mcp/playbook_tools.go#L739-L800, call at #L1020) |
| complexity.side_effect_risk | moderate | the override lookup closure is shared by every playbook render, so knob resolution added there reaches all playbooks |
| risk.correctness | moderate | knob-off text resolved through marker lookup with user-override precedence is new behavior and cannot be tested locally |
| risk.fit | moderate | one slice spans handler protocol redesign, tool texts, mirrors, and manifests across three packages |
| risk.test | moderate | Go tests run only on the remote host, and prose-pinning assertions are deleted so coverage shrinks |
| risk.security_or_contract | moderate | T0 is stated in Decisions without source-line verdict, and tool next_instruction wording changes |

## Phases

### Phase 1: Generic ask channel, knob markers, and stale-reference cleanup

Apply every decision above in one slice: rsrc (ticket-handler, lead-ticket,
populator, design reviewer), skill shims, `skill-authoring.md`, Go
(`internal/mcp` knob removal and marker lookup, whiteboard anchor, tool texts;
`internal/wsdoc` refusal text), test cleanup, mirrors and manifests.

Verification:
- `go test ./internal/mcp/... ./internal/wsrsrc/... ./internal/wsdoc/...`
  from `agents-plugin-tool` on w5090 passes (manifest drift and mirror
  identity tests included).
- `agents-plugin/tests` Python suite passes.
- Acceptance read (not a test): ticket-handler.md states what it does for
  each sage_gate action (`run`, `skip`, `ask`, `check_review_required`,
  `stop_blocked`, `stop_open_decision_queue`); lead-ticket names
  `root_override`; a grep of shipped rsrc and skills finds none of "cheap-tier
  fact population", "design-review gate into", "On: Reviewer Spawn", or
  "### Item classes".
- The Result records the T0 compatibility verdict with source lines for which
  version writes and reads the new override pointIds (an old binary meeting a
  stored user override for an unknown pointId) and the reworded tool texts,
  per `ai-docs/manuals/release-compatibility.md`.
- One throwaway, uncommitted inspection on w5090 (not a test): rendering
  ticket-handler with the knob on and off shows the seed and the off text at
  the marker, and lead-ticket gets no appended trailing paragraphs.
