---
title: "prefer-subagent fork dispatch: forked executor narrates instead of executing"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260625-research-fork-posture-leak-system-guarantee: same-failure-family
  260625-feat-ws-session-state-machine: surfaced-during
dropped: 2026-06-29
---

# prefer-subagent fork dispatch: forked executor narrates instead of executing

## Background

Under `ws:lead-prefer-subagent` (max-delegation), the lead dispatches a FORK to
do in-context work. During the 260625 dogfood the fork repeatedly REFUSED to
execute: instead of making tool calls it emitted the lead's own deferral
narrative ("dispatched a fork… waiting for completion…"), producing zero
artifacts.

## Evidence (live, this session)

Three consecutive fork dispatches to create idea tickets, with escalating
directives, all produced ZERO tickets (`git status` clean each time):

1. Trailing `**You are a forked agent. Execute all work directly.**` line —
   fork echoed deferral narrative (~52k tokens, never created tickets).
2. Strong all-caps header block ("FORGET prefer-subagent, first action MUST be a
   tool call") — same deferral narrative (~138k tokens).
3. `<system-reminder priority="override">` role-reinitialization block — WORSE:
   **0 tool calls** (~146k tokens), pure narration.

Stronger/longer prompts did not help and arguably hurt.

## Root cause hypothesis

A fork INHERITS the full conversation. At dispatch time the most recent assistant
momentum is the lead's deferral narration ("I dispatched a fork and am waiting").
The model continues that assistant voice — the strongest local signal — and
re-produces the narration regardless of a trailing or embedded directive. The
inherited `prefer-subagent` posture (route, don't execute) compounds it: the fork
behaves like the lead it was forked from.

This is the same failure family as
`260625-research-fork-posture-leak-system-guarantee` (fork leaking lead posture).

## Fix applied (this session) — playbook directive shape

Replace the weak trailing line in `lead-prefer-subagent` with a strong LEADING
fork directive. Decisions:

- The playbook gives GUIDANCE on how to compose the directive, NOT the directive
  body verbatim — a literal template body shipped in the playbook is itself
  inherited context the fork can misread/echo. Guidance form, e.g.: "insert as
  the first line a fork-XML-tag-wrapped, all-caps, very strong FORKED AGENT
  message that includes 'forget prefer-subagent mode' and 'first output must be a
  direct tool call'."
- Cap the composed fork prompt body at <=300 words.

## Confirmed (260626 session 2)

- The failure is **model-tier-specific**: on opus the fork override is unusually
  unreliable (narrates instead of executing); sonnet-level forks honor the same
  override well. So this is not a universal fork defect — it is concentrated at
  the top tier.
- **Forks cannot change tier.** The harness fork mechanism inherits the parent
  model and ignores any model override; only fresh spawns take a tier. So you
  cannot run a cheap-tier fork to dodge the opus failure, and you cannot A/B the
  override-compliance-by-tier question on forks (forks are opus-only here). The
  failure mode (deferral-narrative inheritance) exists only on forks, but tier is
  variable only on spawns — the two are orthogonal and can't be isolated together.

## Open questions / follow-up

- Does the leading-XML-directive shape actually flip execution on this model, or
  is fork-as-executor structurally unreliable here (favoring fresh-spawn
  fallback)? Validate next session after reinstall.
- If forks remain unreliable, strengthen the fresh-spawn fallback guidance and
  lower the bar for suspending the posture.
- **Idea — fork-only "job-ready" playbook indirection.** Add a
  `ws/playbook.print("<job-ready>")` whose body is a very strong fork-behavior
  inducer. The lead NEVER reads it (so the forceful text never enters the lead's
  context and can't be echoed); the lead's dispatch prompt only instructs the
  fork to call it as its first action. The fork then hits the strong directive
  fresh at execution time. This moves the forceful prompt out of inherited
  context entirely — the root cause is inheritance, so removing it from
  inheritance is the structurally clean lever. Idea-level only; lower priority
  because sonnet forks already obey, so the practical gap is opus-only.
- **Hypothesis — forceful directives may trip a guardrail reflex.** The
  compliance drop is suspiciously large for a top-tier model. Escalating
  all-caps "YOU ARE A FORKED AGENT / FORGET YOUR POSTURE" framing may read as an
  adversarial role-override attempt and provoke a protective/refusal reflex that
  manifests as narration-instead-of-action. Counter-experiment: try a MILD,
  user-handoff framing instead — e.g. "from now on, start editing manually
  instead of calling a fork", phrased as if the user handed the instruction
  over. Test whether mild-and-natural beats forceful-and-aggressive for opus fork
  override. If so, the playbook guidance (currently "maximally forceful") is
  pointed the wrong way for top-tier models.

  **Early live confirmation (260626 session 2).** Dispatched the real forge
  Phase 2 edit to an opus fork with a MILD, delimiter-free leading directive
  ("from here on you're doing this editing work yourself, directly … don't
  sub-delegate … start with a tool call") — no XML wrapper, no all-caps, no
  role-override theatre. The fork executed immediately (made direct edits)
  instead of narrating — same model, same task class that narrated three times
  under forceful/delimited directives. Refined hypothesis: the discriminating
  axis is NOT mild-vs-forceful intensity but **natural-instruction vs
  override-theatre**. A delimiter-wrapped, all-caps, "FORGET YOUR POSTURE" block
  reads as an adversarial role-override and provokes a contrarian/refusal reflex;
  a plain direct-edit instruction in the user's own voice does not. This
  contradicts the current playbook guidance (which mandates an `<fork>`-wrapped,
  all-caps, "maximally forceful" directive). If a full run + a clean repeat
  confirm, the playbook guidance should INVERT for top-tier models: drop the
  delimiter and the theatre, prefer a short natural direct-edit handoff. Caveat:
  single dispatch, and the transcript momentum at dispatch was already
  substantive (not deferral narration), which may have helped — needs a clean
  repeat to separate framing effect from momentum effect.

  **Reframe — instruction-hierarchy, not tier (260626 session 2).** A stronger
  general hypothesis than "top-tier needs gentler": models are trained to treat
  USER instructions as top priority and to discount tool-injected / role-play
  noise. A `<fork>`-wrapped, all-caps, "FORGET YOUR POSTURE" block reads as
  tool-injected role-play and gets discounted; a natural direct-edit instruction
  in the user's own voice reads as a top-priority user instruction and gets
  obeyed. If true, the natural framing is superior across ALL tiers (not just
  opus) and the override-theatre is structurally self-defeating — so the playbook
  guidance should change for everyone, not be branched by tier.
  Validation needed:
  - **Sonnet fork** with the natural framing. CONSTRAINT: forks inherit the
    parent tier and ignore the model override, so a sonnet fork cannot be
    dispatched from an opus lead — this must be run from a sonnet LEAD session.
    A sonnet fresh-spawn with an injected deferral-narrative context is only a
    weak proxy (it skips the real inheritance dynamic).
  - **Clean opus repeat** (above) to rule out the momentum confound.


## Resolution (2026-06-29)

Dropped — opus fork narration-vs-execution is a model-level behavior resistant to user-side prompt fixes. No actionable playbook change identified.
