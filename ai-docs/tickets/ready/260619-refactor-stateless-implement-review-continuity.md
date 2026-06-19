---
title: Stateless implement-review loop continuity via commit AI Context
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260619-research-ws-delegate-continuity-host-neutral-fallback: motivating research; its finding leaned on the mercenary path, which this ticket supersedes (mercenary is default-off/advanced). Finding needs a correction append when next touched.
  260619-research-claude-teammate-mode-subagent-collection-doc-gap: related collection-mode gap; stateless delegates reduce how load-bearing teammate-mode same-agent resume is.
---

# Stateless implement-review loop continuity via commit AI Context

## Background

The `lead-implement` review fix-cycle relays findings to the **same** implementer
across cycles and resumes it via the harness continuation idiom
(`ContinueIdiom`, e.g. `SendMessage(to: <agentId>)` for Claude;
`playbook_tools.go`). That same-agent resume is the only continuation layer that
is host-feature-gated:

- Native subagent same-agent resume needs the experimental teammate feature; even
  when present (the ws installer enables it) result collection is inconsistent
  (see `260619-research-claude-teammate-mode-subagent-collection-doc-gap`), and it
  is host-divergent (Claude `SendMessage` vs Codex resume-by-task-id).
- The mercenary path is a fully host-neutral stateful resume, but it is
  **default-off / advanced** and `ws:full-only`-gated in the dispatch text
  (`lead-implement.md` Delegate dispatch). So it cannot be the default-path
  answer.

Therefore the default delegation path (native subagent) has no reliable
host-neutral stateful continuation, only fresh-spawn with cold context — and the
relay/re-review prompts do not even state that fallback. This ticket makes the
loop correct under the default path by treating delegates as stateless and moving
continuity to the lead, anchored on the one channel an agent is structurally
compelled to write: the commit message `## AI Context`.

Scope is documentation/contract only — no runtime changes. All edits touch
skill/playbook/convention text, so they are **Ask-first + `lead-skill-authoring`
review** per AGENTS.md.

## Decisions

### D1: Delegates are stateless; continuity never depends on same-agent resume
Each implementer/reviewer dispatch is fed entirely by the relay prompt plus the
self-contained artifact set (brief, plan, review findings, committed diff). The
loop must remain correct when every cycle is a fresh spawn.
- Rejected: baking a "prepare your compaction handoff" instruction into the
  delegate prompt — it re-introduces the exact host-resume dependency this
  redesign removes, and compaction is non-deterministic host behavior.
- Same-agent resume, when a host offers it, is a pure latency optimization for
  re-reading static role context; it is never a correctness dependency.

### D2: The lead is the continuity + adjudication subject; commit `## AI Context` is the primary durable anchor
In-session continuity is the lead's compaction/transcript; cross-session
continuity is reconstructed from `git log` `## AI Context` (already the
AGENTS.md bootstrap behavior). The commit message is chosen because it is the one
record an agent must produce as a side effect of doing the work — "ride the
mandatory action".
- Rejected: a separate progress/diary doc or an MCP logging tool — both are
  extra actions that can be skipped, done lazily, or fail at runtime.

### D3: Strengthen implementer `## AI Context`, and record fix-cycle dispositions inline
Raise the *depth* (not breadth) of `## AI Context`: capture what the diff cannot
show — intent, rejected alternatives, cross-module implications, related
mental-model/spec references — keeping the existing `impl-playbook.md` scope
(non-obvious invariant/ordering/lifecycle/cross-module contract). Mechanical
"what changed" narration stays out (the diff shows it).
Additionally, the implementer records each fix-cycle disposition
(`won't-fix`/`deferred` + reason) **inline in the fix commit** that addresses the
cycle, not only in its return message, so the judgment survives to disk.

### D4: Re-review relay carries prior findings + dispositions + updated diff; the reviewer is not asked to classify findings
The re-review input gives the (fresh) reviewer the prior findings list, their
dispositions, and the updated diff, so the reviewer's existing "focus only on
whether reported issues were addressed" charter has a referent again. The
reviewer reviews the current diff per its charter and reports new issues with
severity — it is **not** asked to decide regression-vs-preexisting.
- Rejected: a monotonic "reviewer may add only regression findings" constraint —
  too fuzzy for a context-poor fresh agent and risks suppressing a genuinely
  missed Critical.

### D5: Reviewer verdict carries a severity breakdown; the lead decides "clean"
The reviewer emits a severity-explicit verdict (e.g. `clean`,
`clean with N minor remaining`, `non-clean: M critical/important`). Whether minor
items warrant another cycle is the lead's adjudication, not a machine gate.
- Rejected: a hard machine gate ("no Critical/Important == clean") — it removes
  lead judgment and contradicts D2's lead-as-adjudicator role.

### D6: Convergence is enforced at the lead by dedup against the durable disposition record
The lead does not re-relay a finding already settled (won't-fix/accepted) in a
prior cycle; it recognizes it from the disposition record (D3). Genuinely new
Critical/Important findings are still relayed; minors flow to the final report.
- This is a *semantic* "is this already settled?" judgment by the lead, so it
  needs no stable machine key — the disposition record only has to be
  human-readable (file + what + why).
- Layered defense: lead dedup handles the common churn (re-litigating settled
  items); the existing hard cap (partitioned 3 cycles / single 2, lead
  adjudicates at cycle 2, caller escalation at cycle 3) is the backstop for the
  pathological case of a reviewer inventing new distinct findings each cycle —
  where hitting the cap is then a true "contentious" signal.

## Constraints

- Runtime/code unchanged; only skill/playbook/convention text.
- Host-neutral: no reliance on `SendMessage`/teammate mode (gated) or the
  mercenary path (default-off/advanced) for correctness.
- Every edited surface is Ask-first and requires `lead-skill-authoring` invariant
  review for each changed Invariants/Constraints/Doctrine line.
- Ready-promotion will require spec addressing: the reviewer verdict format and
  loop gate are caller-visible workflow behavior.

## Prior Art

- `agents-plugin/rsrc/impl-playbook.md:12` — implementer already records
  invariants/contracts under `## AI Context -> ### Mental Model Notes`.
- `agents-plugin/rsrc/lead-review/lead-review.md:168-175` — `## AI Context`
  already treated as the intention channel for in-context analysis.
- `AGENTS.md` Project Memory step 4 — lead bootstrap restores rationale from
  `git log -10` `## AI Context`.
- `agents-plugin/rsrc/reviewer/reviewer.md` "Re-review Scope" — anti-churn
  instruction that exists but silently assumes a stateful (remembering) reviewer.
- `ai-docs/tickets/.done/260617-refactor-mcp-stateless-subagent-context.md` —
  stateless-delegate intent this builds on.

## Spec Impact

- **Target spec area**: `workflow-skills.md`, the `lead-implement` review-loop
  contract — anchor `#260612-reviewer-allocation-tier-default` (review allocation
  + relay cap) and the surrounding implementer/reviewer delegate description. That
  anchor currently states the cap mechanics (single 2 / partitioned 3, lead
  adjudication at cycle 2, caller escalation at cycle 3) but says nothing about
  delegate statelessness, the reviewer verdict shape, the re-review relay payload,
  or how convergence is enforced.
- **Expected caller-visible change**:
  - Delegates are stateless; loop continuity is lead-owned (anchored on commit
    `## AI Context`), never on same-agent resume (D1/D2).
  - The reviewer emits a severity-explicit verdict (`clean` /
    `clean with N minor remaining` / `non-clean: M critical/important`); the lead
    decides "clean" — it is not a machine gate (D5).
  - The re-review relay carries the prior findings, their dispositions, and the
    updated diff; the reviewer is not asked to classify regression-vs-preexisting
    (D4).
  - Convergence is enforced at the lead by semantic dedup against the durable
    disposition record, layered over the existing relay cap as backstop (D6).
  - The implementer records each fix-cycle disposition inline in the fix commit
    `## AI Context` (D3).
- **Contract-first spec: no.** The spec update and the three interlocking playbook
  edits must land in the same implementation slice — the ticket requires them to
  land together to avoid an incoherent intermediate, so a spec written ahead of
  the playbooks would itself be the "spec says new behavior, playbooks say old"
  intermediate this redesign avoids. `workflow-skills.md` is updated within the
  implementation slice and the implement commit carries a `## Spec` section
  naming the stem.

## Phases

### Phase 1: Land the stateless continuity contract across implement/review playbooks

Apply D1-D6 as one coherent contract change; the pieces interlock (the
disposition written by the implementer is consumed by lead dedup and fed to the
reviewer), so they must land together to avoid an incoherent intermediate.

Edits (all Ask-first + `lead-skill-authoring` review):
- `implementer.md` — D3: AI Context depth guidance; fix-cycle disposition recorded
  inline in the fix commit (Output "On fix cycle" + Process commit step).
- `reviewer.md` — D4 input expectation on re-review; D5 severity-explicit verdict.
- `lead-implement.md` — D1 stateless doctrine in Delegate dispatch; relay/re-review
  prompts carry prior findings + dispositions + updated diff and state the
  fresh-spawn fallback; D5 lead-owned clean decision; D6 lead dedup against the
  disposition record in the Review loop.

Verification boundary:
- Regenerate rsrc manifest; playbook render/freshness tests green.
- `lead-skill-authoring` invariant checklist applied to every changed
  Invariants/Constraints/Doctrine line.
- Dogfood one implement-review fix loop (with at least one won't-fix disposition)
  and confirm: the disposition lands in a commit's `## AI Context`, a fresh
  reviewer fed the relay does not re-litigate the settled item, and the lead's
  clean decision uses the severity breakdown.

Deferred:
- Correcting the `260619-research-ws-delegate-continuity-host-neutral-fallback`
  finding and cross-referencing the teammate-mode ticket — done when those
  research tickets are next touched, not part of this implementation slice.
