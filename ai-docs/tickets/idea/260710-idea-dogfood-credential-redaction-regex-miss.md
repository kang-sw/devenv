---
title: "Session tooling's credential-redaction regex missed a linked-server passphrase fragment during a dogfood run"
related:
  260707-chore-dashboard-linked-server-tunnel-dogfood-plan: surfaced during this ticket's Phase 2 reversed-topology dogfood walk
---

# Session tooling's credential-redaction regex missed a linked-server passphrase fragment during a dogfood run

## Background

While executing `260707-chore-dashboard-linked-server-tunnel-dogfood-plan`'s
Phase 2 (reversed-topology relink dogfood walk, 2026-07-10), the implementer
subagent reported: "One redaction-regex miss briefly exposed a partial
passphrase fragment in tool output mid-run; treated as burned immediately by
restarting the affected daemon (rotating the passphrase) before continuing."

This is a gap in this session/harness's own credential-redaction tooling
(the mechanism meant to keep pairing tokens, link passphrases, and bearer
tokens out of tool-output transcripts per this repo's own
`AGENTS.md`/ticket-Constraints convention), not a ws-dashboard product bug.
The passphrase was immediately rotated by the implementer as a mitigation,
and confirmed (via `git diff` on the resulting commit) that nothing
credential-shaped landed in the actual git history — only transient tool
output was affected.

Not yet investigated: which specific output shape slipped past the
redaction regex (fragment length/position, quoting style, JSON vs.
plain-text embedding, etc.), whether this is reproducible, and whether the
regex needs a fix or the dogfooding convention needs a different mitigation
(e.g. always treating dogfood-generated passphrases as single-use/rotate-
after, regardless of redaction confidence).

## Escalations

- None yet.

