---
title: "플레이북 다이어트: 스킬별로 사용자와 꼭 필요한 문장만 남기기"
parent: 261116-epic-lead-orchestration-profile
related:
  261116-feat-orchestration-profile-foundation: prerequisite - Phase 2 splits and folds text this ticket compresses first
  261117-feat-role-scoped-workflow-context: prerequisite - its manual modularization starts from the compressed manual
---

# Playbook compression: keep only the sentences each skill needs

## Background

Owner direction (2026-11-17 21:48): before splitting the bloated lead-side
text into sub-lead parts, first reduce each playbook to the sentences it
needs. Splitting first would carry dead text into two files and edit it
twice, and foundation (65) (the `direct` manual is byte-identical to the
pre-split output) would freeze today's bloat.

Word counts at the start (`agents-plugin/rsrc`, 2026-11-17, after develop
550fbced7): `whiteboard.md` 5934 (embedded whole in the lead manual when the
board knob is on), `lead-run.md` 1786, `lead-workflow-manual.md` 1631,
`ticket-handler.md` 1549, `lead-ship.md` 1269, `lead-review.md` 1162,
`lead-delegate.md` 852.

## Decisions

- **Order before the split.** This pass runs before foundation Phase 2 and
  before the manual modularization of
  `261117-feat-role-scoped-workflow-context`. (77)=a. Rejected: Phase 2 split
  first with compression left deferred; compressing every playbook now.
- **Scope.** `lead-delegate`, `lead-workflow-manual`, `whiteboard`,
  `lead-run`, `ticket-handler`, `lead-review`, `lead-ship`, in that order.
  The owner added `lead-delegate` ("delegation is just a way of handing off
  work"). Other lead-kept playbooks stay under the epic's deferred
  compression. (77)=a, (81)=a. Rejected: manual first; whiteboard first.
- **Method.** One skill at a time, in a deep discussion between the lead and
  the owner: the owner states their understanding of the skill's core, the
  lead proposes what must stay in, and the two iterate the diet until the
  owner is satisfied. The lead drafts the text with the owner (epic (09)); no
  worker drafts or cuts. (78), (80). Rejected: an independent fresh-read audit
  proposing cuts for sentence-level approval; a compressed draft shown as a
  diff first.
- **Diet style.** Rewrite as short bullets of the form "situation → action",
  so the reading agent never has to interpret a paragraph to find the rule.
  Move a judgment the lead would otherwise interpret into the executor's
  self-describing report (what blocked it, what to tell the user, where
  follow-ups go), and move rules every spawn site shares into the workflow
  manual. Owner, lead-delegate session 2026-11-17 (82).
- **Delegate report.** Every delegate ends with the root-level
  `delegate-report` block (`status: done | blocked | ask-user`, `result`,
  `blocker`, `ask`, `tell_user`, `resume`, `omitted`): `delegate-implementer`
  includes it, and the lead renders it directly for a non-mutating delegate.
  Its shape follows epic (01) so foundation Phase 2's `sublead-protocol`
  extends it with `next:`/`on_answer:` rather than defining a second report.
  (83)=a, (86)=ok. Rejected: rewriting only `delegate-implementer`'s Report;
  deferring the report move to Phase 2.
- **Tier rule.** One "Choosing a tier" section in the workflow manual, which
  every spawn site points at; the bar is stated as task character, never as
  model names. Calibration from the owner: small ≈ a mid model, medium ≈ a top
  model at medium effort, large covers most hard work, xlarge is research
  grade. (84)=ok.
- **Trivial-ticket hatch.** A ticket whose change is trivial (no open
  decision, no new structure, files named, one commit) may be proposed for
  delegation instead of lead-run; the user's yes runs it, and the lead closes
  the ticket citing the commit. Landed in `lead-delegate`; `lead-run`'s entry
  gets the matching pointer in its diet. (85)
- **Placement.** One epic child ticket tracks the pass. (79). Rejected:
  folding it into `261117-feat-role-scoped-workflow-context` - compression
  preserves meaning, modularization changes structure, and this pass touches
  files that ticket does not.

## Constraints

- A sentence whose removal or rewording would change behavior is raised to
  the owner as its own item, not slipped into the diet.
- Keep render anchors and the override marker blocks in `ticket-handler.md`
  and `lead-ticket.md` (foundation (73), split-gaps (08)=a).
- Tests assert behavior, never playbook sentences (owner's standing
  no-prose-verbatim-test directive).
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/)
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/)

## Session Procedure

How each playbook's diet runs, so a later session resumes it as is:

1. Open a whiteboard topic `diet-<playbook>`; post the playbook's section map
   (words per section, one line of content each, no verdicts).
2. Ask the owner for their reading of the skill's core (one policy item).
3. Answer with what must stay, each with its provenance (`git log --follow`
   on the playbook: which commit or incident created the rule), what to cut
   or move, and a full English draft in a panel.
4. Iterate on the owner's notes; a change of behavior (not just wording)
   becomes its own item, and a new cross-skill interface is an Approval
   Protocol ask.
5. On the owner's ok, author the final text, then regenerate in this order:
   `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run
   TestGenerateRealManifest`; `WS_REGEN_WSFLOW_RSRC=1 ... -run
   TestRegenerateWsflowRsrcMirror`; `WS_REGEN_COMPOSED_SKILLS=1 ... -run
   TestRegenerateComposedSkills`; `WS_REGEN_WSFLOW_SKILLS=1 ... -run
   TestRegenerateWsflowSkillsMirror`; `WSRSRC_REGEN_SKILLS=1 ... -run
   TestGenerateRealSkillsManifest`; copy changed `agents-plugin/rsrc` files
   and `manifest.json` into `agents-plugin-pi/rsrc` (hand-synced,
   `TestPiMirrorUpToDate`); `python3
   agents-plugin-pi/scripts/prepare-workflow-bundles.py` and
   `prepare-native-payload.py --first-party-only`.
6. Run `go test ./internal/wsrsrc`, the targeted `internal/mcp` playbook and
   render tests, and the wsflow package unittests; fix a test that pinned
   removed wording to the behavior it guards. On the owner's office Windows
   PC, Go tests and ws executables run on the remote w5090 host, never
   locally (antivirus; develop handoff note 2026-11-17).
7. One commit per playbook with `## Ticket Updates` naming this ticket and
   the word counts; push; close the board topic.

## Progress

- `lead-delegate`: done (0aca7a448), 852 → 410 words; added
  `delegate-report.md` (151) and the manual's tier rule.
- `lead-workflow-manual`: done, 1754 → 1383 words (`wc -w`; 1631 at the
  start plus the tier-rule section added by the lead-delegate commit). Session setup
  ferrule guidance removed; lead-revive passes root ((91)=a); Explore names
  the tier only (no model names; the tier-model golden test was retargeted
  to the Explore agent term).
- Next: `whiteboard`, `lead-run` (add the trivial-ticket pointer),
  `ticket-handler`, `lead-review`, `lead-ship`.

## Phases

### Phase 1: Diet each playbook with the owner

For each playbook in the scope order: settle the diet with the owner, write
the compressed text, regenerate the manifests and the wsflow and pi mirrors,
run `go test ./internal/wsrsrc` and the targeted `internal/mcp` playbook
tests, then commit and push, one commit per playbook.

Verification: rsrc and mirror tests pass after each commit; `Ticket
Updates` in each commit name this ticket; the Result records the before and
after word count of each playbook.
