---
title: "Single session-loaded ticket-system concept doc; playbook references instead of re-glossing"
related:
  260624-feat-tickets-template-tool-and-convention-diet: precedent — moved type templates out of the convention doc; this addresses the conceptual gap that offloading left
  260702-research-destructive-dedup-methodology: guardrail-vs-restatement discipline bounds what the concept doc may absorb
  260723-feat-ticket-write-verify-commit-gate: sibling — once verify owns mechanical guardrails, the doc carries only concepts
parent: 260723-epic-ticket-write-reshape
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-07-23
---

# Single session-loaded ticket-system concept doc; playbook references instead of re-glossing

## Background

Audit of the ticket-write chain found the concepts are **procedural and
scattered**, not explained once:

- Status dirs (`idea/`/`todo/`/`ready/`), spec addressing, phases, and
  epic/workset semantics are each explained 2-3 times across
  `ticket-conventions.md`, `lead-write-ticket.md` judge tables, and Go constants —
  in different vocabularies (declarative rule vs. judge-decision criterion vs.
  bare mechanical call).
- **Type prefixes** (`feat`/`bug`/`refactor`/`chore`) are never semantically
  distinguished anywhere — just listed flatly.
- **Sage review** has zero conceptual explanation in any doc; its rationale and
  posture semantics live only as Go code comments, invisible to a reading agent.

This is the residue of the token-saving diet: pushing text into Go tools reduced
per-invocation tokens but evaporated or duplicated the conceptual home. Each
decision point re-derives concept meaning, which is itself part of the weight.

## Decisions

- **One layered-explanation concept doc** covering: what each status dir means and
  when to use it; the semantic distinction between type prefixes; what sage review
  is, why it exists, and what its postures mean; what spec addressing is for; the
  phase model; epic vs. workset.
- **Type-prefix distinction is categorization guidance only.** The workflow treats
  `feat`/`bug`/`refactor`/`chore` identically (all "actionable, phased" per
  `judge: ticket-category`); there is no mechanical divergence. The doc gives
  plain-word "which prefix fits" guidance and must **not** imply behavioral
  differences that do not exist.
- **Home and loading: the `workflow_manual` bundle (decided).** The concept doc
  is a bundled runtime doc under the workflow-manual surface
  (`rsrc/lead-workflow-manual/`), grounded through `ws/workflow_manual` at session
  bootstrap — the call every session already makes. The bundle is dual-maintained
  (`agents-plugin/rsrc/` and `agents-plugin-wsflow/rsrc/`, kept in sync); edit
  both trees. Chosen over AGENTS.md
  mandatory-reading (host-specific; against host-neutral-first) and over on-demand
  load (per-invocation, which defeats the amortization the ticket is built on).
  Because `workflow_manual` is session-once, the doc is loaded once and the
  playbook sheds its inline re-glosses.
- **Concepts only, never guardrails.** Any invariant that `ticket.verify`
  (`260723-feat-ticket-write-verify-commit-gate`) can mechanically enforce stays in
  verify; the doc must not dissolve a hard guardrail into soft prose the model may
  not honor (per `260702`'s guardrail-vs-restatement caution).
- The playbook and convention doc **reference** the concept doc for meaning and
  keep only the mechanical call sequence + hard invariants.
- **Go constants stay the mechanical source of truth.** The template/checklist/sage
  Go constants (`agents-plugin-tool/internal/wsdoc/tickets_template.go`,
  `tickets_checklist.go`, `tickets_sage.go`) are not touched by this
  consolidation; the concept doc explains *meaning*, the Go constants remain the
  *mechanical* content, and verify owns *enforcement*.

### Inline vs referenced (open knob, not a blocker)

Within the `workflow_manual` home, one tunable remains — resolvable during
implementation, does not block ready:

- **Recommended: tight inline.** The manual payload carries the layered concept
  definitions kept lean, so grounding is guaranteed and the net effect is
  removing the scattered per-write glosses.
- **Fallback: referenced.** If the inline concept text bloats the always-on
  manual payload unacceptably, degrade to the manual *pointing* to a bundled
  concept doc that the ticket-write path pulls — accepting that this reintroduces
  a per-invocation load for ticket work.

Measure the manual-payload delta during Phase 1 and pick accordingly.

## Phases

### Phase 1: Author the concept doc and de-duplicate the glosses

Write the single concept doc covering all six areas named in Decisions. Then
strip the duplicated conceptual glosses, replacing them with a reference to the
concept doc — removing only *restatements*, never a guardrail verify() does not
yet own. Per-file scope:

- `agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md` — the
  status-dir / type-prefix / spec-addressing / phase / epic-vs-workset
  *explanatory* prose (keep the mechanical rules and hard invariants).
- `lead-write-ticket.md` — the `judge:` decision tables that re-gloss concept
  meaning (e.g. `ticket-category`, `initial-status`, `spec-address-gate`), which
  keep their *decision criteria* but drop the concept re-explanation.

**High-risk step — gloss removal.** Deciding "distinct guardrail vs. pure
restatement" for each removed line is exactly the destructive-dedup failure mode
`260702-research-destructive-dedup-methodology` documents, and that ticket records
the per-merge audit (guardrail-vs-restatement in both directions, flow-position
preservation) as **not yet codified** — a user previously caught two silent
misses. Until 260702 lands a procedure, treat the gloss-removal diff as the
highest-risk part of this phase and give it close line-level review, not a
methodology this ticket can assume exists.

**Acceptance check (measurement boundary stated).** Confirm the concept doc
covers all six areas listed in Decisions (status-dir semantics, type-prefix
distinctions, sage-review rationale/postures, spec addressing, phase model,
epic-vs-workset). Compare the token count of the write-ticket base path
(`lead-write-ticket.md` + `ticket-conventions.md`) before vs. after the gloss
removal; confirm a net reduction on *that* path. State
the boundary explicitly: the win is per-write-invocation gloss savings, while the
concept doc adds a once-per-session baseline via the `workflow_manual` payload —
so amortization clearly wins for multi-ticket sessions, and a session that never
touches tickets pays that baseline for no gloss savings (bounded by keeping the
inline concept text lean, or by the referenced fallback). Also confirm no hard
invariant was softened in the move (each removed line is a restatement, not a
guardrail).

### Result (c1d1b5a0) - 2026-07-23

Authored a `## Ticket System Concepts` section (six subsections: status
directories, type prefixes, sage review, spec addressing, phases, epic-vs-workset)
in the `lead-workflow-manual` bundle and de-duplicated the scattered conceptual
glosses. Home decision: **tight inline** (measured manual delta 180→270 lines /
+53%, judged proportionate as a once-per-session cost; the `native-spawn-binding`
`includes:` referenced pattern was available but not needed). Sage-review posture
semantics were sourced solely from `tickets_sage.go`; type-prefix content is a
pure addition (mechanically-identical framing, no invented divergence) since no
prior gloss existed for it anywhere.

- **Dual-tree mechanism.** Edited only canonical `agents-plugin/rsrc/`, then
  regenerated the `agents-plugin-wsflow/rsrc/` mirror and both shipped manifests
  via `WS_REGEN_WSFLOW_RSRC=1` / `WS_REGEN_MANIFEST=1` Go test modes — never
  hand-edited the mirror (deviation from the ticket's literal "edit both trees";
  the real contract is generate-then-commit, guarded by
  `TestWsflowRsrcMirrorUpToDate`).
- **Gloss removal (highest-risk step).** Pruned only pure meaning restatements
  from `ticket-conventions.md` (status/epic/workset/phase definition prose) and
  the `lead-write-ticket.md` `judge: ticket-category` + `judge: initial-status`
  tables, each replaced by a pointer to the concept section. Unenforced hard
  invariants that merely *read* as prose were kept verbatim — phase numbers never
  renumbered, Result/Edition frozen once written, worksets never change `parent:`,
  epics/worksets do not use implementation phases. Confirmed against
  `tickets_verify.go` / `tickets_mutate.go` enforcement surfaces.
- **Acceptance boundary — honest result.** The write-ticket base path
  (`lead-write-ticket.md` + `ticket-conventions.md`) is effectively flat
  (2693→2673 words, ≈ −0.7%): a net reduction in letter, but the real win is
  structural (concept meaning now lives once in the manual; sage-review and
  type-prefix gain a documented home they never had) rather than raw size, because
  small glosses plus pointer text roughly cancel. All six areas confirmed present.

Verification: `go build ./...`, golden tests
(`TestPlaybookPrintGoldenLeadWriteTicket`, `...LeadWorkflowManual`,
`...ScopedExplorationTierModels`), `TestWsflowRsrcMirrorUpToDate`, shipped-manifest
drift guards, and `internal/wsdoc` all green. Review (partitioned): correctness
**clean** (guardrail-vs-restatement audit traced every pruned line — no invariant
dropped or softened; 2 cosmetic word-drops that survive in kept lines); test
**clean** (no test files touched, golden assertions intact, manifest/mirror
consistent); fit found one important (`judge: initial-status` — a named de-dup
target left undone — plus an over-promising epic pointer) closed in `c1d1b5a0`.
Spec closeout under `documentation-system {#260723-ticket-system-concept-grounding}`
with a workflow_manual cross-reference in `mcp-tools`.

Go constants (`tickets_template.go`, `tickets_checklist.go`, `tickets_sage.go`)
untouched — the concept doc explains meaning, Go stays the mechanical source,
verify owns enforcement.

## Spec Impact

- Target spec area: `documentation-system` (the concept doc as a grounding
  artifact) plus `workflow-manual` / `mcp-tools` (the `ws/workflow_manual` payload
  gains the concept content, so its documented output changes).
- Expected caller-visible change: `workflow_manual` surfaces ticket-system
  concepts once per session; convention + playbook stop re-glossing.
- Contract-first spec: no — the doc's content is authored during implementation;
  the spec entry is a closeout describing the new grounding artifact's role and
  the `workflow_manual` payload change.


## Resolution (2026-07-23)

Phase 1 landed. Added a "## Ticket System Concepts" section (status dirs, type prefixes, sage review, spec addressing, phases, epic-vs-workset) to the lead-workflow-manual bundle as tight-inline session-once grounding, and de-duplicated the scattered conceptual glosses from ticket-conventions.md and the lead-write-ticket.md judge tables — keeping all unenforced hard invariants verbatim, guardrails in ticket.verify, and Go constants as the mechanical source. Dual-tree mirror + manifests regenerated (not hand-edited). Base-path size effectively flat; the real win is structural (single concept home; first-ever documentation of sage-review rationale and type-prefix categorization). Spec closeout under documentation-system 260723-ticket-system-concept-grounding. Shipped in ws 0.35.3. Review clean (correctness/test) after closing one fit important (judge:initial-status de-dup) + minor.
