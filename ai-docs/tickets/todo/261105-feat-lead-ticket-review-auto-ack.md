---
title: lead-ticket - lead confirms review-raised announced and technical items autonomously
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
  may be confirmed by the lead on its own judgment, without a user
  acknowledgement round. Items in the initial draft and trace knock-ons
  raised outside review keep the current acknowledgement rule.
  Rejected: (128b) every announced or technical item raised later in a
  settlement, and (128c) every announced or technical item including the
  initial draft (the user would never see some decisions before they
  persist).
- **Policy questions and reversals are still asked ((125)).** A review-raised
  item that is a policy question, or that reverses an already-confirmed
  decision, goes to the user as today; the workflow manual's
  `### Asking the user` classes a reversal as a policy question even when a
  reason can be cited.
- **Auto-confirmed items stay visible ((127)).** The lead lists them in the
  round's settlement report line, for example `(124) auto-confirmed`, and an
  objection from the user reopens the item under its existing ID like any
  other correction. Rejected: confirming silently (a wrong lead call would
  have no place for the user to catch it).
- **A config knob, default on in both packages ((129), (130)).** The
  behavior is gated by a built-in layered key `ticket_review_auto_ack`,
  default on in ws and in wsflow. Off restores the current rule. Rejected:
  an unconditional change (the user wants a lever), and a wsflow default of
  off (user decision: the friction it removes applies to wsflow users too).
  Rejected name: `sage_review_auto_ack`, because the rule also covers
  fact-populator gaps, not only Sage issues. The name follows the
  unprefixed `sage_review` / `sage_review_design` family in
  `agents-plugin-tool/internal/wsconfig/scope.go`.
- **Mirrors ((126)).** The `lead-ticket` playbook change lands in the ws,
  wsflow, and Pi copies with their manifest hashes, per
  `ai-docs/manuals/wsflow-mirroring.md`.

## Open Decision Queue

Last ID: (133)

(131) [open] [technical] The knob-off text is applied code-side at playbook render (the `workflow.prefer_subagent` and `workflow.whiteboard_enabled` precedent in `internal/mcp/whiteboard_text.go`), so the shipped rsrc text states the default-on rule and the rsrc mirrors stay byte-identical - over two rsrc variants or a template conditional - one source of text and the mechanism already exists.
(132) [open] [technical] The key resolves through the ordinary layered scopes (session, project, repo, global) like `sage_review` - over a global-only key like `workflow.whiteboard_enabled` - it is review posture, which `sage_review` already lets a project commit; whiteboard was global-only only because the manual renders without a repository root, which `lead-ticket` renders do not.
(133) [open] [announced] The key gets a `lead-tune` catalog entry - the whiteboard knob precedent ((106) in `261030-feat-session-whiteboard-mode`).

## Constraints

- Manuals: `ai-docs/manuals/skill-authoring.md` (playbook edit),
  `ai-docs/manuals/wsflow-mirroring.md`,
  `ai-docs/manuals/shipped-surface-boundary.md`, `ai-docs/manuals/ws-mcp.md`
  (config key and render path).
- The Approval Protocol classes this as a convention change ("Ask first");
  the user asked for it directly (2026-11-05).

## Phases

### Phase 1: Review-raised auto-confirmation

Add the `ticket_review_auto_ack` key (default on, both packages, listed by
`config.list`), rewrite `lead-ticket`'s `### Item classes` acknowledgement
rule for review-raised announced and technical items, and add the
auto-confirmed line to the settlement report format. Update the ws, wsflow,
and Pi copies and their manifests.

Verification:

- knob on: the rendered `lead-ticket` playbook lets the lead confirm a
  review-raised announced or technical item and report it as
  auto-confirmed, while policy and reversal items are still asked;
- knob off: the rendered text matches the current acknowledgement rule;
- `config.list` shows the key with default on in ws and in wsflow;
- wsflow mirror and Pi tree drift tests pass.
