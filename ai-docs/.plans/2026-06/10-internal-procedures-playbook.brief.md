# Brief: internal-procedures-playbook

## Intent
Move the workflow's internal procedure bodies off the directly-invocable skill
surface and onto the M1 rsrc playbook surface, so they are served as
`playbook.print` content and executed inline by caller skills. After this slice,
the 9 internal procedures (`lead-implement`, `lead-write-ticket`,
`lead-write-spec`, `lead-workflow-manual`, `lead-check-blockers`,
`lead-verify-design`, `lead-verify-discussion`, `lead-write-skeleton`,
`lead-update-spec`) are no longer `/ws:<name>` entry points; callers retrieve and
run them via `ws/playbook.print(name: ...)`. This is the M2 Phase 2 slice of epic
260605 (playbook-factory pivot) and is skill/prompt **text** conversion only — no
runtime code is deleted (that is M3).

## Scope Boundary
- IN: (1) migrate the 9 internal skill bodies into `agents-plugin/rsrc/<stem>/<stem>.md`
  `kind: print` playbooks; (2) rewire every call site that invokes one of the 9
  (in the 11 entry skills and among the 9 themselves) to `ws/playbook.print` +
  inline execution; (3) make `lead-write-ticket`/`lead-write-spec` reachable only
  as orchestration (not direct `/ws:` entry); (4) relocate `lead-skill-authoring`'s
  invariant-audit target to the rsrc playbook sources and follow the audit
  procedure to the new home; (5) regenerate `manifest.json`; (6) keep the
  rsrc validate-tree CI gate green.
- OUT (Phase 3): reducing the 11 entry skills to thin trigger shims. Phase 2 only
  edits the specific invocation lines in entry skills, not their overall structure.
- OUT (M3): deleting the `ws/subquery` runtime tool, any `agents.*` runtime code,
  or the spawn engine; the `convention.read`/`infra.read` runtime tools stay.
- OUT (separate decision): the anchor's "implement entry routing" ticket-skip gate
  for `lead-proceed` (anchor §"Implement entry routing"). Phase 2 preserves current
  routing semantics; it changes only the invocation MECHANISM, not routing behavior.
- OUT (deferred non-scope): wsflow convergence to the rsrc-playbook model — see
  Constraints.

## Caller-Visible Contract
- The set of directly user-invocable `/ws:<name>` skills shrinks: the 9 internal
  names are no longer typed by the user. The 11 entry skills remain invocable.
- The 9 procedures' content is obtained by callers through
  `ws/playbook.print(name: "<stem>")` and executed inline in the caller's own
  context (context absorption is the accepted, intended consequence).
- Conventions needed by a procedure resolve at print time (the migrated procedure
  still has access to ticket/spec conventions when executed) — exact mechanism is
  an Implementation Strategy item below.
- Procedure-to-procedure invocation (e.g. write-ticket invoking write-spec,
  implement invoking update-spec/workflow-manual) becomes a nested
  `ws/playbook.print` call executed inline, recursively, in the same context.
- No change to the observable behavior of the procedures themselves (same steps,
  same gates, same outputs) — only how their text is delivered to the executor.

## Contract Instructions
- Playbooks live at `agents-plugin/rsrc/<stem>/<stem>.md`, `kind: print`. Follow the
  M1 frontmatter schema exactly (see `agents-plugin/rsrc/sample-playbook/sample-playbook.md`
  and `agents-plugin/rsrc/explore/explore.md`): `kind`, optional `delegates`,
  optional `includes:` (flat bare-name text deps), optional `variables:` (declared
  substitution vars). Playbook stem = the former skill name minus the `lead-`
  decision below (keep the existing `lead-<name>` stem to avoid call-site churn;
  confirm naming with the plan).
- Mark any migrated body that delegates work to a subagent with `delegates: true`
  (the always-on mercenary-tip seam from M1). Do NOT inline native-only delegation
  prose; native delegation is the DEFAULT not the EXCLUSIVE path (epic option-B
  mercenary surface stays lead-invokable in M3).
- Reuse the existing M1 mechanisms: `wsrsrc` loader, `playbook.print`/`playbook.render`
  tools, `wsrsrc.GenerateManifest` for `manifest.json`. Do NOT add a new loader,
  a new tool, or an embedded fallback copy of playbook text (the anchor forbids
  embedded fallback — loud failure only).
- Caller rewiring: every place a skill currently does `invoke ws:lead-<name>`
  (Skill-tool invocation) for one of the 9 must become
  `ws/playbook.print(name: "lead-<name>")` followed by "execute the returned
  procedure inline". The plan must enumerate every such call site exactly.
- `lead-skill-authoring`: its invariant-audit procedure currently targets
  `agents-plugin/skills/**/SKILL.md`. After migration the procedure bodies live in
  `agents-plugin/rsrc/`; update the audit target/glob and any path references so the
  fresh-reader audit runs against the rsrc playbook sources (plus the remaining
  skill files). `lead-skill-authoring` itself stays an entry skill.
- FORBIDDEN: duplicating canonical convention text (ticket-conventions,
  spec-conventions, mental-model-conventions) into rsrc in a way that creates a
  second source of truth. If auto-include is used for conventions, the rsrc text
  must be sourced from the same files `convention.read` serves (loader
  unification), not a hand-copied fork. See Implementation Strategy.
- FORBIDDEN: temporary/mock/stub procedure bodies. The migrated playbook text is
  the real procedure, semantically identical to the skill body it replaces.

## Integration Test Instructions
- Boundary: the rsrc loader + playbook tools. Extend, do not replace:
  - `agents-plugin-tool/internal/wsrsrc/wsrsrc_test.go` — the validate-real-tree /
    loader / manifest tests. Every new playbook must pass tree validation
    (required variants present, declared variables consistent, manifest hashes
    match).
  - `agents-plugin-tool/internal/mcp/playbook_tools_test.go` — `playbook.print`
    golden/behavior tests. Add at least one print test that loads a migrated
    procedure and asserts the body resolves (and, if a procedure auto-includes a
    convention dep, that the included text is present in the printed output).
- Pass criteria: `go test ./...` green from `agents-plugin-tool/`; the
  validate-real-tree test gates all new playbooks; `manifest.json` regenerated and
  consistent. Run the wsflow suite only to confirm NO wsflow drift was introduced
  (`python3 -m unittest discover agents-plugin-wsflow/tests`) — it must stay green
  because Phase 2 does not edit wsflow skills.

## Implementation Strategy Decisions
Settled (do not reopen):
- The 11/9 entry/internal keep-list is final. Classification axis: "is the user
  meant to type `/ws:<name>` directly". The 9 are not entry points after this slice.
- Wiring model: caller invokes `ws/playbook.print(name: "<stem>")` (signature is
  `name` required + optional `context` object; there is NO `session_key`/`session-id`
  argument today — M3 adds the session seam, do not pre-add it) and executes the
  returned procedure text inline in its own context.
- `lead-write-ticket`/`lead-write-spec` are orchestration-only: bodies move to
  playbook content, callers (e.g. `lead-proceed`, `lead-discuss`) invoke them via
  `playbook.print`; they are not user-typed entries.
- Disposition of the 9 SKILL.md: because there is no host mechanism to hide a skill
  from the `/ws:` menu while keeping it skill-invocable, making a procedure
  "not a directly user-invoked entry point" means its SKILL.md entry point is
  removed (its body relocated to rsrc). This is the approved Phase 2 deletion of
  the 9 internal entry points — it is the explicit phase goal, not an incidental
  deletion. (Plan confirms whether any compat stub must remain; default: remove.)
- `lead-write-skeleton` is deprecated and compat-only (kept for compatibility, not
  routed). Minimal-touch: do NOT invest in a full rsrc playbook for it unless the
  plan finds live call sites. Ensure it is not a user entry; if nothing invokes it,
  no rsrc playbook is required — confirm via the call-site survey and document the
  disposition.

OPEN — plan must resolve, escalate to research if multiple viable strategies:
- **Convention auto-inclusion mechanism.** The ticket verification says internal
  procedures resolve "with auto-included conventions". M1 provides the rsrc
  `includes:` mechanism (a `kind: text` dep concatenated at print time). The risk:
  ticket/spec conventions currently live in embedded `infra` served by
  `convention.read`; copying them into rsrc creates split-brain (anchor explicitly
  forbids embedded-fallback duplication). The plan MUST map how `convention.read`
  is implemented and decide one of:
  (a) loader unification — rsrc `kind: text` convention deps sourced from the same
      files `convention.read` serves (single source, clean auto-include), OR
  (b) procedures keep calling `ws/convention.read(name: ...)` inline at execution
      time (no rsrc convention dep, single source preserved), satisfying "available
      conventions" without rsrc duplication.
  Pick whichever preserves a single source of convention truth with the least
  cross-module disturbance; if neither is clean within Phase 2 scope, escalate to
  research. Do not hand-copy convention text.

## Rejected Alternatives
- Single print/render tool with output-kind metadata — rejected in the anchor;
  the split prevents full delegate prompts leaking into lead context.
- Embedded fallback copy of playbook/convention text — rejected (split-brain drift;
  loud failure is the chosen contract).
- Same-change wsflow mirror edit — rejected for Phase 2: wsflow has no
  `playbook.print` surface and rsrc-playbook convergence is deferred epic non-scope;
  the mirror obligation is discharged by the existing follow-up chore ticket.
- Pulling the `lead-proceed` ticket-skip gate into Phase 2 — rejected: routing
  behavior change is a separate decision, not body-migration.

## Approach
- Inventory: confirm the exact current body of each of the 9 skills and every call
  site that invokes them (entry skills + inter-procedure references).
- Convert each body to a `kind: print` rsrc playbook, preserving semantics; mark
  `delegates: true` where the body spawns subagents; declare any needed variables;
  wire convention access per the resolved OPEN decision.
- Rewrite call sites to `playbook.print` + inline execution, including nested
  procedure-to-procedure invocations.
- Make `lead-write-ticket`/`lead-write-spec` orchestration-only; remove the 9 entry
  points' SKILL.md per the disposition decision.
- Relocate the `lead-skill-authoring` audit target to rsrc.
- Regenerate `manifest.json`; extend wsrsrc + playbook_tools tests; run full Go
  suite + wsflow no-drift check.

## Constraints
- Host-neutral first: shared playbook text prefers canonical MCP tool names and
  host-neutral behavior; Claude-specifics are adapter/fallback only.
- Skill/prompt/playbook text edits require the no-prior-context fresh-reader
  invariant audit (workflow-skills #260514) — reviewer allocation covers this
  (Fit + Correctness partitions).
- rsrc text edits do NOT bump the prompt bundle hash; do NOT refresh `runtime.json`
  for rsrc-only changes — regenerate `manifest.json` only. Bump
  `SupportedSchemaVersion` only if the playbook schema shape changes (it should not).
- rsrc includes are flat (root-level bare names); no nested includes.
- All authored playbook/skill text is English.
- wsflow skills are NOT edited in this slice; the wsflow suite must stay green as a
  no-drift check.

## Out of scope
- Phase 3 entry-shim reduction; M3 runtime deletion; the `lead-proceed` ticket-skip
  gate; wsflow rsrc-playbook convergence; the `lead-proceed`/`lead-implement` user
  routing redesign beyond the mechanical invocation rewrite.

## Details
- rsrc layout precedent: `agents-plugin/rsrc/explore/explore.md` (`kind: render`,
  `delegates: true`, declared vars), `agents-plugin/rsrc/sample-playbook/sample-playbook.md`
  (`kind: print`, `includes: [sample-conventions]`, declared `WorktreeID`),
  `agents-plugin/rsrc/sample-conventions.md` (`kind: text`).
- `manifest.json` schema: `{ "schema_version": 1, "files": { "<relpath>": "<sha256>" } }`.
- `playbook.print(name, context?)`: `name` required; `context` optional
  string→string for declared vars. Reserved var names (terminology/model-alias) are
  tool-injected and cannot be overridden by `context`.
- wsflow shipped/excluded sets and forbidden-reference rules: `ai-docs/ref/wsflow-mirroring.md`.

## Verification Contract
- Every migrated procedure resolves through `ws/playbook.print(name: "<stem>")` and
  prints the full procedure body (plus auto-included conventions where applicable).
- `lead-write-ticket`/`lead-write-spec` are not reachable as direct `/ws:` entries
  (their SKILL.md entry points are gone); callers invoke them via `playbook.print`.
- The `lead-skill-authoring` invariant audit runs against rsrc playbook sources.
- `go test ./...` (from `agents-plugin-tool/`) green; validate-real-tree gates new
  playbooks; `manifest.json` regenerated.
- `python3 -m unittest discover agents-plugin-wsflow/tests` green (no wsflow drift).
- No call site references a removed `ws:lead-<name>` skill invocation for the 9.

## References
<!-- [Must] entries: read before starting. [Maybe] entries: consult if uncertain. -->
- `ai-docs/mental-model/prompt-bundle.md` - [Must] rsrc loader, playbook.print/render, manifest regen, includes/variables, delegates tip, M3 root/session seam.
- `ai-docs/mental-model/workflow-skills.md` - [Must] entry/internal skill surface, fresh-reader invariant audit, skill-authoring conventions, lead-write-skeleton deprecation.
- `ai-docs/ref/wsflow-mirroring.md` - [Must] which of the 9 are wsflow-mirrored (8 of 9) and why Phase 2 uses a follow-up ticket not a same-change edit.
- `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` - [Must] binding decisions: skill surface reduction, rsrc-as-plugin-text, keep-list, convention loader-unification direction, mercenary seam.
- `ai-docs/spec/workflow-skills.md` - [Must] `{#260610-entry-skill-surface-reduction}` planned contract (stays 🚧 until Phase 3 also lands).
- `ai-docs/spec/mcp-tools.md` - [Maybe] `{#260609-playbook-tools}` / `{#260609-rsrc-playbook-distribution}` print/render + rsrc contracts.
- `ai-docs/mental-model/named-agent-runtime.md` - [Maybe] delegation/subagent lifecycle context for delegates:true bodies.
- `ai-docs/tickets/idea/260610-chore-wsflow-explore-playbook-mirroring.md` - [Maybe] the follow-up ticket that already scopes "Coordinate with M2 Phase 2".
