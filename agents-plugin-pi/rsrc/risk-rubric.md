# Risk Rubric

One ruler for grading how risky a change is before choosing how much
implementation and review it gets. Grade the axes below from the unit of work
in front of you — its stated scope, the touched tree, and what depends on it —
then read the tier guidance holistically. It is a rubric for judgment, not a
table to total: do not count highs or apply it as a mechanical gate.

## Axes

- **Correctness** — how likely the change is to behave wrong in a way ordinary
  testing would not immediately catch: concurrency, numeric edge cases,
  cross-cutting invariants, or logic that is easy to get subtly wrong even
  when it compiles and superficially runs.
- **Fit** — how likely the change conflicts with the existing architecture, an
  established convention, or another in-flight change, versus following a
  clear, already-proven pattern.
- **Test** — how well the available verification actually proves the change
  correct: an existing, exercised test path is low; a change with no
  meaningful automated check, or one covered only by a test the change itself
  rewrites, is high.
- **Security or contract** — how much the change touches a public interface,
  a security boundary, a data contract, or something an external consumer
  depends on, versus an internal detail nothing outside the change observes.

## Scale

- **Low** — the failure mode is implausible, contained, or would be caught
  trivially by normal verification if it occurred.
- **Moderate** — the failure mode is plausible, or its blast radius is real,
  but ordinary verification or review can reasonably be expected to catch it.
- **High** — the failure mode is plausible and would be costly, hard to
  detect, or hard to reverse if it slipped through, or ordinary verification
  would not catch it at all.

## Tier

Weigh the axes together as one read, not a checklist:

- **medium** — the risk profile reads low-to-moderate across the board; no
  axis stands out as a reason for heavier review.
- **large** — at least one axis reads high, or several moderate axes compound
  into a wide blast radius even without a single high (for example, moderate
  correctness risk on a widely-depended-on surface).
- **xlarge** — the read itself signals top-of-scale risk: multiple axes high
  together, or one axis high on something costly or hard to reverse if wrong
  — a security boundary, a contract many consumers hold, irreversible data
  loss. Pick this proactively when the read warrants it; it is not reserved
  for a retry.

Record the tier picked and the axis (or axes) that drove it. The rubric is the
ruler; the read is the caller's.
