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

**The same owner then supplied the strongest argument against their own
position**, and it is recorded here with equal weight:

> 하긴, 사실 티켓 사이의 충돌을 막으려면 애초에 spec 쪽에 자리를 잡아놔야 하는
> 것도 맞네요.. 섣불리 없애자! 할 만한 아이템이 아닌 것도 맞는 것 같습니다.

Restated: `🚧` is not only documentation of planned behavior, it is a **claim
staked in shared territory**. Two in-flight `ready/` tickets touching the same
spec area collide in the spec, in one place, at write time. This is a different
argument from the spec-corpus-contradiction case below — that one is
ticket-vs-existing-text, this one is ticket-vs-ticket — and it is the argument
`## Spec Impact` answers worst (see Topic 3). Do not treat this research as
leaning toward retirement.

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
- **Concurrent-ticket collision has no other detection surface.** Owner-supplied,
  above. A `🚧` entry makes two `ready/` tickets contending for the same spec area
  collide at write time, in the spec, where both authors are already looking.
  Nothing else in the system does this: `## Spec Impact` is per-ticket prose,
  `tickets.verify` does not read ticket bodies, and
  `260726-feat-verify-ticket-graph-advisories` explicitly excludes body reads. So
  retiring `🚧` does not merely relocate this check — it removes it. Measure how
  often the collision actually occurs before pricing it, but note that the
  counterfactual is "undetected", not "detected elsewhere".
- **Adoption is unmeasured.** How many `🚧` entries exist, how many have a live
  backing `ready/` ticket, and how many went stale after their ticket closed are
  all countable and none are counted.

## Measured 2026-07-26

Counted before opining, because nobody had:

- **Real `🚧` usage across the whole spec corpus: exactly 1.** Six of the seven
  matches are in `documentation-system.md` describing the mechanism itself. The
  only live marker is `ws-web-dashboard/index.md:231`,
  `{#260524-dashboard-workspace-root-prune-policy}`.
- **That one is stale.** Its backing ticket
  `260524-feat-ws-dashboard-workspace-root-prune-policy` is in `.done/`. Per
  `documentation-system.md:240` the marker should have been stripped on
  implementation. It was not. So 100% of real usage sits in the failure state the
  mechanism exists to prevent, and nothing enforces stripping at close.
- **Contract-first declarations across live tickets: 1 yes / 8 no.** The single
  `yes` (`260722-feat-goal-run-autonomy-posture`) is in `todo/` and has produced
  no marker.

**Confound, stated so it is not lost:** adoption was measured over a period when
the mechanism was unusable — the ordering cycle in
`260726-bug-spec-planned-marker-ready-ticket-cycle` dead-ends anyone who tries
contract-first. Low adoption therefore mixes "not wanted" with "not possible".
The eight explicit `no` declarations are judge *choices* rather than mid-procedure
failures, which favours "not wanted", but does not settle it.

**Coverage shape, the structural finding.** `🚧` catches a ticket-vs-ticket
collision only when *both* tickets chose contract-first. Since that is a
per-ticket judge decision, coverage is the **square** of the adoption rate — at
50% adoption it catches 25% of collisions. Any surface driven by `ready/`
membership instead is linear. So the collision argument, taken seriously, is an
argument against `🚧` as its implementation rather than for it.

## Resolution direction (owner, 2026-07-26)

The question was reframed from "keep vs retire `🚧`" to **"where does each bundled
function belong"**, and the reviewer-role split settled it:

| Bundled function | Home |
|---|---|
| Contradiction with existing spec text | **design reviewer** — extended to read the `## Spec Impact` target, not only `spec:` frontmatter |
| Ticket-vs-ticket collision | **design reviewer** — scans `ready/` inventory `## Spec Impact` |
| Contract-vocabulary forcing | unresolved; the only remaining question for `🚧` |

Completeness was considered and **rejected on a hard constraint**: its prompt says
"Read only the ticket file at the provided path; do not load linked docs, specs,
or mental-model files." Corpus-wide checking there would delete the property that
defines the role. Design already reads specs, mental models, and related tickets
and carries the `right-problem check`, so this is a range extension, not a remit
change.

**Scan scope: `ready/` only** (owner decision). `todo/` and `idea/` are not
committed to landing, and including them trades bounded cost for noise.

**Landed inline, 2026-07-26** — the design-reviewer prose change shipped without
its own ticket, as an additive contract that is valuable whether or not `🚧`
survives. `wsrsrc` reads playbooks from the filesystem rather than `go:embed`, so
it is live in-tree immediately; downstream distribution still needs the next
plugin version bump to carry it.

## Topics

### 1. Does contract-vocabulary forcing justify the mechanism on its own?

The only function not relocated above. The claim: writing planned behavior into
the spec's own vocabulary, in place, produces better thinking than describing it
in ticket prose. Two sub-questions: is the claim true, and if true is it worth a
second management point plus an unstripped-marker failure mode that has already
fired once out of one.

Also still open: does `🚧` give planned behavior a stable anchor that other specs
can cross-reference? Check whether any spec actually references a `🚧` anchor —
with n=1 corpus-wide, likely no, but confirm rather than assume.

### 2. What is the real cost?

The sync burden, the staleness risk when a ticket closes or drops, the
"implemented vs planned" ambiguity inside a document read as current state, and
the unsatisfiable-ordering fix priced above.

### 3. Is the `ready/` inventory scan actually equivalent?

The owner's premise. Test it on two axes, because they fail differently:

- **Fidelity.** Is `## Spec Impact` written at a level that could substitute for
  spec-vocabulary contract text? Several were written this session — they state
  direction and caller-visible change, not contract. If substitution requires
  raising the bar, that cost belongs in the comparison, and it interacts with
  `260723-feat-ready-spec-address-hard-gate`.
- **Locality.** This is the harder failure and the one the owner identified. A
  scan is a *read* over N scattered per-ticket sections; the spec is a *write*
  into one shared location. Only the second forces two conflicting authors to
  meet. No amount of raising `## Spec Impact`'s fidelity fixes this, because the
  defect is where the text lives, not how good it is. Any retirement proposal
  must name what replaces this property — a real answer, not "reviewers will
  notice".

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
