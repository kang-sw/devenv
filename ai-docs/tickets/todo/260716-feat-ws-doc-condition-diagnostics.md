---
title: Hidden doc-condition diagnostics — verification crawl, consumption counters, workflow health metrics
sage-review-design: blocked
related:
  260716-feat-mental-model-openup-injection: consumer — injection telemetry rides this substrate and its landing is gated on this ticket
---

# Hidden doc-condition diagnostics — verification crawl, consumption counters, workflow health metrics

## Background

A 2026-07-16 lead-discuss evaluation (three downstream repos: libhbs,
InspectTGV_AIDriven, PipelineDevProj; ~60 sampled mental-model claims verified
against source) established that spec and mental-model docs are accurate and
high-value where sampled, but three measurement gaps remain:

1. **No verification loop for doc truth.** Zero drift was found in samples,
   but time-dependent claims ("no callers (grep-verified)", compiler-version
   workarounds) are pinned to nothing that would detect drift later.
2. **Consumption is unmeasured.** Docs are reachable only through
   `specs.find`/`mental_models.find` discovery (no static load path), so
   "doc X is read when relevant" cannot be audited today.
3. **Workflow health is vibe-checked.** Ratios that matter (docs:code commit
   trend, sage block rate) are derivable from git history but never computed;
   the devenv docs ratio drifted 42.8% → 55.3% (recent 150 commits) without
   anyone noticing.

## Decisions

- The surface is a **hidden developer diagnostic** ("doc condition check"),
  reachable through `ws:lead-tune` (or an equivalent lead-only entry), not
  advertised in default playbook guidance. Output is advisory/non-blocking.
- **Verification crawl is a freshness checker, not a generator.** The manual
  closeout capture loop is retained: regeneration would destroy the
  non-derivable trap/rationale content that the 3-repo audit showed dominates
  mental-model value (A-class 40-70%). Rejected alternative: post-crawl doc
  regeneration.
- **Counters accumulate to a machine-local file** (not git-tracked, in the
  spirit of `_index.local.md` / cache-dir state): per-doc call/hit counts for
  `specs.find` and `mental_models.find`, extensible to injection hit/miss
  telemetry from `260716-feat-mental-model-openup-injection`.
- Rejected alternative: RAG/embedding retrieval — the active corpus is small
  (~100k words), agents already have grep + discovery tools; the bottleneck is
  salience and invalidation, which retrieval infra does not address.

## Phases

### Phase 1: Consumption counters substrate

Per-doc invocation/hit counters for `specs.find` and `mental_models.find`,
persisted to a machine-local file keyed by doc stem. A lead-only read surface
reports counts (raw dump is acceptable for the first slice). Include a
zero-consumption view (docs never surfaced over the recorded window) as the
demotion-candidate signal. Verification: counters increment across separate
MCP sessions; file survives restart; no counter I/O on the hot path fails a
tool call (best-effort writes).

### Phase 2: Doc condition check — verification crawl playbook

A lead-tune-gated diagnostic procedure that dispatches cheap exploration
subagents to sample N checkable claims per selected spec/mental-model doc and
verify them against source, reporting per-claim verdicts
(accurate / stale / unverifiable) with file:line evidence. Drift findings
route to `idea/` tickets through the normal capture flow rather than editing
docs in place. Verification: run against this repo and confirm the report
format; seed with the claim-sampling method used in the 2026-07-16 audit.

### Phase 3: Workflow health metrics

Computed view over git history: docs:code commit ratio trend (monthly), sage
verdict distribution (pass/concern/block), drift findings count from Phase 2
runs. Script or MCP-computed — implementation may choose; caller-visible
output is a compact table. Verification: numbers reproduce the hand-computed
2026-07-16 baseline (42.8% full-history / 55.3% recent-150 docs ratio) within
rounding.

## Blocked (2026-07-16)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Phase 3 sage verdict distribution has no source matching its stated vocabulary | important | missing |
| 2 | Phase 3 docs-vs-code classification method is unspecified | minor | autonomous |
