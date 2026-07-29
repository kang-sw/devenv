---
title: enter.implement has no prose-only dimension, and low_ceremony_if_safe is unreachable for shipped skill text
related:
  260630-epic-skill-playbook-diet: same direction of travel — remove ceremony that does not earn its cost
  260605-research-ws-native-subagent-pivot: delegation defaults and the routing surface this questions
related-mental-model:
  - workflow-skills
---

# enter.implement has no prose-only dimension, and low_ceremony_if_safe is unreachable for shipped skill text

## Background

Raised by the user during a dogfood run: a change that added **two sentences of
prose** to `lead-prefer-subagent/SKILL.md` plus one spec paragraph took the full
pipeline — survey plan, delegated implementer, two partitioned reviewers, one
relay, an implementation branch, and a merge commit. Landed as `82e5f6ff`.

Part of the weight traces to lead-supplied facts, not the resolver; that half is
recorded honestly below. But two structural gaps look real.

## Topic 1: no prose-only dimension

`enter.implement` resolves delegation, plan depth, and review allocation from
`span`, `surface`, `new_public_symbol`, `new_type_contract`, `test_surface`, and
the risk group. There is no fact that distinguishes:

- a change to a Go type contract spanning four modules, from
- a change to two sentences of shipped prose that happens to be governed by a
  spec anchor.

Both are honestly describable as `span=multi-file`, `surface=public-interface`,
with moderate fit and contract risk. Every individual fact is true; the composite
verdict is disproportionate. The resolver cannot see that the artifact is prose,
that its "tests" are drift guards rather than behavior, and that its blast radius
is bounded by a mirror regen.

## Topic 2: low_ceremony_if_safe is structurally unreachable here

The user explicitly asked to commit straight to `main`, which maps to
`policy.low_ceremony_if_safe=yes`. The resolver returned:

> policy.low_ceremony_if_safe=yes not applicable; continuing with standard
> branch path

The low-ceremony exception is gated on the target independently satisfying
automatic direct-edit and lead-only review, which `surface=public-interface`
defeats. But **shipped skill and spec prose is always public-interface by
definition**. So for the entire class of workflow-prose changes — which this repo
makes constantly — the one lever the user has is permanently inert. It is not
that the lever was overridden by judgment; it cannot ever engage.

## Lead-side contribution (recorded so the research does not over-blame the router)

On the observed run the lead supplied `surface=public-interface`,
`fit=moderate`, and `security_or_contract=moderate`. Per the documented
allocation rule, public-interface alone does not add review partitions — the two
`moderate` risk values did. Those were defensible individually and arguably
inflated for a prose change. Any fix should ask whether the facts are
mis-specified for prose, not only whether the resolver is.

## Open questions

- Is the right shape a new fact (`artifact_class: prose|code`), a new
  `surface` value, or a policy flag? A new fact is the most honest but widens
  a schema the diet epic wants narrower.
- Should the low-ceremony gate use something other than `surface` for its
  direct-edit predicate, so prose can qualify without weakening it for code?
- Does the standing preference "skill/spec prose is lead-authored directly"
  belong in `config` (a `lead-tune` knob) rather than being re-derived per run?
  The user stated it mid-run on this session; it had to be applied by hand as an
  explicit direct-edit override after routing had already committed to delegated.
- Does any of this survive the diet epic, or does the epic's pressure on prose
  cost already argue for a cheaper default path for prose changes?

## Notes

- Do not treat "the run felt heavy" as sufficient evidence to loosen delegation
  broadly. The partitioned review on this run **did** find a real Important
  defect (a stale "the sole carve-out" absolute in the spec) that the lead and
  the implementer both missed. Any cheaper prose path must not lose that.
