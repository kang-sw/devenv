---
title: lead-ticket - lead confirms review-raised announced and technical items autonomously
related:
  261105-feat-whiteboard-explicit-item-ref-syntax: motivating case (item (124))
  261030-feat-session-whiteboard-mode: precedent (code-side knob text, catalog entry)
  261105-feat-whiteboard-knob-harness-and-session-scope: shared fact (session-resolver render path)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: e125d260dce1cac0
sage-review-completeness-reviewed: e125d260dce1cac0
---

# lead-ticket - lead confirms review-raised announced and technical items autonomously

## Background

`lead-ticket`'s Open Decision Queue requires a user acknowledgement for every
announced default and technical decision, including those raised late in a
settlement: "Announced or technical items raised later in the same settlement
(a trace knock-on, a new fact-populator gap) need their own acknowledgement"
(`agents-plugin/rsrc/lead-ticket/lead-ticket.md`, `### Item classes`).

At `ready/` promotion this costs a full user round trip after the reviewers
have already passed. In `261105-feat-whiteboard-explicit-item-ref-syntax`,
both Sage reviewers passed with only minor autonomous issues, yet one of them
(the list of fields that parse references) had to become an announced item,
(124), and wait for a separate acknowledgement before the move. The user
wants minor confirmations raised during review to stop stalling promotion
(2026-11-05).

## Decisions

- **Scope: only items raised during review ((128a), user decision
  2026-11-05).** An announced default or technical decision that arises from
  fact population (a `decision_gaps:` item) or from a Sage reviewer issue
  marked `autonomous` (see (140)) may be confirmed by the lead on its own judgment, without a user
  acknowledgement round. Items in the initial draft and trace knock-ons
  raised outside review keep the current acknowledgement rule.
  Rejected: (128b) every announced or technical item raised later in a
  settlement, and (128c) every announced or technical item including the
  initial draft (the user would never see some decisions before they
  persist).
- **Reviewer `missing` issues are still asked ((140), user decision
  2026-11-05).** Only reviewer issues marked `autonomous` and fact-populator
  `decision_gaps:` items qualify for auto-confirmation; a reviewer issue
  marked `missing` gets a user round whatever class the lead gives it, since
  a `missing` issue is what blocks a promotion and the user should see what
  cleared the block. e3b0aac88 removed the rule that classed every `missing`
  issue as a policy question, so the new text states the `missing`
  exclusion as an auto-confirmation limit, not a class, and must not
  reintroduce the phrase "is always a policy" that `playbook_tools_test.go`
  forbids. Rejected: any review-raised item the lead classes as announced
  or technical, `missing` included (a lead could clear a block without the
  user seeing it).
- **Trace knock-ons keep the acknowledgement rule ((148)).** A trace
  knock-on of a review-raised item keeps the acknowledgement rule unless the
  reviewer issue itself named it, and `### Item classes` says so. Rejected:
  letting every knock-on of an auto-confirmed item auto-confirm too (the
  lead could chain unreviewed decisions behind one reviewer issue).
- **Policy questions and reversals are still asked ((125)).** A review-raised
  item that is a policy question, or that reverses an already-confirmed
  decision, goes to the user as today; the workflow manual's
  `### Asking the user` classes a reversal as a policy question even when a
  reason can be cited.
- **Auto-confirmed items stay visible ((127)).** The lead lists them in the
  round's settlement report line, for example `(124) auto-confirmed`, and an
  objection from the user reopens the item under its existing ID like any
  other correction. The report may first reach the user in the turn that
  stamps and moves the ticket to `ready/`; an objection then reopens the
  item on the `ready/` ticket, like any later correction today. Rejected:
  confirming silently (a wrong lead call would have no place for the user
  to catch it).
- **A config knob, default on in both packages ((129), (130)).** The
  behavior is gated by a built-in layered key `ticket_review_auto_ack`,
  default on in ws and in wsflow. Off restores the current rule. Rejected:
  an unconditional change (the user wants a lever), and a wsflow default of
  off (user decision: the friction it removes applies to wsflow users too).
  Rejected name: `sage_review_auto_ack`, because the rule also covers
  fact-populator gaps, not only Sage issues. The name follows the
  unprefixed `sage_review` / `sage_review_design` family in
  `agents-plugin-tool/internal/wsconfig/scope.go`. ws and wsflow run the
  same binary, so "default on in both packages" is one entry in
  `builtinConfigDefaults` (`internal/mcp/server.go`) plus
  `NoAgentVisible: true` on the `config_registry.go` tuning entry, so
  wsflow's agentless `config.list` and `config.tune` expose it; the package
  `config-manifest.json` files hold only `claude.` adapter keys and do not
  change.
- **Mirrors ((126)).** The `lead-ticket` playbook change lands in the ws,
  wsflow, and Pi copies with their manifest hashes, per
  `ai-docs/manuals/wsflow-mirroring.md`.
- **Knob-off text is applied code-side at render ((131)).** The shipped
  rsrc text states the default-on rule; when the knob resolves off, the
  playbook render substitutes the current acknowledgement rule, the
  mechanism `internal/mcp/whiteboard_text.go` already uses for
  `workflow.whiteboard_enabled`. The rsrc mirrors stay byte-identical.
  Rejected: two rsrc variants, or a template conditional in the rsrc text
  (two sources of text, or a new render mechanism).
- **Ordinary layered scopes ((132)).** The key resolves through session,
  project, repo, and global scopes like `sage_review`. It is review
  posture, which `sage_review` already lets a project commit. Rejected: a
  global-only key like `workflow.whiteboard_enabled`; that knob's
  global-only premise (the manual renders without a repository root) does
  not hold, since `handleWorkflowManual` already resolves `workflow.lang`
  through the session resolver (see
  `261105-feat-whiteboard-knob-harness-and-session-scope`). The render
  reads the key through `s.sessionResolver(key, builtinConfigDefaults())`
  (`internal/mcp/server.go`), which carries the session layer and anchors
  the repo root; the existing helper pattern
  `NewResolver(configOpts, builtinConfigDefaults(), nil, nil)` would
  silently ignore session, project, and repo writes, because both
  `printPlaybook` callers pass `wsconfig.Options{}`.
- **Tuning catalog entry ((133)).** The key gets a `lead-tune` catalog
  entry, so `config.list` shows it and `config.tune` writes it, following
  the whiteboard knob precedent ((106) in
  `261030-feat-session-whiteboard-mode`).

## Constraints

- Manuals: `ai-docs/manuals/skill-authoring.md` (playbook edit),
  `ai-docs/manuals/wsflow-mirroring.md`,
  `ai-docs/manuals/shipped-surface-boundary.md`, `ai-docs/manuals/ws-mcp.md`
  (config key and render path).
- The Approval Protocol classes this as a convention change ("Ask first");
  the user asked for it directly (2026-11-05).
- Out of scope ((141), user decision 2026-11-05): recording auto-confirmed
  items on the session whiteboard. The whiteboard `answer` requires an
  `answer_quote` of the user's verbatim words, so it needs a whiteboard
  store contract change (for example a lead-confirmed status); captured as
  `261105-feat-whiteboard-lead-confirmed-status`. With the whiteboard knob
  on, an auto-confirmed item stays `open` on the board until then; the
  settlement report line ((127)) keeps it visible.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 261007-feat-odq-technical-decision-class (2026-10-07, commit e05c7b23): "Supersedes the two-class decision ... all other rules of that ticket (silence is not consent, objection -> policy under the same ID, reversals always policy, reviewer `missing` always policy) now apply to technical items" — bearing: constrains, except the reviewer-`missing` always-policy rule, which e3b0aac88 (2026-10-23) removed (a reviewer issue is now classed like any other item; `playbook_tools_test.go` forbids the phrase "is always a policy")
- 260925-feat-odq-announced-defaults-and-policy-questions (2026-09-25, Decisions): "Announced defaults are confirmed by one explicit owner acknowledgement covering the whole group, which may arrive in the same turn as the policy answers." — bearing: superseded in part (review-raised announced and technical items, knob on, per (128a))
- 261023-feat-ws-shared-question-classes-and-ids (2026-10-23, Result c6af23a7): "`lead-ticket` `### Item classes` points at the manual and keeps the documentation-placement, open-round-hold, and reviewer-`missing` rules." — bearing: constrains (the reviewer-`missing` rule was later replaced by e3b0aac88)
- f98d8e83 (2026-11-04, commit): "Knob-on text is applied code-side in printPlaybook (the prefer_subagent precedent) with replace-or-append fallback, so rsrc mirrors stay byte-identical and a prompt override that drops the anchor text loses nothing." — bearing: supports
- 261030-feat-session-whiteboard-mode (2026-10-30, Decisions): "listed in the lead-tune tuning catalog (technical decision (106))." — bearing: supports
- 261105-feat-whiteboard-knob-harness-and-session-scope (2026-11-05, Decisions): "Session scope (user decision 2026-11-05). The knob can be set for a single session." — bearing: supports
- dd4b01b5 (2026-09-26, commit): "The golden forbidden-phrase check runs on the whitespace-normalized body too, because the old promotion-step reference wrapped 'final' and 'confirmation' across lines and evaded a raw grep." — bearing: constrains
- d5682a35 (2026-09-26, commit): "The pi rsrc has no regen entrypoint (bump-ws-version.sh sync_tree is the only syncer), so lead-ticket.md and manifest.json were copied and checked with diff -r." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-ticket/lead-ticket.md plus wsflow and Pi copies and rsrc manifests, agents-plugin-tool/internal/wsconfig/scope.go, agents-plugin-tool/internal/mcp/server.go builtinConfigDefaults, agents-plugin-tool/internal/mcp/config_registry.go, a render-time variant beside agents-plugin-tool/internal/mcp/whiteboard_text.go |
| scope.surface | public-interface | new config key visible through config.list and writable through config.tune, and default-on shipped lead-ticket playbook text |
| scope.new_public_symbol | yes | config key ticket_review_auto_ack and its wsconfig item constant |
| scope.new_type_contract | no | none; a boolean on/off layered key like existing on/off knobs |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/playbook_tools_test.go TestPlaybookPrintGoldenLeadTicket, agents-plugin-tool/internal/mcp/whiteboard_tools_test.go, agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go, agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go |
| complexity.reuse_points | confirmed | applyWhiteboardVariants replace-or-append mechanism agents-plugin-tool/internal/mcp/whiteboard_text.go#L72 called from agents-plugin-tool/internal/mcp/playbook_tools.go#L1041; it receives configOpts (wsconfig.Options{} from both printPlaybook callers, server.go and workflow_manual.go), so the key is read through s.sessionResolver(key, builtinConfigDefaults()) at that call site; the Claude preload (preload.ts) and the Pi preload (lead-skills.ts) both reach it through playbook.read |
| complexity.side_effect_risk | moderate | default-on changes the rendered lead-ticket acknowledgement rule for every downstream ws and wsflow session |
| risk.correctness | moderate | knob-off text must reproduce the current rule byte-exactly and the variant anchor must match the shipped rsrc text |
| risk.fit | moderate | the rule must stay coherent with the workflow manual Asking the user reversal and policy rules and with the reviewer missing rule |
| risk.test | moderate | golden pins and forbidden-phrase checks on lead-ticket text plus mirror drift tests must be updated without weakening them |
| risk.security_or_contract | moderate | new layered config key contract and an observable workflow behavior change shipped default-on |

## Phases

### Phase 1: Review-raised auto-confirmation

Add the `ticket_review_auto_ack` key (one `builtinConfigDefaults` entry,
default on) and its tuning catalog entry with `NoAgentVisible: true`, so
`config.list` shows it and `config.tune` writes it in ws and wsflow. Rewrite
every `lead-ticket` site that states the acknowledgement rule for
review-raised announced and technical items:

- `## Open Decision Queue` intro (the settle point and "the acknowledgement
  already showed every item");
- `### Item classes` (the "raised later in the same settlement" bullet;
  this site also carries the (140) reviewer-`missing` limit);
- `## Ground: fact population` step 3 ("never resolve one from the
  populator's evidence alone");
- `## Stops` ("an announced default or technical decision not yet
  acknowledged");
- `### Response format` (the per-round settlement report gains the
  `(nn) auto-confirmed` form).

The knob-off variant carries one replace-or-append pair per changed anchor.
Update the ws, wsflow, and Pi copies and their manifests.

Verification:

- knob on: the rendered `lead-ticket` playbook lets the lead confirm a
  review-raised announced or technical item and report it as
  auto-confirmed, while policy, reversal, and reviewer-`missing` items are
  still asked, and the rendered body, raw and whitespace-normalized (the
  dd4b01b5 golden check), does not contain "is always a policy";
- knob off: the whole rendered `lead-ticket` body equals today's rendering
  of the current rsrc text, not only the `### Item classes` bullet;
- a session-, project-, or repo-scope `off` reaches the render;
- `config.list` shows the key with default on, and `config.tune` writes it,
  in ws and in wsflow (agentless);
- wsflow mirror and Pi tree drift tests pass.
