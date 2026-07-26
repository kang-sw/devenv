---
title: Does the 🚧 planned-marker mechanism earn its management cost, or does ready/ ## Spec Impact already cover it?
related:
  260726-bug-spec-planned-marker-ready-ticket-cycle: currently sage-blocked; this research may supersede it rather than unblock it
  260726-bug-sage-ready-enforcement-single-chokepoint: its contract-first-spec was reversed yes -> no on exactly this reasoning
  260723-feat-ready-spec-address-hard-gate: owns the strength of the ready spec-address gate, which is the alternative surface named here
  260723-epic-ticket-write-reshape: hosts the enforcement-placement reshape this question sits inside
---

# Does the 🚧 planned-marker mechanism earn its management cost, or does ready/ ## Spec Impact already cover it?

## Owner Statement

Raised directly by the owner (2026-07-26), verbatim intent:

> spec의 WIP 앵커가 ready 티켓하고 중복되는데, 실제로는 관리 포인트 중복이고,
> "지금 구현된 내용"하고도 배치됨. 어차피 미구현 내용 포함해서 스펙을 확인하고
> 싶은거면 ready/ inventory의 spec impact를 훑으면 되는데 괜히 관리 포인트만
> 늘어나는 거 아닌가 싶은 거죠

Restated: a `🚧` entry duplicates a `ready/` ticket that already exists and
already describes the same planned behavior. That is a second place to keep in
sync, and it also sits inside a document whose job is to state *what is currently
implemented*. If the goal is to read the spec including not-yet-built behavior,
scanning the `ready/` inventory's `## Spec Impact` sections reaches the same
information without a second management point.

A second owner statement the same day, on the adjacent ticket: "지금의 스펙 계약이
별로 마음에 안 들어요. Spec Impact로 충분하다는 의견." That reversed
`260726-bug-sage-ready-enforcement-single-chokepoint`'s `contract-first-spec`
from yes to no, making it the first concrete instance of the preference.

## Why this is research, not a change ticket

The question is whether a shipped mechanism should exist. Neither the cost side
nor the benefit side is measured yet, and the answer determines whether an
already-written ticket gets implemented, rewritten, or dropped. Nothing should be
edited until the topics below are settled.

## Standing evidence

Evidence already in hand, gathered while triaging the downstream field report —
recorded here so it is not re-derived:

- **The mechanism's ordering is currently unsatisfiable.**
  `260726-bug-spec-planned-marker-ready-ticket-cycle` documents a three-rule cycle
  with no documented resolution order, which downstream had to break by human
  decision. That ticket is sage-**blocked**, and the blocking finding was that its
  proposed fix is violated by construction: `lead-write-spec` step 7 commits
  unconditionally while the contract-first branch invokes it inline. Fixing it
  therefore costs a cross-playbook commit-ownership change plus a widening of
  `tickets.verify`'s subject set to read spec bodies. That is the concrete price
  of keeping `🚧`, and it is the strongest single input to this question.
- **The one case that motivated keeping contract-first.** Downstream, existing
  spec text actively contradicted a ticket's central decision (a schema spec said
  writers must preserve existing index meaning while the ticket repurposed an
  index under owner authorization). A ticket-local `## Spec Impact` cannot resolve
  a contradiction living in the spec corpus. Any proposal to retire `🚧` must say
  what happens to this case — it is the mechanism's best surviving argument.
- **Adoption is unmeasured.** How many `🚧` entries exist, how many have a live
  backing `ready/` ticket, and how many went stale after their ticket closed are
  all countable and none are counted.

## Topics

### 1. What does `🚧` do that `## Spec Impact` cannot?

Enumerate honestly, then test each claim against the corpus. Candidate answers:
contradiction resolution in place (above); giving planned behavior a stable
anchor other specs can reference; forcing the author to write the contract in the
spec's own vocabulary rather than the ticket's. Whether any of these survives
contact with actual usage is the crux.

### 2. What is the real cost?

The sync burden, the staleness risk when a ticket closes or drops, the
"implemented vs planned" ambiguity inside a document read as current state, and
the unsatisfiable-ordering fix priced above.

### 3. Is the `ready/` inventory scan actually equivalent?

The owner's premise. Test it: is `## Spec Impact` currently written at a fidelity
that would substitute for spec-vocabulary contract text? Several were written
this session — they state direction and caller-visible change, not contract. If
substitution requires raising `## Spec Impact`'s bar, that cost belongs in the
comparison, and it interacts with `260723-feat-ready-spec-address-hard-gate`.

### 4. What happens to the dependent tickets?

`260726-bug-spec-planned-marker-ready-ticket-cycle` is the direct dependent — if
`🚧` retires, that ticket is dropped rather than unblocked, and its blocked
finding about inline commit ownership must be re-homed because it is a real
defect independent of `🚧`. Also check `judge: contract-first-spec`'s remaining
role and whether `lead-write-spec` keeps a planned-content branch at all.

## Non-Scope

- Do not edit `spec-conventions`, `lead-write-spec`, or any `🚧` entry from this
  ticket. It produces a recommendation; a separate actionable ticket implements it.
- The inline commit-ownership defect is a genuine bug on its own terms. Do not let
  its fate be decided as a side effect of the `🚧` verdict.
