---
title: "Typed decision models (TypeSafe Jev and peers): new domains for ws, not grafts onto lead judgment"
related:
  260915-research-lead-run-tier-selection-qualitative: context; the worker-tier choice is one in-place graft site surveyed here and deprioritized
  261002-feat-pi-lead-ws-owned-compaction: context; the Pi compaction trigger and fallback summarizer are graft sites surveyed here
---

# Typed decision models (TypeSafe Jev and peers): new domains for ws, not grafts onto lead judgment

## Background

TypeSafe AI released Jev on 2026-09-15: a "System One" decision model that
generates no text. Callers send a state (text or JSON) plus named typed
questions and get back typed answers with calibrated probabilities. Adoption
moved fast (Vercel reported 13% of paid AI Gateway users within 24 hours), and
an ecosystem of harness plugins appeared within two weeks.

The question is whether ws should adopt this primitive and where. A first
pass looked for in-place graft sites: ws-mcp model routing, the route
resolvers, Pi compaction triggers. The user judged that the lead-judgment
paths are already heavily optimized and that grafting a decision model onto
them is not elegant. The productive direction is **new domains**: places
where a cheap, calibrated, typed judgment makes possible something ws does not
do at all today. This ticket records the survey and the candidates. It
authorizes no implementation.

## The primitive

- **Question types:**
  - `Choice`: one of the caller's options, with per-option probabilities and a confidence.
  - `Score`: an ordered rubric of up to 10 levels; returns the probability-weighted score, the distribution and a confidence.
  - `Noul`: the probability that a proposition is true.
- **Batching:** many questions share one state in one call, and adding questions barely changes latency.
- **Cost and latency:**
  - $0.042 per million input tokens; output tokens are free.
  - Vendor-published latency is roughly 70–500 ms.
  - OpenRouter's own benchmark measured p50 194 ms and p95 633 ms.
- **Limits:**
  - State up to 32k tokens.
  - Text input only.
  - Hosted only (TypeSafe API, or OpenRouter at `/api/alpha/decisions`, still alpha); proprietary, no self-host.
- **Weaknesses:**
  - Accuracy drops when the state carries irrelevant detail.
  - Unreliable at arithmetic, counting and date comparison.
  - Not deterministic across runs.
  - Gives no rationale.
- **Calibration:** holds only in aggregate. Thresholds are application policy that has to be tuned on the caller's own labeled data. Probabilities near 0.5 should be treated as "needs follow-up", not as a weak lean.
- **Market:** analysts expect the major labs to ship competing decision models, so any integration should not be vendor-shaped.

## Ecosystem use cases surveyed

Use cases beyond the obvious routing, moderation and tool-gating:

| Use case | Mechanism | Example |
|---|---|---|
| Prune-not-summarize compaction | `Noul(keep call)` and `Noul(keep result in full)` for every tool call/result pair. Below threshold, the result is stubbed (`ok, N chars (omitted)`) or the pair dropped; everything kept stays verbatim. The first message and the newest N are pinned. | fast-jev-compaction, cc-mod-jev, jev-compact (Claude Code plugins) |
| Oversized tool-output gate | When a large output arrives, ask whether the full text is needed and cut the middle if not. | dsh-jev result-shaper: 32,680 → 237 chars |
| Semantic loop/stagnation detection | Judge the trajectory as CONTINUE / WARN / REPLAN / HALT. Catches semantic loops, not only exact repeats. | dsh-jev loop-guard: pLoop 0.88 on real loops, 0.00 on healthy ones |
| "Done" claim verification | Check transcript evidence before trusting an agent's completion claim. | MarkTechPost #17 |
| Trace mining into reusable skills | Score each captured agent run for evidence, reuse potential and human-correction signals; a policy then promotes, reviews or discards it as a skill. | Beacon (Asymptote Labs), across Claude Code, Codex, Cursor, OpenCode and more |
| Semantic linting at edit time | `Noul` per team rule against a changed hunk, flagged as it is written. | MarkTechPost #20; diff verification for "API boundary breach / missing test" |
| Citation / evidence grounding | `Choice{supports, insufficient, contradicts, unrelated}` over (claim, cited excerpt). | awesome-jev coding-agent use cases |
| Run-boundary fault triage | Classify a terminal failure as harness bug, environment issue or plan defect. | Servant-Software Guardrails #755 (scoping only, no results) |
| Retrieval reranking / tool pruning | Score candidates for relevance and inject only the top-K. | dsh-jev tool-pruner: 12 → 5 tools; legal reranking top-1 5% → 18% |

## Negative and cautionary evidence

- **dsh-jev A/B pilot:** 20 real tasks, a full plugin suite (tool pruning, loop guard, result shaping, skill routing). Outcome was 2 wins, 2 losses and 13 ties, with no stable advantage. Token savings were negligible: median prompt −3.2%, uncached input +16.6%. Bolting decisions onto an already-working coding loop does not obviously pay.
- **Substitute models:** a fork of fast-jev-compaction tried open-weight decision checkpoints in place of Jev. They scored every tool call almost uniformly, so almost nothing was pruned. The fork's authors say these models score near chance on decision types they were not fine-tuned for. A "same interface, cheaper backend" swap is not free; discrimination has to be measured per decision type.
- **Benchmark sample sizes:** the published benchmarks are small (60 and 40 items) and vendor-adjacent (OpenRouter's own blog).
- **Explicit non-goals in other harnesses:** Guardrails #755 notes that a decision model cannot guard generative work in flight, cannot read files or logs beyond the state it is given, and sends data to a third party unless an enterprise zero-retention option is used. Its plan wires nothing in until a replay comparison on labeled data says it is worth it.

## In-place graft assessment (deprioritized)

The first pass mapped where a decision primitive could replace an existing ws mechanism:

- **ws-mcp model routing:**
  - What it is: `ResolveAgentForHarnessConfig` (`agents-plugin-tool/internal/wsconfig/config.go:426`) is a static tier → {backend, model, effort} lookup, with the tier fixed per playbook frontmatter or `tier_override`.
  - Why it is a poor fit: ws-mcp never sees request content, so the "query router" pattern has nothing to classify.
- **Worker-tier and review-breadth selection:**
  - What it is: `ticket-fact-populator` (a full LLM) grades 11 enum route facts, including four `risk.*` rows, under an anti-`low` calibration rule. `selectProceedRoute` (`internal/mcp/proceed_resolver.go:480`) and `deriveImplementReviewAlloc` (`internal/mcp/implement_resolver.go:823`) apply deterministic rules over those facts. Worker tier is the lead's dispatch-time read (`260915-research-lead-run-tier-selection-qualitative`).
  - Conceptual fit: good, since each row is a ≤4-level `Score`.
  - Practical fit: poor. Grounding needs reading the code, there is no labeled data, and the step runs once per ticket ahead of a multi-minute worker, so latency and cost gains are nil.
- **Pi compaction trigger:**
  - What it is: `fireCompactionTriggers` (`agents-plugin-pi/src/goal-loop.ts:1183`) uses only the context percent. The advisory fires at 50% at `agent_end`, the hard steer at 80% at `turn_end`. "Is this a safe compaction point" is left to the lead's prose judgment.
  - Candidate: a `Noul` at a natural boundary is feasible. It is still a graft onto a lead-owned judgment.
- **Pi runaway guard:**
  - What it is: `decideOnSettle` (`goal-loop.ts:407`) force-stops after 10 consecutive tool-call-free re-fires.
  - Candidate: a semantic loop detector could fire earlier, but a false-positive stop is costly.
- **Not a fit at all:**
  - Reviewer verdicts and follow-up severity need diff and code reading beyond a 32k text state.
  - The overflow fallback summarizer needs text generation.

## Candidate new domains for ws

All entries here are non-authoritative proposals (see Outcome Ledger). Each
names the new capability, why it is new rather than a graft, and the open
risk.

1. **Dogfood-surprise mining from session traces.**
   - What it would do: an offline batch over ws runtime debug events and session transcripts asks, per tool interaction, whether a ws tool behaved contrary to the caller's evident expectation. Hits become candidate `idea/` tickets for the lead to accept or discard.
   - Why it is new: today the `Dogfood surprises get captured` rule depends entirely on the lead noticing in the moment. This is the Beacon pattern, pointed at ws's own quality loop.
   - Risks: privacy of the transcripts sent out; the 32k state forces per-interaction windows.
2. **Prune-not-summarize pre-pass for Pi.**
   - What it would do: score stale tool call/result pairs and stub them verbatim-preservingly before the context percent reaches the compaction advisory.
   - Why it is new: it complements the ws-owned prose compaction rather than replacing it, and it pushes the compaction point later without loss of what is kept.
   - Risk: the substitute-model evidence above; discrimination must be measured on real Pi sessions.
3. **Intent-aware exec output shaping.**
   - What it would do: ws-mcp `exec` jobs already cap inline output (`InlineBudget`) and offer tail/grep. An optional caller-supplied intent would let a decision pass choose which segments of a long log matter, instead of returning head or tail.
   - Why it is new: today the selection is positional, not semantic.
4. **CI and exec failure triage.**
   - What it would do: classify a failing job or exec result as environment/flake versus a real failure (the Guardrails "run-boundary triage" pattern).
   - Where it applies: the ship CI gate, e.g. Windows shard flakes such as `260915-bug-windows-exec-abort-tempdir-cleanup-flake`.
5. **Edit-time semantic lint for project-declared rules.**
   - What it would do: a `Noul` per changed hunk against the rules a project declares, flagged at commit time as a warning, not a gate.
   - Which rules: this repo's shipped-surface boundary (Architecture Rule 4), the skill-authoring invariant checklist and wsflow mirroring drift.
   - Generic form: driven from the project's own `### Implementation Conventions` table, never from this repository's rules, per Rule 5.
   - Why it is new: today these are caught only at review time by the fit partition.
6. **Evidence grounding for AI-authored claims.**
   - What it would do: `Choice{supports, insufficient, contradicts, unrelated}` over (claim, cited excerpt).
   - Targets: ticket `## Route Facts` evidence cells, review finding citations, rationale entries and manual claims.
   - Why it is new: it would detect drifted docs and stale tickets semantically, beyond the current freshness checks.
7. **Retrieval reranking for notes and rationale.** Rank `note.query` and `rationale.query` results by relevance to the current task before they are injected into a session, rather than dumping all matches.

## Deferred design sketch: a backend-neutral decision interface

Discussed and explicitly **not pursued now**. Recorded so a later ticket does not re-derive it.

- **Tool contract:** a ws-mcp tool `decision.ask(state, questions)`:
  - Questions are typed `choice | score | bool` with instructions and options or levels.
  - The response gives per-question value, probabilities and confidence, plus `backend`, `calibrated: bool` and `latency_ms`.
- **Pluggable backends:**
  - `jev`, via OpenRouter or TypeSafe: calibrated, about 200 ms.
  - `llm-http`: an OpenAI-compatible endpoint running a light model (e.g. the small tier's model). Option distributions come from logprobs; not calibrated.
  - `llm-cli`: `codex exec` or `claude -p` reusing host auth. Needs no secret, but takes seconds and yields only self-reported confidence.
  - One OpenRouter client and key would serve both `jev` and `llm-http`.
- **Default and fallback:** off by default. When unavailable the tool returns `unavailable`, and every consumer keeps a deterministic fallback.
- **Secrets:** config stores the name of an environment variable (e.g. `decision.api_key_env`), never the key itself, so no key reaches the config ledger.
- **Shadow log:** every decision is logged to wsstate (state hash, questions, answers, eventual outcome). This builds the labeled data that the calibration caveat requires.
- **Consumers:** Pi would reach it through its existing ws-mcp bridge (`startBridge`) with `pi.*` adapter config switches, so it needs no HTTP client of its own.

## Constraints any follow-up must respect

- **ws-mcp has no outbound network today.**
  - Process spawns are limited to git and `execjob`; `net/http` appears only in tests.
  - There is no secret handling.
  - Any backend that calls a model API makes ws-mcp a model client for the first time, which is an architecture change.
- **The Pi extension already has outbound paths.**
  - Raw `node:https` in `web-fetch.ts`.
  - Provider auth via `ctx.modelRegistry` / `getProviderAuth`.
  - Whether an OpenRouter key is reachable through Pi provider auth is unverified.
- **Architecture Rules 3–5.**
  - Shipped behavior must be host-neutral and downstream-first.
  - A proprietary hosted API is opt-in and never a hard dependency.
  - A shipped lint (candidate 5) reads project-declared rules through a generic hook.
- **Data egress.** Enabling any hosted backend sends state text (transcripts, ticket bodies, diffs) to a third party. Opt-in documentation must say so.

## Outcome Ledger

### Verified Findings

- **Jev's primitive:** Jev exposes `Choice`/`Score`/`Noul` with calibrated probabilities. It costs $0.042 per million input tokens with free output, caps state at 32k tokens and is hosted only; OpenRouter's endpoint is alpha. Measured latency is p50 194 ms and p95 633 ms (OpenRouter benchmark).
- **Jev's weaknesses:** degraded accuracy on states with irrelevant detail; unreliable arithmetic, counting and dates; non-deterministic; no rationale.
- **A full plugin suite gave no stable win:** a Jev suite on an existing coding loop showed no stable advantage in a 20-task A/B (2W/2L/13T; dsh-jev).
- **Substitute models did not discriminate:** open-weight substitute decision checkpoints scored near chance on untrained decision types (fast-jev-compaction fork).
- **ws-mcp routing is static:** model routing is a static tier lookup and ws-mcp never sees request content (`wsconfig/config.go:426`).
- **No network in ws-mcp:** ws-mcp makes no outbound network calls and handles no secrets.
- **Pi compaction is percent-only:** Pi compaction triggers use only the context percent (50% advisory, 80% hard; `goal-loop.ts:150,153,1183`).
- **Pi runaway guard is a counter:** it is a 10-strike re-fire counter (`goal-loop.ts:147,407`).

### Confirmed Decisions

- **Research only for now.** No actionable ticket is authored from this survey yet.
- **No in-place grafts onto lead judgment.** Grafting a decision model onto existing lead-judgment paths (tier selection, route facts, compaction trigger, runaway guard) is deprioritized; the user judged those paths already heavily optimized and a graft inelegant. Further exploration targets new domains.
- **The interface sketch stays deferred.** The `decision.ask` common interface is recorded as a deferred sketch, not adopted.

### Proposals

- **New domains:** the seven candidate new domains above (dogfood-surprise mining, prune-not-summarize pre-pass, intent-aware exec output shaping, CI/exec failure triage, edit-time semantic lint over project-declared rules, evidence grounding, note/rationale reranking).
- **Interface form:** if any candidate proceeds, build the backend-neutral `decision.ask` interface rather than a Jev-specific client, and start in shadow mode to collect labeled data before any decision gates behavior.

### Open Questions

- **Which candidate is best?** Which has the best value-to-egress ratio? Offline or batch candidates (1, 6) avoid the hot path entirely and may be the safest first experiment.
- **Is `llm-http` good enough?** Does a light LLM with logprob-derived distributions discriminate well enough on ws decision types, or does the substitute-model failure generalize?
- **Can Pi reach an OpenRouter key?** Is it reachable through Pi provider auth (`getProviderAuth`)?
- **What would the eval set be?** Where does labeled data come from? Candidates: shadow logs, the review ledger's follow-ups, ticket history.
- **Do lab entrants change the plan?** Do major-lab decision models, if released, change the backend choice or the case for a vendor-neutral interface?

### Rejected Alternatives

- **Jev as ws-mcp's model router.** ws-mcp has no request content to route on, and a hosted proprietary dependency in the shipped Go binary conflicts with Rules 3–4.

## Sources

- https://openrouter.ai/blog/insights/what-is-jev/
- https://openrouter.ai/blog/tutorials/jev-vs-llm-when-to-use-each/
- https://www.langchain.com/blog/building-a-harness-with-jev
- https://www.techtarget.com/it-infrastructure/news/366650696/Jev-decision-model-touted-as-quicker-cheaper-LLM-alternative
- https://www.marktechpost.com/2026/09/27/20-agentic-use-cases-of-typesafe-ais-jev/
- https://github.com/Anil-matcha/awesome-jev-by-typesafe/blob/main/docs/coding-agent-use-cases.md
- https://github.com/zhangxaochen/dsh-jev
- https://github.com/AndreaScotti01/fast-jev-compaction
- https://github.com/Servant-Software-LLC/Guardrails/issues/755
- https://aicoder.com/news/news-20260921-agent-beacon
- https://docs.typesafe.ai/introduction/coding-agents
