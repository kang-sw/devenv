---
title: Make implement review fixes follow implementation owner
spec:
  - 260505-implementation-workflow-skills
related-mental-model:
  - workflow-skills
---

# Make implement review fixes follow implementation owner

## Background

After `lead-implement` unified direct editing and delegated implementation, review
fix ownership became easy to misread. The current skill says lead fixes
correctness, security, contract, and regression findings, while the detailed
review steps relay findings to the implementer only for partitioned review.
That wording can make delegated review findings get applied by the lead instead
of returning to the implementation agent.

The intended contract is that review findings are fixed by the implementation
owner:

- Direct-edit mode: the lead is the implementation owner and applies fixes.
- Delegated mode: the implementer agent is the implementation owner and receives
  non-clean review path files.
- The lead owns triage, style-only or scope-expanding rejection, adjudication,
  caller escalation, verification, and re-review orchestration.

Review relay should continue to pass findings by review path files, not by
copying reviewer file contents into lead context.

## Phases

### Phase 1: Clarify review-fix ownership

Update the full `lead-implement` skill and matching workflow documentation so
review-fix ownership is expressed through the implementation owner rather than
through lead-owned fixing.

Preserve these behavior constraints:

- Direct-edit review fixes remain lead-applied because the lead is the
  implementation owner in that mode.
- Delegated review fixes are sent back to the implementer agent with the review
  relay prompt.
- Lead triage still classifies fix, won't-fix, deferred, style-only, and
  scope-expanding findings before relay or rejection.
- Correctness, security, contract, and regression findings cannot be silently
  rejected.
- Review path files remain the relay artifact.
- Re-review and cycle caps stay equivalent unless the implementation finds a
  documented reason to change them.

Check `agents-plugin-wsflow/` for mirroring impact. If wsflow behavior is
already intentionally different through `lead-edit`, document that no wsflow
skill change is needed; otherwise update the wsflow surface in the same logical
change.

Verification:

- Run the skill-authoring invariant checklist for changed skill lines.
- Run a fresh-reader audit focused on the edited `lead-implement` review
  section.
- Verify the spec and mental-model entries no longer imply that the lead
  directly fixes delegated implementation findings.

## Staleness audit (2026-06-19)

Still live and unimplemented (Phase 1 has no Result). The current `lead-implement`
expresses delegated review relay and direct-edit fixes as separate steps
(`agents-plugin/rsrc/lead-implement/lead-implement.md:97-98`) but does not yet frame
both as variants of a single "implementation owner" governing rule. The ticket's
intent is unchanged; baseline confirmed against the current skill.
