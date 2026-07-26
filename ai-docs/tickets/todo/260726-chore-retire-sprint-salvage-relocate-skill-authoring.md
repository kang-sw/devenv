---
title: "Retire lead-sprint and lead-salvage; relocate lead-skill-authoring out of the distribution surface"
sage-review-design: required
related:
  260630-epic-skill-playbook-diet: supersedes this epic's lead-sprint diet target and lead-salvage out-of-scope entry
  260605-research-ws-native-subagent-pivot: migration anchor; entry-skill surface shrinks
  260708-feat-lead-revive-hook-replacement: references both retired skills
  260703-chore-review-delegates-true-classification: references both retired skills
  260626-research-playbook-print-lead-surface-leak: references both retired skills
  260702-research-destructive-dedup-methodology: references lead-skill-authoring as method source
spec-remove:
  - 260505-sprint-session-container
  - 260513-wsflow-sprint-skill
  - 260510-salvage-recovery-workflow-skill
related-mental-model:
  - workflow-skills
---

# Retire lead-sprint and lead-salvage; relocate lead-skill-authoring out of the distribution surface

## Background

Three `lead-*` skills carry no inbound routing from any other live skill or
playbook body, but each occupies the shipped distribution surface and the
user-invocable `/ws:<name>` trigger list:

- `lead-sprint` (155-line body) — an episode-oriented session shell. Its
  distinguishing feature, the `sprint/` branch container, was already removed by
  `260523-refactor-lead-sprint-episode-shell`; what remains routes real work back
  out to `lead-proceed`/`lead-implement`.
- `lead-salvage` (212-line body) — premise-collapse recovery. No recorded
  invocation; its ticket-writing half already delegates to `lead-write-ticket`.
- `lead-skill-authoring` (146-line body) — this is a different case. It is not
  unused: it is the authoring and audit manual for *this* repository's own
  skills and playbooks, and `AGENTS.md` binds it as a mandatory pre-edit read.
  It is upstream maintenance documentation that is currently shipped downstream
  as an invocable skill, which is a packaging error rather than a dead skill.

Grep across `agents-plugin/{rsrc,skills}` and `agents-plugin-wsflow/{rsrc,skills}`
finds **zero** references to any of the three from any other skill or playbook
body. This is the same evidence bar the diet epic used to authorize the
`lead-verify-design` deletion on 2026-07-20.

## Decisions

- **`lead-sprint`: delete.** Remove the skill shim, the rsrc playbook body, the
  wsflow shim, the wsflow rsrc body, and both manifest entries. Its sprint-edit
  micro-edit episode gets no replacement and is not reintroduced elsewhere. No
  successor route was decided; `lead-implement` direct-edit mode is the nearest
  existing path, but nothing in this ticket redirects callers to it.
- **`lead-salvage`: delete.** Same surfaces. It has no wsflow skill shim (only a
  mirrored rsrc body), so its wsflow footprint is one file plus a manifest entry.
- **`lead-skill-authoring`: relocate, do not delete.** The body content survives
  verbatim; only its *distribution* is removed. The rsrc playbook body — not the
  9-line SKILL.md shim — is the substance and becomes the relocated document
  under `ai-docs/ref/`. `_index.md` references it under an auditing-oriented
  heading (e.g. "On auditing skill/playbook content") so a session looking for
  authoring rules still finds it without a plugin round-trip.
- **UNCONFIRMED — disposition of the `enter.sprint` and `enter.salvage` MCP
  tools.** Not decided by the user; Phase 1 must not execute this step until it
  is. Deleting the skills strands both tools: each is a static mode switch with
  no resolver, no facts, and no verdict, and each has exactly one caller, which
  this ticket removes. Two readings are live — (a) retire them, so the public MCP
  schema does not carry two tools no procedure can reach; (b) keep the handlers
  as CLI-testable primitives, the same argument that preserved `format: "json"`
  in `260726-feat-enter-verdict-scenario-output`, and record them as caller-less.
  Phase 1's tool-removal step assumes (a) and is gated on confirmation.
  Registration sites if (a): dispatch `server.go:559-562`, schemas
  `server.go:3488` and `:3502`, handlers `session_state.go:1129-1135`.
- **Phase boundaries follow blast radius, not skill count.** The two deletions
  share identical mechanics and land together; the relocation is separate because
  it must repoint a binding `AGENTS.md` invariant in the same commit that moves
  the file.

## Constraints

- **`AGENTS.md` binds `lead-skill-authoring` as a hard invariant in three
  places** — Code Standards rule 5 (`AGENTS.md:79`), the Documentation System
  index (`:160`), and the pre-edit read list (`:169`). All three name the path
  `agents-plugin/skills/lead-skill-authoring/SKILL.md`. These must be repointed to
  the new `ai-docs/ref/` path in the same commit that removes the file; a phase
  that deletes the skill and defers the `AGENTS.md` edit leaves the repository's
  own root rules pointing at a missing file.
- **`skills_mirror.go` carries `lead-salvage` and `lead-skill-authoring` in its
  `disqualifyingTokens` denylist**
  (`agents-plugin-tool/internal/wsrsrc/skills_mirror.go:36-37`).
  That list gates *substitution-mirrored generation by content*, not distribution,
  so the entries do not block deletion. Decide explicitly whether to remove them:
  removing them newly permits mirroring of any future skill body that mentions the
  retired names, which is inert only while no such body exists. Recommendation:
  leave both entries in place as inert defensive guards and record why.
- **Mental-model bullets carry stable `{#slug}` anchors.** Follow the
  `lead-verify-design` precedent: mark removed rather than silently deleting an
  anchored bullet, so anchor references elsewhere resolve to an explicit tombstone.
- **`mental-model/workflow-skills.md:73` states "The four core lead skills call
  one `enter.*` tool each"** and enumerates all four. After Phase 1 this is two.
  This bullet is load-bearing for `enter.*` call-site placement and must be
  rewritten, not just trimmed.
- **`CHANGELOG.md` is append-only history.** It mentions all three names; do not
  rewrite historical entries.
- **`.done/` tickets and `ai-docs/.plans/` are historical artifacts.** They are
  the bulk of the raw grep hit count (62/42/78 total hits) and are explicitly out
  of the cascade scope.
- **Version bump on dev-merge** runs through
  `agents-plugin-tool/scripts/bump-ws-version.sh`; never hand-edit edition points.

## Prior Art

- `lead-verify-design` deletion, executed 2026-07-20 under
  `260630-epic-skill-playbook-diet`, plan
  `ai-docs/.plans/2026-07/20-1520-verify-design-diet.md`. It establishes the full
  removal sweep this ticket repeats: rsrc bodies, wsflow shim, manifests, Go
  golden test, wsflow inventory test, and spec/mental-model/`wsflow-mirroring`/
  `_index.md` reference updates.
- `260523-refactor-lead-sprint-episode-shell` already removed sprint's branch
  container, which is why the remaining body is a thin router.

## Spec Impact

Target area: `ai-docs/spec/workflow-skills.md`.

Removed stems (declared in `spec-remove:`):

- `260505-sprint-session-container` — the whole `## Sprint Session Shell` section
  (`:936`).
- `260513-wsflow-sprint-skill` — the wsflow sprint mirror paragraph (`:213-225`).
- `260510-salvage-recovery-workflow-skill` — the two salvage paragraphs
  (`:401-416`).

Edited, not removed:

- The skill-name list (`:30-48`) loses three entries.
- The invocable-surface sentence (`:54-58`) drops from 15 entry skills to 12 and
  loses `lead-sprint`, `lead-salvage`, and `lead-skill-authoring` from the
  `/ws:<name>` enumeration.
- `260513-wsflow-agentless-skill-surface` (`:201-211`) loses `lead-sprint` from
  the shipped list; its exclusion sentence (`:227-229`) reduces to
  `lead-write-skeleton` alone once the other two names no longer exist.
- The documentation-closure sentence (`:782`) loses its `lead-sprint` clause.
- `260514-skill-authoring-carried-context` (`:199`) must state where the
  authoring manual now lives.

Caller-visible change: three `/ws:<name>` entry points and two MCP tools
disappear from the public surface. Contract-first spec: no — this removes
existing documented behavior rather than introducing new contract.

## Phases

### Phase 1: Retire lead-sprint and lead-salvage

Delete both skills and their `enter.*` tools, leaving the tree green.

Surfaces to remove:

- `agents-plugin/skills/lead-sprint/SKILL.md`,
  `agents-plugin/skills/lead-salvage/SKILL.md`
- `agents-plugin/rsrc/lead-sprint/`, `agents-plugin/rsrc/lead-salvage/`
- `agents-plugin-wsflow/skills/lead-sprint/`,
  `agents-plugin-wsflow/rsrc/lead-sprint/`, `agents-plugin-wsflow/rsrc/lead-salvage/`
- Manifest entries in `agents-plugin/skills/manifest.json`,
  `agents-plugin/rsrc/manifest.json`, `agents-plugin-wsflow/rsrc/manifest.json`
  (regenerate; do not hand-edit hashes)
- `handleEnterSprint`, `handleEnterSalvage`, `deriveSprintTodos`,
  `deriveSalvageTodos` in `agents-plugin-tool/internal/mcp/session_state.go`
- The `enter.sprint` / `enter.salvage` schema blocks in
  `agents-plugin-tool/internal/mcp/server.go` and their dispatch entries
- Assertions in `agents-plugin-tool/internal/mcp/playbook_tools_test.go` and
  `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`

Spec and mental-model edits for these two skills land in this phase, on contact:
the three removed spec stems, the skill-name and invocable-surface lists, the
wsflow shipped list, the `:782` documentation-closure clause, and the
`{#260625-ws-session-state-machine}` "four core lead skills" bullet.

Verification: `go test ./...` in `agents-plugin-tool`, the wsflow bundle test,
`ws/spec_index_verify`, and a repo grep confirming no live (non-`.done/`,
non-`.plans/`) reference to either name survives outside the ticket graph, which
Phase 3 handles.

### Phase 2: Relocate lead-skill-authoring to ai-docs/ref/

Move the content out of the distribution surface without losing it.

- Move `agents-plugin/rsrc/lead-skill-authoring/lead-skill-authoring.md` to
  `ai-docs/ref/`. Strip the `kind: print` frontmatter, which is playbook-serving
  metadata with no meaning outside rsrc. Preserve the body otherwise verbatim;
  this phase is a relocation, not a rewrite.
- Delete `agents-plugin/skills/lead-skill-authoring/` (SKILL.md and
  `agents/openai.yaml`), `agents-plugin/rsrc/lead-skill-authoring/`,
  `agents-plugin-wsflow/rsrc/lead-skill-authoring/`, the two `defaultPrompt`
  strings in `agents-plugin/.codex-plugin/plugin.json` (`:28-29` — these are
  Codex prompt suggestions, not a skill registration entry), and all three
  manifest entries.
- Repoint all three `AGENTS.md` sites (`:79`, `:160`, `:169`) at the new path in
  this same commit.
- Add the `_index.md` reference under an auditing-oriented heading — the
  document is now found by reading, not by invoking a skill, so the pointer must
  say when to read it.
- Update `{#260514-skill-authoring-carried-context}` and the
  `260513-wsflow-agentless-skill-surface` exclusion sentence.

Verification: same test set as Phase 1, plus a check that no live file references
the old plugin path.

### Phase 3: Ticket-graph cascade and closing sweep

Update every live ticket that names a retired skill, then verify globally.

Live tickets to reconcile:

- `todo/260630-epic-skill-playbook-diet` — remove `lead-sprint` from the Phase 3/4
  curated target list and `lead-salvage` from Out of Scope; record that this
  ticket superseded both entries rather than deleting them silently.
- `todo/260708-feat-lead-revive-hook-replacement`
- `todo/260703-chore-review-delegates-true-classification`
- `todo/260702-research-destructive-dedup-methodology`
- `todo/260722-feat-goal-run-autonomy-posture`
- `todo/260716-feat-mental-model-comment-placement-rule`
- `todo/260620-bug-ws-delegate-playbook-output-language-unbound`
- `idea/260605-research-ws-native-subagent-pivot`
- `idea/260626-research-playbook-print-lead-surface-leak`

Also sweep `ai-docs/ref/wsflow-mirroring.md` and `ai-docs/ref/codex-integration.md`.

Closing verification: `ws/references_trace`, `ws/spec_index_verify`, full Go and
wsflow test runs, and a final grep whose only surviving hits are `CHANGELOG.md`,
`.done/` tickets, `.plans/`, and this ticket.

## Out of Scope

- The remaining `260630-epic-skill-playbook-diet` targets (`lead-write-spec`,
  `lead-add-rule`, `lead-workflow-manual`). This ticket removes two of that
  epic's entries; it does not execute the rest of the diet.
- `260726-feat-enter-verdict-scenario-output`. That ticket already scopes
  `enter.sprint`/`enter.salvage` as out of scope for verdict-scenario work; this
  ticket removes them outright, so the two are compatible and must not be
  sequenced against each other.
- Rewriting the relocated `lead-skill-authoring` body. Content revision is a
  separate concern from packaging.
- Any replacement for sprint-edit or salvage workflows.
