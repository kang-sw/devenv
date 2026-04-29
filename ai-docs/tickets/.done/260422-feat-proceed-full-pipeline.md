---
title: "Extend /proceed to full-pipeline routing"
related:
  260422-feat-write-ticket-review: interaction — auto-invoking /write-ticket will trigger the mandatory document-reviewer step when that feature lands
spec:
  - 260422-proceed-full-pipeline-routing
---

# Extend /proceed to full-pipeline routing

## Background

Currently `/proceed` is invoked after `/write-ticket` and routes to skeleton, plan,
and implementation stages. In practice, users invoke `/proceed` immediately after
`/discuss` (or mid-conversation with no explicit ticket path) and expect it to handle
the full pipeline.

The change makes `/proceed` a true universal router: two new judges fire before the
existing implementation judges, routing to `/write-spec` and `/write-ticket` as needed.
This collapses the canonical chain from `/discuss → /write-spec → /write-ticket → /proceed`
to `/discuss → /proceed`.

## Decisions

**`judge: needs-spec` always delegates to `/write-spec`'s own gate.** `/proceed` does
not pre-judge spec-impact; it always invokes `/write-spec`, which exits immediately via
its own `judge: spec-impact` if no spec work is needed. This applies regardless of
whether the ticket has a `spec:` frontmatter field or whether a spec entry already
exists — /write-spec's gate is the authoritative judge in both cases. Cost: `/write-spec`
is invoked even when no spec work is needed, but its gate is fast and the invocation
is explicit.

Rejected: a pre-check on ticket `spec:` frontmatter or spec files before invoking
/write-spec — adds surface area in /proceed and duplicates logic /write-spec already
handles internally.

**`judge: needs-ticket` behavior change.** The "vague idea" case (clear direction,
unclear scope) changes from stop/suggest to auto-invoke `/write-ticket`. `/write-ticket`
handles scope clarification internally. After `/write-ticket` completes, `/proceed`
captures the ticket path from its output and uses it as the target for all downstream
stages, including the downstream context-passing in the Execute step (same pattern as
/write-plan path passing).

The "exploratory" case (unclear direction, not requesting implementation) still stops
and routes to `/discuss` — there is nothing to write a ticket about yet. This floor
is intentional and must not be changed.

**New judges are prefix stages, not routing table rows.** The existing routing table
(skeleton/plan/implementation selection) is preserved unchanged. The two new judges
add prefix steps that invoke upstream skills and continue; they do not produce new
branching in the downstream table.

## Phases

### Phase 1: Update proceed/SKILL.md and chain references

### Result (2e92011) - 2026-04-22

All four target files updated. `/proceed` now fires `judge: needs-spec` unconditionally before `judge: needs-ticket`, collapsing the canonical chain to `/discuss → /proceed`. `write-ticket` step 8 emits `Ticket:` completion artifact. `_index.md` and `workflow-skills.md` updated to reflect the simplified chain. `workflow-routing.md` mental-model domain added. CLAUDE.md architecture rule added requiring skill-authoring.md compliance on all plugin document edits.

Deviations: implementation required multiple fix rounds for skill-authoring.md compliance (prose rules, judge placement, invariant rationale removal). The `judge: needs-spec` Judgments entry was added then removed — unconditional judges belong in the Route handler only, not Judgments.

**Changes to `claude/skills/proceed/SKILL.md`:**

1. In the **Assess** step, extend fact-gathering to include:
   - Whether the target is exploratory vs. actionable (currently determined inside
     `judge: needs-ticket`; lifting the signal into Assess makes it available earlier).

2. Add `judge: needs-spec` in the **Route** step, before `judge: needs-ticket`:
   - Always invoke `/write-spec`; its own `judge: spec-impact` handles the no-op exit.
   - Continue to `judge: needs-ticket` regardless of whether /write-spec wrote anything.

3. Update `judge: needs-ticket` in the **Route** step:
   - "Vague idea" row: change from stop/suggest to auto-invoke `/write-ticket` →
     capture the produced ticket path from /write-ticket's output → continue with
     that path as the target.
   - "Exploratory" row: unchanged — still stops and routes to `/discuss`.
   - "Clear scope" row: unchanged.

4. Update the **Route** step by adding a routing prefix above the existing numbered
   judgment list:
   ```
   judge: needs-spec (fires first) → judge: needs-ticket → [existing pipeline judges]
   ```

5. Update the **Execute** step (step 4) to extend downstream context-passing:
   - After a `judge: needs-ticket` auto-invoke, capture the ticket path from
     `/write-ticket`'s output. Use it as the target for skeleton, plan, and
     implementation stages — the same pattern as the existing plan-path capture
     (`"if /write-plan produces a plan path, pass it to /implement"`).

6. Update the **Announce** step to include spec and ticket stages when they fire.
   The pipeline line is based on /proceed's decision to invoke each stage, not on
   the post-hoc outcome (i.e., show `/write-spec` in the pipeline line even if its
   gate exits without writing):
   - Spec fires + ticket fires: `## Pipeline: /write-spec → /write-ticket → <implementation stages>`
   - Spec fires only: `## Pipeline: /write-spec → <implementation stages>`
   - Ticket fires only: `## Pipeline: /write-ticket → <implementation stages>`
   - Neither fires: existing announce format unchanged.
   - Direct-edit: existing announce format unchanged.

7. Update the **Doctrine** section: replace "routing accuracy under session-warmth
   awareness" with language that captures universal-router scope — /proceed now spans
   the full canonical chain, not just implementation stages. Preserve the
   "announce → delegate" and "warmth improves briefing" principles.

**Changes to `claude/skills/write-ticket/SKILL.md`:**
- In the **On: invoke** step 8 (Proceed prompt), add: after suggesting `/proceed`,
  also emit the created ticket path as a completion artifact so `/proceed` can capture
  it when invoking `/write-ticket` as a prefix stage. This mirrors the plan-path
  emission in `/write-plan/SKILL.md`.

**Note on `260422-write-ticket-document-review`:** once that feature lands,
auto-invoking `/write-ticket` from `/proceed` will trigger a mandatory document-reviewer
inside /write-ticket. This is correct behavior — the reviewer runs as part of the
ticket-creation gate regardless of caller. No skip or bypass.

**Changes to `ai-docs/_index.md`:**
- Canonical Flows section: simplify full-ceremony chain to `/discuss → /proceed`.

**Changes to `ai-docs/spec/workflow-skills.md`:**
- Update the Canonical Workflow Chain section chain diagram from
  `/discuss → /write-spec → /write-ticket → /proceed` to `/discuss → /proceed`.
- Update the `/proceed` feature body prose to describe the new full-pipeline routing
  as implemented behavior (including the new judges and their behavior).
- Strip `> [!note] Planned 🚧` callout at `{#260422-proceed-full-pipeline-routing}`.
- In the `/proceed` body, replace "no spec entry exists" (from the planned callout)
  with accurate language: routes to `/write-spec`, which determines internally whether
  spec work is needed.

Constraint: the "exploratory" stop case in `judge: needs-ticket` must not be changed
to auto-invoke — it is the deliberate floor that prevents /proceed from routing when
there is genuinely nothing to implement.

Success:
- `/proceed` invoked mid-discussion (no ticket) invokes `/write-spec` (which exits
  via its own gate if no spec work is needed), then auto-invokes `/write-ticket`,
  then proceeds to implementation routing.
- `/proceed` invoked with an existing ticket path (no new spec/ticket work needed):
  `/write-spec` is invoked and exits immediately, ticket judge passes, downstream
  routing table behavior is unchanged.
- `/proceed` invoked on an exploratory discussion stops and routes to `/discuss`.
- After prefix stages complete, downstream routing selects `/edit` / `/implement` /
  `/parallel-implement` correctly (no regression in existing routing table behavior).
- The ticket path produced by auto-invoked `/write-ticket` is passed through to
  skeleton, plan, and implementation stages.
- Announce output matches the multi-stage pipeline format when prefix stages fire;
  existing announce format is used when neither prefix stage fires.
