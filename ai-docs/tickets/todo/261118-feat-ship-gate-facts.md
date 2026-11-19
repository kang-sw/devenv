---
title: "lead-ship release gate의 기계적 판정을 Go로 옮기고 follow-up 재확인을 ship-handler로 넘기기"
parent: 261116-epic-lead-orchestration-profile
related:
  261117-refactor-playbook-compression: prerequisite - its lead-ship split (9ce62f010) is the text this ticket shortens
---

# Move the release gate's mechanical checks out of lead-ship prose

## Background

After the playbook diet, `lead-ship` is 853 words, about 370 of them the
release gate, above the epic's ~400-word budget for a lead-facing entry. The
owner observed (2026-11-18) that much of the gate is mechanical procedure.

The gate's six steps split as follows:

- Mechanical: step 1 (query review follow-ups, re-check each against `HEAD`,
  resolve fixed ones through `resolves`), step 2 (read `review.marker`'s
  `found` first), step 4's unreviewed-commit count, and step 5's cut point
  (the head of the `pass`/`concern` stamp the gate's review wrote, or its
  cleared-prefix stamp).
- User judgment or lead-only: step 3's bootstrap / review / neither choice,
  step 4's `lead-review` run (a lead skill that asks the user on `fix` or
  `judgment`, and that a worker cannot start), and every override.

The gate's prose rules each came from an incident in which a model misread one
of the mechanical steps: an empty ledger read as "0 unreviewed commits" shipped
all history unreviewed (f3ac6a20d), and a `bootstrap` seed was counted as
clearing a cut point (b517cdffc). That is why the mechanical part moves into
code rather than into a smaller-tier worker's prose.

## Decisions

- **Split shape.** Go computes the gate's mechanical facts (marker found,
  frontier head, the unreviewed range and its commit count, and the cut point
  a completed review clears) and returns them to the lead; `ship-handler`
  takes step 1's follow-up re-check; the lead keeps step 3's choice, step 4's
  `lead-review`, and every override. The track landed first and this runs as
  its own unit. (139)=a. Rejected: implementing it on track/playbook before
  landing (delays a tested unit); moving the whole gate text into
  `ship-handler` at tier medium without Go (hands the misread-prone steps to a
  weaker reader); waiting for the epic's ship child `ship-runner` (blocked on
  foundation Phase 2).

## Constraints

- Not yet settled; settle through the Open Decision Queue before `ready/`:
  whether the facts extend `review.marker`'s JSON or come from a new tool, the
  exact fields, and how the cut point is reported when no review ran. Each
  option states its compatibility tier (an additive field or tool is T0).
- The gate's behavior is preserved: found-first, only a `pass`/`concern` stamp
  clears a cut point, the gate itself calls no `review.stamp`, the marker
  moves only through bootstrap and `lead-review`.
- The facts must also serve the epic's later `ship-runner`, so the tool shape
  names no profile.
- Playbook prose is drafted by the lead with the owner (epic (09)).
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Convention: ai-docs/manuals/performance.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/)

## Phases

### Phase 1: Gate facts in Go, gate prose on top of them

Add the gate-facts surface with tests covering an empty ledger, a bootstrap
seed, a trailing block above a cleared prefix, and a zero-commit frontier.
Rewrite `lead-ship`'s release gate to branch on the returned facts, and add
the follow-up re-check to `ship-handler`; regenerate the manifests and the
wsflow and Pi mirrors.

Verification: the new Go tests pass; `go test ./internal/wsrsrc` and the
targeted `internal/mcp` playbook and render tests pass; `lead-ship`'s word
count is recorded before and after.
