---
title: Fill the gaps left by the lead-ticket / ticket-handler split
related:
  261105-feat-lead-ticket-review-auto-ack: knob mechanism this ticket replaces
  261105-feat-whiteboard-knob-harness-and-session-scope: whiteboard knob injection this ticket re-anchors
  261116-feat-orchestration-profile-foundation: receives G3 and decision (07) into its Phase 2; its Decision (53) knob implementation is replaced here
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 2c1d66eaf4952306
sage-review-completeness-reviewed: 2c1d66eaf4952306
completed: 2026-11-17
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
the gaps below (evidence by inspection; no `go test` was run). After the draft,
upstream landed d25bdfcc4 (261116-feat-orchestration-profile-foundation
Decision (53), Phase 1) and 2da06e481; their effect on each gap is stated.

- G1 (behavior-breaking; behaviorally closed by d25bdfcc4): the
  `ticket_review_auto_ack` knob was applied only on `playbook.read` and only to
  `lead-ticket`, so with the knob off `replaceOrAppend` appended off-texts to
  the end of lead-ticket and the handler, rendered through `playbook.render`,
  always auto-confirmed. d25bdfcc4 re-keyed `ticketReviewAutoAckVariants` per
  playbook (lead-ticket, ticket-handler) and applies it on `playbook.read` and
  in `renderPlaybookWithPlan`. What remains: the mechanism is still sentence
  substitution through `replaceOrAppend`, which drifts silently on the next
  edit (the failure this gap came from), and a verbatim pin test
  (`ticket_review_text_test.go`) guards it, which contradicts the no-prose-test
  directive below.
- G2 (failing test; behaviorally closed by 2da06e481): the whiteboard knob
  anchor `lineTicketQueueStateEnd` now targets the current lead-ticket text
  (`\n\nTrace before writing — any change`) and the test passes. What remains:
  the anchor is again a prose substring that desyncs silently, and
  `whiteboard_tools_test.go` pins copied lead-ticket sentences.
- G3 (behavior gap; moved to 261116-feat-orchestration-profile-foundation
  Phase 2 by user (09)): the handler has no path for `tickets.sage_gate`
  actions `ask`, `check_review_required`, and `stop_blocked`, and no channel to
  ask the lead or user. Phase 2 already folds the handler's report into the
  `sublead-protocol` report block, which is where the ask channel belongs.
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
- G7 (closed by d25bdfcc4): `ticket_review_text.go` claimed a test pins every
  on-text; the pin test now makes the header comment true. It goes away with
  the variants; the header comment must then describe only what remains
  (`ticketReviewAutoAckEnabled`).

## Decisions

- **Procedure knowledge lives in the subagent playbook (user directive,
  2026-11-16).** A lead playbook does not carry "when the handler reports X,
  do Y" rules. The handler playbook carries "in situation X, ask the lead in
  this form". The lead side only relays.
  Rejected: per-status response rules in lead-ticket (the shape the split left
  behind).
- **(07) Lead-addressed handler asks are self-describing (user (07)=b).** An
  ask from the handler that the lead, not the user, must answer says what to
  check and how to reply; lead-ticket keeps one generic rule for it (an ask
  addressed to the user is relayed, an ask addressed to the lead is answered
  by the lead), and the procedure stays in the handler playbook. Forwarded to
  261116-feat-orchestration-profile-foundation Phase 2 as an input to its G3
  work (see (09)); this ticket does not implement it.
  Rejected: keeping the awaiting-confirmation full-read rule in lead-ticket as
  an exception to the directive (a); folding it into a user-addressed ask
  (c, adds a user round).
- **(09) G3 moves to 261116-feat-orchestration-profile-foundation Phase 2
  (user (09)=c).** The handler's handling of the sage_gate `ask`,
  `check_review_required`, and `stop_blocked` actions, the generic
  handler-to-lead ask channel, and the fold of the `decisions` status
  (including the ticket-handler `## Lead replies` section that parses lead
  answers and every lead-run reference to the `decisions` status) are designed there through the
  `sublead-protocol` report, carrying (07) as an input. This ticket keeps the
  knob markers, G2, G4, G5, G6, and test cleanup. G4 still makes the sage_gate
  texts for those actions caller-neutral and correct (see G4); the handling
  procedure is not written here.
  Rejected: designing a handler ask status here (the protocol fold would
  reshape it and leave two report shapes in the same files).
- **(08) `ticket_review_auto_ack` is kept (user (02)) and reimplemented with
  override-point markers (user (08)=a).** This replaces the
  sentence-substitution implementation that d25bdfcc4 landed under
  261116-feat-orchestration-profile-foundation Decision (53); (53)'s
  retention of the knob stands. Wrap the handler's auto-confirm rule text in
  `<!-- ws:override:<pointId> -->` blocks (seed = knob-on text) and have the
  override lookup (`buildOverrideLookup` in `internal/mcp/playbook_tools.go`)
  return the knob-off text when the resolved knob is off. Markers go in
  ticket-handler.md. The three lead-ticket on-texts that d25bdfcc4 re-keyed
  (the "plus items the handler auto-confirms" mentions) are reworded so they
  hold with the knob on or off (the handler's `auto_confirmed:` relay is empty
  when the knob is off), leaving lead-ticket with no knob-dependent text apart
  from the whiteboard marker in G2. The markers are resolved in the shared
  render core, so `playbook.read` and `playbook.render` of the handler both
  honor the knob resolved from the caller's session key. A user prompt
  override on the same pointId takes precedence over the knob: the knob-off
  text is a fallback consulted only after every bucket in
  `harnessBucketReadOrder` found no stored override (the lookup stops at the
  first bucket hit), and the knob is resolved with config defaults, as
  `ticketReviewAutoAckEnabled` does, not the prompt-override defaults the
  lookup's resolver is seeded with. `applyOverrideMarkers` calls the lookup
  once per bucket in `harnessBucketReadOrder` (`playbook_tools.go`
  ~L862-L866), so the knob-off text must be a separate fallback after all
  buckets miss (or answered only for the final bucket), never a per-bucket
  answer that would shadow a stored override in a later bucket. The keyless
  branch (~L762-L767) builds a builtins-only closure; resolve the knob lazily,
  only for marker points that need it, so other playbook renders gain no new
  failure path. The implementer picks the exact shape. The off-texts approved under (53) remain
  the knob-off wording (the implementer names the pointIds and matches each
  seed to the text present when the phase runs); change one only where a G4 or
  G5 edit alters the sentence it replaces. Delete
  `ticketReviewAutoAckVariants` and `applyTicketReviewAutoAckVariant`
  (`ticket_review_text.go`), their `playbook.read` call (`server.go`), the
  `renderPlaybookWithPlan` block (`playbook_tools.go`), and the verbatim pin
  test in `ticket_review_text_test.go`; keep `ticketReviewAutoAckEnabled`.
  Rewrite that file's two behavior tests
  (`TestPlaybookReadLeadTicketReviewAutoAckFollowsKnob`,
  `TestTicketHandlerReviewAutoAckFollowsKnobOnRenderAndRead`) as structural
  checks only: the knob-off render differs from on, the override point
  resolves, scopes and other playbooks behave as before; no copied sentences.
  Update the knob description text (server.go config description,
  `wsconfig/scope.go` comment) to name the handler. This reverses
  261105-feat-lead-ticket-review-auto-ack's rejection of "rsrc variants or
  template conditionals for knob-off text" (user (06), 2026-11-16): that
  choice only followed the code-side substitution precedent of the whiteboard
  knob, and the split showed it desyncs silently. The user saw (53)'s
  rationale before choosing.
  Rejected: retiring the knob (deletes functionality users may rely on);
  keeping (53)'s re-keyed substitution and pin test (the same silent desync
  recurs on the next edit, and only a prose-anchor test guards it, against the
  no-prose-test directive); a new `{{.Var}}` placeholder (needs
  session-config plumbing in `buildPlaybookVars`, more than markers).
  Sequencing: Phase 2 of the foundation ticket reshapes ticket-handler.md and
  lead-ticket.md; whichever lands second keeps the marker blocks and re-reads
  them against the then-current text.
- **G2**: replace the whiteboard knob's lead-ticket sentence anchor
  (`lineTicketQueueStateEnd`, as re-anchored by 2da06e481) with an
  override-point marker at the current anchor site in lead-ticket's queue
  Section list, using the same fallback-after-all-buckets placement as the
  auto-ack knob (same reason as (08): a substring anchor desyncs silently).
  The whiteboard knob resolves per harness (`whiteboardEnabled` ->
  `bucketedKnobOn` -> `resolveHarnessBucketed`, `whiteboard_tools.go`
  ~L842-L868) and must keep that, unlike auto-ack's plain `resolver.Get`.
  Polarity is reversed: the seed is an empty extension slot, and knob-on
  injects the bullet after `withNamespace`. "Same fallback" means the
  placement, not the knob resolution. The bullet keeps covering every queue the lead puts to
  the user, its own drafting queue included, not only handler asks. The
  lead-discuss and workflow-manual whiteboard variants are out of scope.
  Delete the lead-ticket sentence assertions in `whiteboard_tools_test.go`
  (the pins on "Trace before writing" and "Section — `## Open Decision
  Queue`"); keep or add only structural checks (marker present, knob on/off
  renders differ).
  Rejected: re-anchoring on another heading or sentence substring (same
  silent-desync shape).
- **G4**: reword the affected tool texts caller-neutral and correct for the
  handler flow (gate before move; "settle the queue" without "with the user";
  "commit with the caller's next commit"; drop every dangling
  "On: Reviewer Spawn", in the sage_gate `run` text as well as `stop_blocked`;
  replace hard-coded `ws/` tool names in these texts, including the stamp
  instructions in `tickets_mutate.go`, with the session's namespace). This
  includes the sage_gate next_instruction texts for `ask`,
  `check_review_required`, and `stop_blocked`: they stay accurate and name no
  caller role or procedure; the handler's handling of those actions is (09).
  Delete test assertions that pin those sentences.
- **G5**: lead-ticket names `root_override: <worktree>` for the occupied-root
  and `revise` renders.
- **G6**: update both lead-ticket `SKILL.md` descriptions (and regenerate the
  skills manifest), say "the caller" in the populator and design reviewer, and
  change `skill-authoring.md` Layouts so a lead skill opens with what the
  playbook covers, not an Identity line (user (04); worker, reviewer, and
  delegate playbooks keep their identity lines).
- **Prose tests (user directive)**: add no test that pins English prose; in
  the area this ticket touches, delete assertions that pin prose sentences or
  removed headings (the verbatim auto-ack pin test and the sentence checks in
  the two auto-ack behavior tests, the whiteboard lead-ticket sentence
  assertions, the G4 sentence pins) and the variants' header comment.
  Structural checks stay: manifest hashes, mirror byte-identity, variable
  substitution, code-parsed headings, marker presence, on/off render
  difference. Prose tests elsewhere are out of scope.
- **Compatibility: T1 (display-degraded).** No MCP tool or argument, config
  key, or store shape changes; tool response wording changes; new override
  pointIds are additive. Each connection reads its own rsrc files
  (`agents-plugin-tool/internal/mcp/process_env.go` ~L226-L234,
  `EnvRsrcRoot`), so a shared daemon and rsrc files can be at different
  versions. New daemon + old files: no markers, knob-off is ignored, the
  auto-confirm text renders. Old daemon (d25bdfcc4 variants) + new files: the
  variants miss the reworded sentences and `replaceOrAppend` appends the
  off-texts at the end of lead-ticket. No stored data changes; reopening with
  matched versions renders correctly. T1 permits a patch.

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
- Out of scope: lead-run compression; the handler ask channel, the `decisions`
  fold, and handler handling of sage_gate `ask` / `check_review_required` /
  `stop_blocked` (261116-feat-orchestration-profile-foundation Phase 2);
  prose tests outside the touched area; the duplicated manifest regen env vars
  (`WSRSRC_REGEN`, `WS_REGEN_MANIFEST`).

## Prior Decisions

- 261105-feat-lead-ticket-review-auto-ack (2026-11-05, 8836292a): "Rejected: auto-confirming every late item (128b/128c), a wsflow default of off, rsrc variants or template conditionals for knob-off text, a global-only key." — bearing: contradiction-candidate
- 261105-feat-lead-ticket-review-auto-ack (2026-11-05, Result 1ace37e49): "The knob-off variant is applied in the `playbook.read` handler after `printPlaybook`, not inside it. `printPlaybook` carries no session key... `lead-ticket` reaches the model only through `playbook.read`." — bearing: contradiction-candidate (premise gone: the handler renders through `playbook.render`; d25bdfcc4 applies the variants on both paths, and this ticket replaces the variants with markers resolved in the shared render core)
- 75374980 (2026-11-15, commit): "Known follow-up: ticketReviewAutoAckVariants (mcp/ticket_review_text.go) still targets the old lead-ticket text; it should move to the handler render." — bearing: supports (the follow-up was done by d25bdfcc4; this ticket replaces its mechanism)
- 261116-feat-orchestration-profile-foundation (2026-11-16, Decision (53), Result d25bdfcc4): "Auto-ack fix. Re-anchor `ticketReviewAutoAckVariants` on the current text, apply the `ticket-handler` variants in the render path, and add the pin test... Rejected: retiring the `ticket_review_auto_ack` knob." — bearing: contradiction-candidate (this ticket keeps the knob but replaces the substitution and pin test with override markers by user (08)=a; (53) weighed only re-anchor versus retire, not markers)
- 261116-feat-orchestration-profile-foundation (2026-11-16, Result 2da06e481): "the knob-on `lead-ticket` queue step is re-anchored on the rewritten queue `Section —` list. Both came from 6d1e7e8c6. No rsrc prose changed." — bearing: contradiction-candidate (closes G2 behaviorally; this ticket replaces the re-anchored substring with a marker)
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
| scope.new_public_symbol | no | none; new override pointIds are additive playbook text |
| scope.new_type_contract | no | none; no Go type or signature added, no new handler report shape (G3 is in the foundation ticket's Phase 2) |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/ticket_review_text_test.go (verbatim pin test deleted, two behavior tests reduced to structural checks), whiteboard_tools_test.go (lead-ticket sentence pins at ~#L1243-L1246 replaced by structural checks), tickets_sage_test.go#L123 and #L187 edited or deleted; no new test file planned |
| complexity.reuse_points | confirmed | applyOverrideMarkers in renderPlaybookBodyForCaller (internal/mcp/playbook_tools.go#L1037), buildOverrideLookup (#L760-L783), ticketReviewAutoAckEnabled (internal/mcp/ticket_review_text.go#L78-L88) |
| complexity.side_effect_risk | moderate | the override lookup closure is shared by every playbook render, so knob resolution added there reaches all playbooks |
| risk.correctness | moderate | knob-off text resolved through marker lookup with user-override precedence is new behavior and cannot be tested locally |
| risk.fit | moderate | one slice spans the marker mechanism, tool texts, mirrors, and manifests across three packages, and shares ticket-handler.md and lead-ticket.md with the foundation ticket's Phase 2 |
| risk.test | moderate | Go tests run only on the remote host, and prose-pinning assertions are deleted so coverage shrinks |
| risk.security_or_contract | moderate | T1 is stated in Decisions with source lines for the version skew, the Result must still record the verdict, and tool next_instruction wording changes |

Code locations (HEAD 0c6cd585c):

- `internal/mcp/server.go`: auto-ack knob description #L3049-L3051;
  `playbook.read` auto-ack call #L2265-L2269 (delete); `playbook.render`
  starts #L2272, `buildOverrideLookup` call #L2317, `renderPlaybookWithPlan`
  call #L2349; sage_gate next_instruction texts #L3866-L3878 (`stop_blocked`
  #L3866, Route Facts #L3868, queue #L3872, `ask` #L3874, `run` #L3876,
  `check_review_required` #L3878); posture-note texts #L3934, #L3936.
- `internal/mcp/whiteboard_text.go`: lead-ticket anchor
  `lineTicketQueueStateEnd` #L31-L33, bullet constant #L34, `replaceOrAppend`
  #L62-L68, lead-ticket case #L116-L122.
- `internal/mcp/ticket_review_text.go`: variants #L26-L63, apply #L65-L76,
  `ticketReviewAutoAckEnabled` #L78-L88.
- `internal/mcp/playbook_tools.go`: `buildOverrideLookup` #L760-L783,
  `applyOverrideMarkers` in `renderPlaybookBodyForCaller` #L1037, auto-ack
  block in `renderPlaybookWithPlan` #L1192-L1199 (delete).
- `internal/mcp/tickets_sage_test.go` #L123, #L187;
  `internal/wsdoc/tickets_mutate.go` #L392, #L700-L703 (stamp instructions,
  refusal text); `internal/wsconfig/scope.go` #L79-L86 (knob comment).

## Phases

### Phase 1: Knob markers, caller-neutral tool texts, and stale-reference cleanup

Apply every decision above in one slice: rsrc (ticket-handler markers,
lead-ticket knob-independent wording, whiteboard relay marker and
`root_override`, populator, design reviewer), skill shims, `skill-authoring.md`,
Go (`internal/mcp` variant and render-block removal and marker lookup,
whiteboard anchor, tool texts; `internal/wsdoc` refusal text), test cleanup,
mirrors and manifests.

Verification:
- `go test ./internal/mcp/... ./internal/wsrsrc/... ./internal/wsdoc/...`
  from `agents-plugin-tool` on w5090 passes (manifest drift and mirror
  identity tests included).
- `agents-plugin/tests` Python suite passes.
- Acceptance read (not a test): lead-ticket names `root_override`; a grep of
  shipped rsrc and skills finds none of "cheap-tier fact population",
  "design-review gate into", "On: Reviewer Spawn", or "### Item classes"
  (the last only if that heading no longer exists); a grep of
  `agents-plugin-tool/internal/mcp/*.go` non-test code finds no "On: Reviewer
  Spawn", and the `tickets_mutate.go` stamp instructions hard-code no `ws/`
  tool names (G4); `skill-authoring.md` Layouts shows the lead-skill "opens
  with what the playbook covers" change (G6); no test file in the touched
  area asserts copied playbook sentences.
- The Result records the T1 compatibility verdict with source lines for which
  version writes and reads the new override pointIds (an old binary meeting a
  stored user override for an unknown pointId), the daemon/rsrc version
  skew cited in the Compatibility decision (`process_env.go` `EnvRsrcRoot`),
  and the reworded tool texts,
  per `ai-docs/manuals/release-compatibility.md`.
- One throwaway, uncommitted inspection on w5090 (not a test): rendering
  ticket-handler and lead-ticket with the knob on and off, through both
  `playbook.read` and `playbook.render`, shows the seed and the off text at
  each marker and no appended trailing paragraphs.

### Result (420ed45e9) - 2026-11-17

Implemented G1/G2 marker resolution, G4 caller-neutral tool guidance, G5 explicit
worktree targeting, and G6 ownership/layout cleanup. G3 and the handler report
protocol remain with the foundation ticket as decided. The handler points are
`TicketHandlerPersistConfirmation` and `TicketHandlerReviewAutoAck`; the lead's
empty whiteboard extension is `TicketQueueWhiteboard`. Knob fallbacks answer only
in the final `all` bucket, after stored prompt overrides miss, and resolve lazily
with config defaults. Auto-ack retains plain layered resolution; whiteboard
retains harness buckets. Sentence substitutions, their append fallback, and the
affected prose pins are gone. Canonical resource/skills manifests were regenerated
on w5090 and fetched back; wsflow/Pi resource mirrors match canonical bytes.

Verification:
- w5090, job `67b0cc19b3a54328bf745de95fdf85ae`:
  `go test ./internal/mcp/... ./internal/wsrsrc/... ./internal/wsdoc/...` passed
  (mcp 230.154s; wsrsrc/wsdoc cached). This is the closing full Go check.
  Earlier runs exposed stale wsdoc prose checks, then a separate incorrect
  zero-counter assumption introduced by their structural cleanup; both were
  fixed in tests, not by changing correct production behavior.
- Python: 179 tests passed on w5090 (3 platform/process skips), excluding the
  pre-existing daemon-workflow bundle suite and the Git-dependent neutrality
  suite. The latter ran locally: 7 passed. wsflow package suite locally:
  22 tests passed (1 platform skip). Remote source sync deliberately carries no
  `.git`, so the Git-dependent suite's initial remote setup error was an
  environment limitation, not a product failure.
- Throwaway w5090 inspection, job `ddda238721944d1e872ca36796ae194f`:
  both playbooks, both `playbook.read`/`playbook.render`, both knob settings.
  Handler seed/off rules resolve in place; the lead queue bullet appears only
  with whiteboard on; all eight outputs have zero remaining markers and no
  appended trailing knob paragraphs. Inspection was uncommitted and removed.
- Acceptance read: `root_override` appears for revise and occupied-root
  rendering; the forbidden stale phrases are absent from canonical rsrc and
  skills and `On: Reviewer Spawn` is absent from MCP Go code. The wsdoc stamp
  instructions use the move environment's namespace. Lead skill Layouts opens
  with coverage rather than Identity. `git diff --check` passed.
- Independent lite R1 found lost blocked-stage selection coverage. R2 was
  fixes-only and accepted the restored identifier-based stage tests plus the
  explicit decision not to introduce a recovery-decision abstraction solely
  to test English prose. Fresh-versus-same wording has intentionally reduced
  assertion coverage under the no-prose-pin directive; routing logic is
  unchanged. No unresolved review finding remains.

Compatibility: **T1, patch permitted**. At both d25bdfcc4 and this result,
`internal/mcp/config_registry.go:593-594` accepts dynamic `prompt.<pointId>`
keys; `server.go:1194-1200` writes their existing bucketed string values through
the resolver, and `internal/wsconfig/resolver.go:204-206,331-346` updates the
existing override map without discarding other pointIds. The old lookup
(`d25bdfcc4:internal/mcp/playbook_tools.go:760-783`) reads only marker points
present in the loaded text; a stored override for an unknown point is retained
but unused with old text. The new lookup (`playbook_tools.go:760-819`) and marker
pass (`:838-929`) read those same values; reopening matched new code/text restores
the override, without a data migration. Connection-specific resource selection
is `internal/mcp/process_env.go:222-234` (`EnvRsrcRoot`), so daemon/text skew
remains possible: new daemon + old text has no knob markers; old daemon + new
text can append old knob-off paragraphs after its sentence anchors miss
(`d25bdfcc4:ticket_review_text.go:26-76`). Neither path rewrites stored prompt
values. Reopening matched versions fixes rendering. Tool response changes are
wording only, with session namespace substitution at `server.go:2144,2168` and
move-warning namespace selection at `internal/wsdoc/tickets_mutate.go:691-720`;
no tool, argument, config key, or storage shape changed.

Forward: The pre-existing `test_daemon_workflow_bundle` Python suite was excluded
rather than investigated. At base `17a588f28`, its archive-path test fails for
`back\\slash` on Windows and the following concurrent materialization case
exceeded a 90-second cap; the initial full Windows suite also exceeded 2100
seconds. This is one deferred packaging/platform verification issue, not a
split-gap implementation failure.

Workflow substitutions: unavailable `workflow_manual`/todo/convention tools were
replaced by AGENTS, bundled manuals/conventions, and the route verdict. Branch
creation used plain Git; prompt floors/review wrappers used `playbook.read`
rather than render to keep local temporary artifacts under
`D:/Temp/split-gaps/`; delegates used Pi spawn/send. No local Go tests, merges,
pushes, co-author trailers, or implementer markings were used. The final
acceptance read corrected the knob description's remaining lead-ticket wording
to ticket-handler read/render after the closing check; this was text-only.
