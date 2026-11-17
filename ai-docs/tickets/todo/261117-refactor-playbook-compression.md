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

## Phases

### Phase 1: Diet each playbook with the owner

For each playbook in the scope order: settle the diet with the owner, write
the compressed text, regenerate the manifests and the wsflow and pi mirrors,
run `go test ./internal/wsrsrc` and the targeted `internal/mcp` playbook
tests, then commit and push, one commit per playbook.

Verification: rsrc and mirror tests pass after each commit; `Ticket
Updates` in each commit name this ticket; the Result records the before and
after word count of each playbook.
