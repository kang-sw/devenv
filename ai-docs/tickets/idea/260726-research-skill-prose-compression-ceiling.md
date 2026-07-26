---
title: "Skill prose compression ceiling - the audit result"
parent: 260630-epic-skill-playbook-diet
related:
  260726-refactor-retire-workset-convention: sibling outcome of the same session; unrelated in substance
---

# Skill prose compression ceiling - the audit result

## Why this exists

An audit ran on 2026-07-26 against a specific compression hypothesis. **The
hypothesis was largely wrong.** Recording the negative result so the same audit
is not re-run from scratch.

**Fidelity caveat.** This ticket was written at the end of the originating
session, from the conclusion rather than from the working notes. The qualitative
findings below are reliable; the line counts are approximate and are marked
where they need re-measurement before anyone acts on them.

## The hypothesis under test

Stated by the user, roughly:

1. Nearly all ws skill prose is procedure-heavy.
2. Many deterministic judgment algorithms have already moved into MCP tools.
3. Therefore, a modern agent should be able to work from a **single workflow
   document** plus enumerated capabilities ("i exists / ii exists / iii exists"),
   picking the right tool from conversation context, instead of imperative
   `if A then B via C` procedure text.

Target: **extreme compression.** Skills read: `lead-discuss`, `lead-proceed`,
`lead-implement`, `lead-write-ticket` - the ones already compressed to their
limit by prior models.

## Findings

### The enumeration rewrite does not pay

Converting `if A then B via C` into a capability list moves the branch condition
from the document into the agent's inference. That is only a win when the branch
is cheap to re-derive. Most of the surviving imperative text encodes a branch the
agent would get *wrong* by default - ordering constraints, gate preconditions,
single-writer rules - which is exactly the text that cannot become a bullet in a
capability list without losing its force.

The prose is procedure-heavy because the procedure is the content, not because
prior authors failed to compress.

### Single-document consolidation does not pay either

The per-skill split is load-bearing: skills are loaded on demand, so splitting is
what keeps any single load small. Merging them into one workflow document
converts N small conditional loads into one large unconditional load. The
"loaded once per session" framing hides that the merged document is paid for on
every session regardless of which workflow actually runs.

### Actual remaining compression is small, and mostly not a prose problem

Approximate, **needs re-measurement**: of roughly 567 lines examined, around 55
lines (~10%) looked genuinely reducible. Of those, the majority (~40 lines) were
reducible only by moving the logic into Go - i.e. they are
`260630-epic-skill-playbook-diet`'s **Lever B (MCP-ification)** work, not prose
editing. The pure-prose residue is on the order of ~15 lines across four skills.

## Implication for the parent epic

The diet's prose lever is close to exhausted on these four skills. Further
reduction on this surface should be pursued as Lever B (move the deterministic
part into a tool and let the prose shrink as a consequence), not as another
rewrite pass.

This does not generalize to skills outside the four examined. The audited set was
deliberately the already-compressed one.

## If this is picked up

Re-measure before acting. The line counts above are the weakest part of this
record; the qualitative findings are the part worth keeping.
