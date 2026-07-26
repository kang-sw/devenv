---
title: Code review loops fill their cycle budget — investigate what keeps generating
  non-clean rather than what fails to stop it
related:
  260726-bug-lead-implement-lost-review-relay-cycle-cap: owns the termination half —
    the budget, the adjudicator, and the elevated implementer. This ticket owns the
    generation half. A cap bounds the damage; it does not explain why non-clean keeps
    being produced
  260630-epic-skill-playbook-diet: any remedy here is prompt text, so it lands under
    the diet's compression constraints
---

# Code review loops fill their cycle budget

## Field observation

Delegated code review in the implement loop almost always runs its cycles to the
end rather than reaching `clean` early. Reported by the maintainer across repeated
runs; not yet measured.

The adjacent ticket
`260726-bug-lead-implement-lost-review-relay-cycle-cap` establishes that no cycle
budget is currently installed in the playbook or the generated todos, so "fills
three cycles" describes observed behavior, not an enforced limit.

## Why this is separate from the cap ticket

The cap ticket asks *how the loop stops*. This one asks *why it keeps going*. A
budget bounds the cost of a non-converging loop; it does not make the loop
converge. If the generator is the real defect, the cap ships and the loop still
consumes its full allowance every run — which is the behavior actually complained
about.

## What the prompt evidence already shows

Read of `agents-plugin/rsrc/code-reviewer.md` and the three partition playbooks.
Code review is **not** in the same position as the ticket reviewers were before
`v0.36.15`: it already has a severity table, a bounded input (the diff), and an
explicit re-review narrowing rule. So the ticket-reviewer remedy does not
transplant, and two different candidate causes remain.

**Candidate A — `Important` is defined by category, not by consequence.** The
severity table gives `Important` as "standards violations, missing boundary
validation, architectural drift". *Architectural drift* is elastic: most diffs
admit some reading of it. The reviewer's constraints then forbid a `clean` status
while any `Important` finding remains. So `clean` requires zero findings in an
open-ended category. This is the same defect shape `v0.36.15` fixed one level up —
the table exists, but its middle row enumerates kinds rather than naming a
consequence.

**Candidate B — the re-review narrowing rule has a hole the fix loop drives
through.** The rule says to focus only on whether reported issues were addressed
and not to re-review unchanged code. But a fix *is* changed code. Every relay adds
diff surface that is legitimately in scope, so the rule bounds re-review against
the original diff while the loop keeps growing the diff. Unlike Candidate A this
failure is specific to code review — a ticket reviewer's input does not grow as
the loop runs.

The two are not exclusive; the question is which dominates.

## The measurement that discriminates them

For past runs, classify each cycle-2 and cycle-3 finding as **carryover** (a
restatement of an earlier finding) or **new**, and for each new finding record
whether it points at code introduced by a fix or at code present in the original
diff.

- Mostly carryover → neither candidate; the loop is failing to *resolve*, and the
  cap ticket's adjudicator and elevated implementer already own that.
- New findings in original-diff code → Candidate A: the reviewer keeps finding
  fresh `Important` items in a fixed surface, which points at an elastic category.
- New findings in fix-introduced code → Candidate B: the surface itself is growing.

Review findings files from completed runs are the cheapest source. Gather across
several downstream repositories rather than this one — the maintainer's
observation comes from downstream use, and this repo's own runs are few and
atypical.

## Also noted, lower confidence

- `clean` is defined only as the absence of Critical/Important, with no positive
  statement. `v0.36.15` treated the same shape in the ticket reviewers as a real
  defect (a verdict back-derived from an issue list rather than judged).
- The reviewer doctrine names "defect signal density", which does not name a
  finite resource the way the authoring rules' doctrine format asks for.

## Deliberately not decided

No remedy is chosen. Candidates A and B imply different edits — a severity
redefinition versus a re-review scope rule — and committing to either before the
measurement would repeat the mistake of fixing the surface that was easiest to
see.
