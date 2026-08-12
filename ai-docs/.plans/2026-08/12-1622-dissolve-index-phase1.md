# Plan: 260807-refactor-dissolve-project-index — Phase 1: Author the versioned lead-bootstrap dissolution step, validated on devenv

## Relevant Ticket Contract

- Dissolution is a `lead-bootstrap` change, not a one-off devenv edit: rewrite both faces of `AGENTS.template.md` (session-start `## Project Memory` read step; fresh-bootstrap scaffold) plus a new migration-checklist item, then validate by running the upgrade on devenv itself.
- Delivery rides the existing version-gate: one new checklist item per package (no new version-gating/alarm mechanism).
- Dual-distribution mirroring is mandatory: every template/procedure edit lands identically (behaviorally) in both `agents-plugin/` and `agents-plugin-wsflow/`; each package keeps its own independent version-tag lineage (ws: v0045→v0046; wsflow: v0006→v0007 — confirmed by reading both templates' `<!-- Template Version -->` tags, not assumed).
- Per-region disposition is exhaustive and settled (ticket `## Decisions`): volatile/tracked Session Notes → `repo` note layer (qualitative staleness pruning, no automated mechanism); every-session orientation (repo identity, plugin topology, canonical flows, doc-system routing) → `AGENTS.md` body; procedures + the `## Read Before Editing` table → `manuals/` (table replaced by the already-landed generated `# Manuals` ambient index); derivable tables (ticket/spec inventory) → generated (`project_tree`); `## Runtime Surfaces`/`## MCP Runtime Notes`/`## Prompt And Agent Inventory`/`## Skill Inventory`/`## Current Branch Rules` → drop as duplicate or fold one pointer line into AGENTS.md, per-region rule in ticket body.
- Convergence invariant: fresh-bootstrap and upgrade-migration must reach the same `AGENTS.md` shape, neither with `_index.md`.
- Absorbed 260725 scope: clear residual `Ticket Focus` references from devenv's own `AGENTS.md` and `ai-docs/WORKFLOW.md` via regeneration, not hand-edit. Section-placement gotcha: the shipped checklist item's own hint text says `## Project Knowledge`, but devenv's real bullet lives under `## Ticket System`.
- Spec Impact (planned, not necessarily hand-edited by this implementer — the doc pre-pass owns `spec/` edits): `spec/documentation-system.md {#260505-project-memory-index}` and `spec/workflow-skills.md` bootstrap section both currently describe `_index.md` as canonical and must describe the dissolved model plus transitional coexistence.
- Verification boundary (from ticket): `ai-docs/_index.md` gone from devenv; template neither creates nor reads `_index.md` on fresh or upgrade; no shipped skill/playbook/convention/AGENTS.md step/spec entry instructs reading it or declares it canonical; a fresh session with no manual file reads still gets the orientation it previously got from `_index.md`.

## Out of Scope

- Phase 2 (un-migrated-downstream coexistence contract, hard-dependency guard, spec documentation of the transitional state) — separate phase, not touched here beyond the graceful-coexistence behavior Phase 1's edits must not break.
- Actually authoring `spec/documentation-system.md` / `spec/workflow-skills.md` prose — noted for the doc pre-pass, not planned as an implementation step here.
- `260728-research-index-ticket-table-drift` (`idea/`, motivates only, not a build input).
- Any change to note-tool mechanism, staleness-threshold tooling, or reconciliation automation — Resolved Decision explicitly rejects building this; migration is one-time qualitative judgment.

## Codebase Findings

- `agents-plugin/skills/lead-bootstrap/AGENTS.template.md` — canonical ws template.
  - `#L3-L9` `## Project Memory` read step (step 1 reads `_index.md`) — rewrite target.
  - `#L73-L81` `## Project Knowledge` — currently has no repo-identity/topology/canonical-flows content; new "every-session orientation" placeholder section belongs near here (new heading, e.g. `## Project Orientation`, inserted before `## Project Knowledge`).
  - `#L83-L117` fresh-bootstrap `<!-- MIGRATION: ... -->` scaffold block — lists `_index.md` as the memory store; must stop instructing fresh-project `_index.md` creation and instead point at the new homes.
  - `#L123-L204` `<!-- MIGRATION CHECKLIST -->`; last entry `v0045` ends at `#L203`; insert new `v0046` item before the closing `-->` at `#L204`.
  - `#L206` `<!-- Template Version: v0045 -->` → bump to `v0046`.
- `agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md` — wsflow mirror, **independent version lineage** (confirmed: currently `v0006`, not synced 1:1 with ws numbers — e.g. ws's `v0044`/`v0045` map to wsflow's `v0005`/`v0006`).
  - `#L3-L10` `## Project Memory` (has an extra step-4 `git log -10` line the ws copy dropped at its own `v0043` — pre-existing wsflow-specific drift, out of scope for this ticket, do not fix incidentally).
  - `#L74-L82` `## Project Knowledge`, mirror insertion point for the new orientation section (uses "wsflow runtime"/"wsflow MCP parser" wording per existing pattern).
  - `#L84-L117` fresh-bootstrap scaffold block, same rewrite as ws copy.
  - `#L124-L176` migration checklist; last entry `v0006` ends `#L175`; insert new `v0007` item before `#L176` closing `-->`.
  - `#L178` `<!-- Template Version: v0006 -->` → bump to `v0007`.
- `agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md` and `agents-plugin-wsflow/rsrc/lead-bootstrap/lead-bootstrap.md` are **byte-identical** (confirmed via `diff`, no output) — edit the ws copy then copy verbatim to the wsflow path (no host-specific wording in this file; `{{.SkillNamespace}}` handles substitution).
  - `#L37` `## On: fresh` step 1 "Copy template to `AGENTS.md`, stripping template-internal migration blocks." — no direct `_index.md` mention; fine once the template's own scaffold block stops naming it. Step 3 "Create `ai-docs/` structure per the template setup block." inherits the rewritten scaffold automatically.
  - `#L76-L97` `## On: index health check` — already conditional on `ai-docs/_index.md` existing (`## On: invoke` step 6, `#L33`); this is already the Phase 2 coexistence behavior and needs no structural change, only confirm wording doesn't imply `_index.md` is still the default/expected state.
- `agents-plugin/skills/lead-bootstrap/WORKFLOW.md` (master guide template) — **also documents `_index.md` as the memory store** and is not named in the ticket body but is required by the Phase 1 verification bar ("no ... `AGENTS.md` step, or spec entry instructs reading it or declares it canonical" — `WORKFLOW.md` is installed, agent-readable guidance with the same problem). Needs the same redistribution:
  - `#L13-L21` `## Authority Files` — `_index.md` pointer.
  - `#L22-L39` `## `ai-docs/` Layout` — `_index.md` bullets (`#L24`, `#L26`).
  - `#L87-L124` `## Index Health` — whole section describes `_index.md` scope-drift cleanup as the *only* project-memory maintenance step; must add/point to the new note/manuals/AGENTS.md homes, or be reframed as coexistence-only guidance.
  - `#L136-L146` `## Manual Fallback` step 1 reads `AGENTS.md`, `_index.md`, this guide — drop `_index.md`.
  - Compare against `agents-plugin-wsflow/skills/lead-bootstrap/WORKFLOW.md` (diff confirmed: wording-only differences, `ws`→`wsflow`/`AI agents and automation` — same structural sections at the same relative locations); mirror identically.
  - **Evidence this is currently drifting, not hypothetical**: the master ws `WORKFLOW.md#L41-L52` `## Tickets` section already has **no** `Ticket Focus` bullet (post-260710 sync), but devenv's installed `ai-docs/WORKFLOW.md#L47-L49` still has one — proof that devenv's installed copy is stale relative to the master template and that a lead-bootstrap upgrade run (which only "preserve[s] project additions, only merge[s] missing bootstrap semantics" per `lead-bootstrap.md#L54`) does not automatically strip stale content; devenv's `WORKFLOW.md` needs an explicit re-sync/regeneration pass during the devenv validation step, not just relying on the generic upgrade merge.
- `AGENTS.md` (devenv root, `v0041`) — `#L199` under `## Ticket System` (not `## Project Knowledge`) carries `Check '## Ticket Focus' in 'ai-docs/_index.md'...` — this is the section-placement gotcha the ticket calls out; the shipped `v0044` item's own condition text (`AGENTS.template.md#L188`) says to remove it from `## Project Knowledge`, so a literal-section-scoped regen run on devenv would miss it. The devenv validation step must locate and remove this bullet by content match, not by trusting the `v0044` item's stated section.
- `ai-docs/WORKFLOW.md` (devenv installed copy) — `#L47-L49`, `#L109`, `#L122` all reference `_index.md ## Ticket Focus`; clear via re-sync from the (rewritten) master template, not hand-edit.
- `ai-docs/_index.md` (devenv, 239 lines) — full redistribution map for the devenv validation step:
  - `#L6-L19` `## Repo` → orientation → new `AGENTS.md` section.
  - `#L21-L26` `## Current Branch Rules` → drop per Decision (codex-untracked rule already in `AGENTS.md` Commit Rules `#L121-L122`; branch-verify line moot; no-freeze line volatile/drop).
  - `#L28-L56` `## Plugin Topology` → orientation → new `AGENTS.md` section.
  - `#L58-L97` `## Read Before Editing` → table rows replaced by the already-landed generated `# Manuals` ambient index; the regen-command bash block (`#L86-L97`) is **already fully duplicated** in `ai-docs/manuals/wsflow-mirroring.md#L241,264-266` (confirmed) — drop, no new manuals write needed.
  - `#L99-L107` `## Runtime Surfaces` → fold the one non-duplicate line ("schemas are runtime-discoverable...") into AGENTS.md orientation if not already covered by `AGENTS.md#L153-L164 ## Documentation System`; rest drops as duplicate.
  - `#L109-L119` `## MCP Runtime Notes` → the Windows launcher startup steps are **already present verbatim in substance** at `ai-docs/manuals/ws-mcp.md#L88-L91` (confirmed) — drop entire section, no new manuals content needed.
  - `#L121-L130` `## Prompt And Agent Inventory` / `## Skill Inventory` → drop as source-derivable, optional one-line AGENTS.md pointer.
  - `#L132-L144` `## Canonical Flows` → orientation → new `AGENTS.md` section.
  - `#L146-L157` `## Specs` table, `#L159-L212` `## Tickets` table → derivable → drop (generated by `project_tree`).
  - `#L214-L239` `## Session Notes` → migrate via `ws/note.write(session_key, layer:"repo", notes:[{key,value,priority}])` (writes git-tracked `ai-docs/ws-notes/<key>.md`, confirmed tool schema) with qualitative staleness pruning — execution-time judgment call per Resolved Decision, not pre-decided here (the "Closeout: 260625 Phase 2" entry reads as historically stale and a pruning candidate; the "Open: verify Codex hook..." line reads as still-live — implementer confirms against current state at execution time, not from this survey).
- `ai-docs/spec/documentation-system.md#L13-L18` `## Project Memory Index {#260505-project-memory-index}` — current text declares `_index.md` "the project memory and active inventory document"; contradicts the dissolved model. Doc pre-pass target, not this implementer's edit per task framing.
- `ai-docs/spec/workflow-skills.md#L1178-L1200` — bootstrap section documents the `_index.md` health-check as the whole memory-maintenance story with no dissolution/note/manuals wording. Doc pre-pass target.
- Note-tool mechanism confirmed via tool schema: `mcp__plugin_ws_ws__note_write(session_key, layer, notes[{key,value,priority}])`, `layer: "repo"` → `ai-docs/ws-notes/<key>.md`, git-tracked, staged through the caller's normal `git.commit`. This is the landed destination for tracked Session Notes (prerequisite `260810-feat-repo-tracked-note-layer`, confirmed landed at ws 0.40.2-dev per `_index.md#L15-16` and ticket frontmatter).

## Implementation Plan

1. **ws `AGENTS.template.md`** (`agents-plugin/skills/lead-bootstrap/AGENTS.template.md`):
   a. Rewrite `## Project Memory` step 1 (`#L7`) to point at the new homes instead of reading `_index.md` (note layers for volatile/tracked state, `manuals/` for procedures, generated output for inventories, `AGENTS.md` body itself for orientation) — keep step 2 (`_index.local.md`, still valid as machine-local memory unless separately dissolved — out of scope here) and step 3 (`git log`) as-is.
   b. Add a new `## Project Orientation` (or similarly-named) section before `## Project Knowledge` (`#L73`) carrying placeholder subsections for repo identity, project map/plugin topology, and canonical flows — generic placeholders (`[Project-specific]`-style), matching the template's existing placeholder convention (e.g. `[Project-specific rule]` at `#L25`).
   c. Rewrite the `<!-- MIGRATION: ... -->` fresh-bootstrap scaffold block (`#L83-L117`): remove `_index.md` from the `ai-docs/` layout listing as a creation target (or reduce its role to optional/legacy), and replace the "`_index.md` should cover project summary..." instructions with guidance to populate the new `## Project Orientation` (or equivalent) section in `AGENTS.md` directly, plus pointers to `manuals/`/notes/generated tables for the other categories.
   d. Insert new checklist item `v0046` after `v0045` (`#L203`, before `#L204`): directs an upgrading project to migrate `_index.md`'s resident orientation into `AGENTS.md`, move `# Session Notes` → `repo` note layer via `note.write` with qualitative staleness pruning, procedures → `manuals/`, derivable tables → generated, remove the read-`_index.md` step, delete the file. Cross-reference the ticket's exhaustive per-region disposition table so the item text doesn't have to repeat every region rule.
   e. Bump `<!-- Template Version: v0045 -->` (`#L206`) → `v0046`.
2. **wsflow `AGENTS.template.md`** (`agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md`): mirror step 1's edits with wsflow's own version numbers — new checklist item as `v0007` (after `v0006` at `#L175`, before `#L176`), bump tag `#L178` `v0006` → `v0007`, same `## Project Orientation`/scaffold rewrite with "wsflow" wording per the file's existing `ws`→`wsflow` substitution pattern (confirmed via `diff`: only wording tokens differ, not structure).
3. **`lead-bootstrap.md`** (`agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md`): update `## On: fresh` step 1-3 wording so it's explicit the copied scaffold no longer creates `_index.md` (falls out of step 1c/2 above automatically once the template scaffold block changes, but confirm the procedure text doesn't separately assert `_index.md` creation anywhere else in the file — none found in this read). Confirm `## On: index health check` / `## On: user approves index cleanup` sections read correctly as coexistence-only (Phase 2) behavior post-dissolution — no structural edit expected, re-read after step 1 lands to confirm no stale implication that `_index.md` is the default state.
4. **Copy `lead-bootstrap.md` to the wsflow path** (`agents-plugin-wsflow/rsrc/lead-bootstrap/lead-bootstrap.md`) verbatim — confirmed byte-identical today, no host-specific tokens to adjust.
5. **`WORKFLOW.md` master template**, both `agents-plugin/skills/lead-bootstrap/WORKFLOW.md` and `agents-plugin-wsflow/skills/lead-bootstrap/WORKFLOW.md`: apply the same redistribution — drop/rewrite `_index.md` references in `## Authority Files` (`#L13-21`), `## `ai-docs/` Layout` (`#L22-39`), `## Index Health` (`#L87-124`, reframe as Phase-2-only coexistence guidance or point at the new homes), and `## Manual Fallback` step 1 (`#L136-146`). Mirror wording-only differences per the existing `ws`/`wsflow` pattern (confirmed via `diff`).
6. **Regenerate/commit the wsrsrc manifests** per `AGENTS.md#L83-97` (devenv's own Read Before Editing procedure): `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest` then `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror` from `agents-plugin-tool/`, since `lead-bootstrap.md` is a canonical `agents-plugin/rsrc/` playbook. `AGENTS.template.md`/`WORKFLOW.md` are `skills/`-tree files (not `rsrc/`), not covered by this regen — confirm during execution whether the skills-tree manifest (`agents-plugin/skills/manifest.json`) needs its own regen for a `skills/lead-bootstrap/*` non-`SKILL.md` asset change (only `SKILL.md` hashing is confirmed in the mental model; verify `AGENTS.template.md`/`WORKFLOW.md` aren't separately hashed before assuming no regen is needed).
7. **Validate on devenv** — run the new upgrade item against devenv itself as the real downstream target:
   a. Migrate `## Repo`, `## Plugin Topology`, `## Canonical Flows` content from `ai-docs/_index.md` into devenv's own `AGENTS.md` under the new orientation section.
   b. Migrate `## Session Notes` via `note.write(layer:"repo", ...)`, qualitatively pruning stale entries (judgment call at execution time per the finding above).
   c. Drop `## Current Branch Rules`, `## Read Before Editing`, `## Runtime Surfaces`, `## MCP Runtime Notes`, `## Prompt And Agent Inventory`, `## Skill Inventory`, `## Specs`, `## Tickets` per the disposition table (all confirmed duplicate/derivable/already-homed — see Codebase Findings).
   d. Delete `ai-docs/_index.md`.
   e. Update devenv's `AGENTS.md#L153-164 ## Documentation System` bullet "Project memory and focus: `ai-docs/_index.md`" to point at the new homes.
   f. Remove the residual `Check '## Ticket Focus'...` bullet from devenv `AGENTS.md#L199` (under `## Ticket System`, not `## Project Knowledge` — the section-placement gotcha) — locate by content match, not by the checklist item's stated section.
   g. Re-sync devenv's `ai-docs/WORKFLOW.md` from the (now-rewritten) master `WORKFLOW.md` template, clearing the stale `Ticket Focus`/`_index.md` references at `#L47-49,109,122` — this is a content re-sync/regeneration, not a targeted hand-edit of just those lines, consistent with the ticket's "never a hand-edit" instruction.
   h. Bump devenv's `<!-- Template Version: v0041 -->` to `v0046`, applying every intervening checklist item (`v0042`-`v0046`) in order, not just the new one — devenv has never run `v0042`-`v0045` (confirmed: devenv is at `v0041`, template is at `v0045`).
8. Report the two Spec Impact targets (`spec/documentation-system.md {#260505-project-memory-index}`, `spec/workflow-skills.md` bootstrap section) as pending for the doc pre-pass; do not hand-edit them as part of this implementer's plan execution unless the doc pre-pass explicitly delegates them here.

## Verification Plan

- Structural, no compiled-code test suite: `grep -rn "_index.md" agents-plugin/skills/lead-bootstrap/ agents-plugin-wsflow/skills/lead-bootstrap/` should show no remaining creation/read instruction (template-internal migration-checklist historical mentions of `_index.md` in past items, e.g. `v0039`-`v0041`, are expected and fine — they describe history, not live behavior).
- `grep -rn "_index.md" AGENTS.md ai-docs/WORKFLOW.md` (devenv root) should return nothing live (no hits, or only inert historical/comment text if any survives — expect none).
- `grep -rni "ticket focus" .` repo-wide should return only immutable migration-history checklist entries (`v0041`/`v0004` items), `CHANGELOG.md`, and ticket bodies — no live reader instruction in `AGENTS.md`/`WORKFLOW.md` (ticket's own stated verification bar).
- Confirm `ai-docs/_index.md` no longer exists in devenv (`ls ai-docs/_index.md` fails).
- Confirm devenv `<!-- Template Version -->` tag reads `v0046`.
- `diff` the two `AGENTS.template.md` migration-checklist new-item text and the two `WORKFLOW.md` files to confirm wording-only (not structural) divergence remains, matching the existing pattern.
- Re-run the two wsrsrc regen commands from `agents-plugin-tool/` (see Implementation step 6) and confirm no diff / clean `go test` output for `lead-bootstrap.md` mirror parity.
- `python3 -m unittest discover agents-plugin-wsflow/tests` (per devenv's own Read Before Editing gate) to catch wsflow package-level drift from the `lead-bootstrap.md`/template edits.
- Manual/structural: confirm a fresh session with only `AGENTS.md` in context (no `_index.md` read) still has repo identity, plugin topology, and canonical flows available — i.e. the new orientation section actually landed in devenv's `AGENTS.md` body with real content, not left as a placeholder.

## Escalations

- None blocking. Two lead rulings recorded here as binding authority for the
  implementer and reviewers (both resolved during survey adjudication, 2026-08-12):

  1. **WORKFLOW.md is in scope (confirmed).** Although Phase 1's bullets name only
     `AGENTS.template.md` and `lead-bootstrap.md`, the master `WORKFLOW.md`
     template (both `agents-plugin/` and `agents-plugin-wsflow/`) and devenv's own
     installed `ai-docs/WORKFLOW.md` ARE in scope. They are shipped, agent-readable
     bootstrap guidance that documents `_index.md` as the memory store, so leaving
     them violates the ticket's own verification bar ("no shipped skill, playbook,
     convention, AGENTS.md step, or spec entry instructs reading it or declares it
     canonical") and the convergence invariant (a fresh-bootstrapped project would
     otherwise receive a WORKFLOW.md pointing at a dissolved `_index.md`). Same
     surface class as the already-in-scope `AGENTS.template.md`; the ticket
     Background itself names WORKFLOW.md. Apply the same redistribution per plan
     step 5 / devenv step 7g.

  2. **devenv version bump v0041 -> v0046 is honest; no unrelated migration is
     pulled in (confirmed by inspection).** The intervening items are all no-op or
     already-in-scope for devenv: v0042/v0043 (`## Project Memory` git-log step
     cleanup) are no-op — devenv's step 3 is already `git log --oneline --graph -50`
     with no `git log -10`/`-20` step 4; v0044 (remove the `Check '## Ticket Focus'`
     reader bullet) IS this ticket's 260725-absorbed cleanup (plan step 7f); v0045
     (retire spec `🚧` planned markers) is no-op — `grep -rn "🚧" ai-docs/spec/`
     returns nothing in devenv. So step 7h applies v0042-v0046 in order with only
     the dissolution (v0046) and the Ticket-Focus cleanup (v0044) doing real work,
     and the tag legitimately reaches v0046. Do NOT expand scope hunting for other
     v0042-v0045 effects — they were verified empty here.
