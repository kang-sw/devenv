# Risk Rubric

Grade the unit of work holistically across four axes before picking a worker
tier (use the tier anchors below as holistic guidance, not a mechanical count):

- **Correctness** — subtle bugs ordinary tests miss (concurrency, edge cases,
  cross-cutting invariants).
- **Fit** — conflict with architecture, conventions, or in-flight work (when
  known) vs. an established pattern.
- **Test** — strength of automated verification (exercised path or non-runtime
  doc = low; partial or indirect coverage = moderate; unverified code/behavior
  or self-rewritten test only = high).
- **Security or contract** — public interface, security boundary, data
  contract, or external consumer impact.

Scale: **Low** (contained, caught by normal verification) · **Moderate**
(plausible with real blast radius, catchable by verification/review) ·
**High** (plausible and costly, hard to detect/reverse, or missed by ordinary
checks).

Tier:
- **medium** — low-to-moderate across the board; no axis stands out.
- **large** — any high axis, or compounding moderates with wide blast radius.
- **xlarge** — multiple high axes, or one high on a costly/irreversible
  contract or security boundary (pick proactively when warranted, without
  waiting for a retry).

Record the tier picked and the driving axis (or axes).
