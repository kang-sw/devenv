# Plan: 260807-feat-manuals-doc-tier — Phase 2: bootstrap migration of ref/ and inline _index procedures into manuals/

## Relevant Ticket Contract

- Move procedure-shaped docs out of `ai-docs/ref/` into `ai-docs/manuals/`. Migration
  source is `ai-docs/_index.md`'s "Read Before Editing" table: each row's
  description becomes the moved manual's `summary:` frontmatter line.
- Named candidates: `ref/skill-authoring.md`, `ref/wsflow-mirroring.md`,
  `ref/codex-integration.md`, `ref/ws-mcp.md`, `ref/windows-dogfood.md`,
  `ref/ws-agent-runtime.md`. Static/historical refs with no applicability
  description (e.g. `ref/claude-home-legacy.md`) stay in `ref/`.
- Update `agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md` (and its wsflow
  mirror) so its ref-handling routing step sends new procedure/manual-shaped
  docs to `ai-docs/manuals/`, not `ai-docs/ref/`.
- Verification boundary: every named `_index.md` row resolves to a file under
  `ai-docs/manuals/` with `summary:` equal to that row's description and
  appears in the injected `# Manuals` block; no procedure doc meeting the
  manuals boundary remains under `ref/`; `lead-bootstrap` (+ mirror) names
  `manuals/` as the destination.
- Phase depends on Phase 1 (shipped, `b7ec4e29`): `wsdoc.ManualsList`/`ManualsFind`,
  `computeManuals` ambient injection, `manuals.list`/`manuals.find` tools all
  exist and are untouched by this phase.

## Out of Scope

- Dissolving `ai-docs/_index.md` itself (separate epic child;
  `260807-refactor-dissolve-project-index.md`, `todo`). This phase only drains
  the 6 named rows' path targets — the table rows stay, pointing at the new
  location.
- Any change to Phase 1's `ManualsList`/`computeManuals`/`manuals.*` tool code —
  it already handles an arbitrary flat `ai-docs/manuals/*.md` set correctly.
- `ai-docs/ref/agent-harness-capability-tiers.md`, `ref/claude-home-legacy.md`,
  `ref/design.md`, `ref/verify-dashboard-archive-recovery.sh`,
  `ref/worktree-ticket-scope.md` — none have a `_index.md` Read-Before-Editing
  row, so per the ticket's migration-source rule they stay in `ref/`.
- `ai-docs/ship/ws.md` and the `ws/infra.read(...)` entries also listed in the
  `_index.md` table — not `ref/` paths, not named by the ticket.
- Historical surfaces: `CHANGELOG.md`, anything under `ai-docs/tickets/.done/`,
  `ai-docs/.plans/`, and other open tickets' body text that merely *mentions*
  a `ref/` path in passing (e.g. `260807-refactor-dissolve-project-index.md`,
  `260605-research-ws-native-subagent-pivot.md`) — these are point-in-time
  records, not living cross-reference surfaces, and are not touched.

## Codebase Findings

### The 6 candidates, confirmed present in both `ref/` and the `_index.md` table

- `ai-docs/_index.md#L62-L67` — exact table rows; each description is the
  literal `summary:` text to write:
  - `ai-docs/ref/skill-authoring.md` → "On auditing skill/agent/prompt/convention content — authoring rules and invariant checklist"
  - `ai-docs/ref/wsflow-mirroring.md` → "Required before editing full ws skills, shared `agents-plugin/rsrc/` playbooks, or plugin surfaces that may need wsflow mirrors"
  - `ai-docs/ref/codex-integration.md` → "Probed Codex CLI behavior"
  - `ai-docs/ref/ws-mcp.md` → "MCP operational runbook, launcher environment, release and verification steps"
  - `ai-docs/ref/windows-dogfood.md` → "Native-Windows source-build dogfood / Phase C cold-load acceptance procedure"
  - `ai-docs/ref/ws-agent-runtime.md` → "Durable agent runtime contract"
- None of the 6 source files currently carry any frontmatter (`head -8` on
  each shows a bare `# Title` as line 1) — this is a clean prepend, not a merge.
- `ai-docs/manuals/` does not exist yet on disk (`ls` fails) — expected steady
  state; `wsdoc.ManualsList` (`agents-plugin-tool/internal/wsdoc/manuals.go#L27-L34`)
  returns `(nil, nil)` for a missing dir, not an error, so this is safe.

### Internal cross-references between the moved docs (must repoint, not just move)

- `ai-docs/ref/skill-authoring.md#L132` — links to `ai-docs/ref/wsflow-mirroring.md`.
- `ai-docs/ref/codex-integration.md#L36` — links to `ai-docs/ref/skill-authoring.md`.
- `ai-docs/ref/codex-integration.md#L101` — links to `ai-docs/ref/ws-mcp.md`.
- (Line numbers shift by however many frontmatter lines are prepended; locate
  by content, not by number, when editing.)

### Living-surface reference fanout (must repoint; grep confirmed each)

- `AGENTS.md#L79,L160,L161,L164,L169` — 4 distinct pointers: skill-authoring
  (×3), codex-integration (×1), ws-mcp (×1). `AGENTS.md#L152,L159` mention
  `ai-docs/ref/` generically (the retained-ref-tier description) — leave as-is.
- `ai-docs/_index.md#L62-L67` (table path cells only — keep descriptions),
  `#L75` (skill-authoring prose pointer), `#L79` (wsflow-mirroring prose
  pointer), `#L113` (ws-mcp prose pointer).
- `ai-docs/spec/plugin-runtime.md#L112` — wsflow-mirroring pointer.
- `ai-docs/spec/workflow-skills.md#L218` — skill-authoring pointer.
- `ai-docs/spec/documentation-system.md#L205-L211` — the Phase-1-authored
  paragraph literally states "Migrating existing `ai-docs/ref/`/`ai-docs/_index.md`
  procedure content into this tier is a separate, later phase; this tier ships
  with zero manuals in this repository until that migration lands." This
  sentence becomes false the moment this phase lands — it is a required
  accuracy fix, not optional polish, since Phase 2 directly falsifies it.
- `ai-docs/mental-model/workflow-skills.md#L35` — skill-authoring pointer (also
  references the earlier `260726` relocation; note the further move).
- `ai-docs/mental-model/workflow-skills.md#L117` — wsflow-mirroring pointer.
- `ai-docs/mental-model/prompt-bundle.md#L75` — wsflow-mirroring pointer.
- `ai-docs/mental-model/mcp-runtime.md#L125` — ws-mcp pointer.
- `ai-docs/mental-model/plugin-runtime.md#L41` — ws-mcp pointer.
- `ai-docs/mental-model/documentation-system.md#L40,L74` — generic `ref/` tier
  description, does not name any of the 6 files; leave unchanged.
- `ai-docs/spec/mcp-tools.md#L466` — references `ref/worktree-ticket-scope.md`,
  which is NOT a candidate (stays in ref); no change.
- `agents-plugin/rsrc/lead-scope-worktree/lead-scope-worktree.md#L14-L15,L28` and
  `agents-plugin-tool/internal/mcp/scope_announcement.go#L35` — reference
  `ref/worktree-ticket-scope.md`, also not a candidate; no change.
- Go source comments (accuracy fix, not test-breaking):
  `agents-plugin-tool/internal/mcp/session_state.go#L574` (skill-authoring),
  `agents-plugin-tool/internal/wsdoc/tickets_sage.go#L100` (skill-authoring),
  `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go#L14` (wsflow-mirroring),
  `agents-plugin-tool/internal/wsrsrc/skills_compose_test.go#L12` (wsflow-mirroring),
  `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L2767` (skill-authoring).
  `agents-plugin-tool/internal/wsdoc/project_tree_test.go#L15` uses
  `"ai-docs/ref/guide.md"` as an unrelated synthetic fixture path — do not touch.

### `lead-bootstrap` ref-handling routing table

- `agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md#L86-L96` is the
  "index health check" second-pass routing table (`## On: index health check`
  step 7). `#L91` is the only row that currently mentions `ai-docs/ref/`:
  `| Static reference material | Compact to \`ai-docs/ref/\` or API-doc pointers |`.
  No row currently exists for procedure/how-to content. This is the concrete
  "ref-handling step" the ticket means.
- `agents-plugin-wsflow/rsrc/lead-bootstrap/lead-bootstrap.md` is currently
  **byte-identical** to the canonical file (confirmed via `diff`, no output) —
  it is a generated mirror per `ai-docs/ref/wsflow-mirroring.md`
  (post-migration: `ai-docs/manuals/wsflow-mirroring.md`). Never hand-edit it;
  edit the canonical file only and regenerate.
- `agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md` step 3 in `## On: fresh`
  ("Create `ai-docs/` structure per the template setup block") and
  `AGENTS.template.md`/`WORKFLOW.md` in the same skill dir have **no**
  `ai-docs/ref/` or `manuals` mentions (grep confirmed empty) — no change
  needed there; this migration is repo-content-only, not a template-scaffold
  change.

### Shipped-rsrc regen requirement (risk signal — prior phases hit this)

- `ai-docs/_index.md#L83-L97` — **any** edit to canonical `agents-plugin/rsrc/`
  content is incomplete until, from `agents-plugin-tool/`:
  ```bash
  WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest
  WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror
  ```
  Both `-count=1` are mandatory (env-gated test bodies, cache returns stale
  green `ok` otherwise). This regenerates `agents-plugin/rsrc/manifest.json`,
  `agents-plugin/skills/manifest.json`, and the wsflow
  `lead-bootstrap.md` mirror — do not hand-edit any of the three.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L2929-L2946` —
  `TestPlaybookPrintGoldenLeadBootstrap` asserts substring presence
  (`"idempotent downstream migration"`, absence of `"Continuity tip"`), not a
  full-body hash — editing the routing table row does not break this specific
  test, but `TestValidateRealTree`
  (`agents-plugin-tool/internal/wsrsrc/wsrsrc_test.go#L888`) and
  `TestWsflowRsrcMirrorUpToDate`
  (`agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go#L54`) will fail
  until the regen commands run.

### Frontmatter format and a non-obvious parser constraint

- `agents-plugin-tool/internal/wsdoc/frontmatter.go#L19-L60` — simple custom
  parser, not real YAML: `---` delimited, `key: value` lines, first `:` splits
  key/value. `wsdoc.ManualInfo` (`manuals.go#L13-L16`) reads only the
  `summary` key.
- `frontmatter.go#L75-L86` (`cleanScalar`) — **truncates the value at the first
  literal `" #"` (space + hash)**, treating it as an inline YAML comment, and
  strips a fully-matched leading/trailing quote pair. None of the 6 table
  descriptions contain `" #"` or wrapping quotes, so a direct copy of each
  description as the `summary:` value is safe — but this is a real constraint
  to keep in mind if wording is touched during the move (do not introduce a
  ` #` sequence).
- Frontmatter block to prepend, verbatim shape:
  ```
  ---
  summary: <exact _index.md row description>
  ---

  ```
  (blank line before the existing `# Title`, matching the file's existing
  structure.)

### computeManuals rendering (for verification only, not modified)

- `agents-plugin-tool/internal/mcp/manuals_announcement.go#L22-L36` — renders
  `- <path> — <summary>` per manual, sorted by path (`manuals.go#L52`); a
  missing `summary:` renders an explicit no-summary marker instead of being
  dropped. All 6 manuals will get a real summary, so no marker case applies
  here.
- `agents-plugin-tool/internal/mcp/manuals_workflow_manual_test.go`,
  `manuals_announcement_test.go`, `agents-plugin-tool/internal/wsdoc/manuals_test.go`
  all use synthetic temp-dir fixtures (`WS_RSRC_ROOT` env override or an
  isolated root), not the real `ai-docs/manuals/` tree — populating real
  manuals does not perturb these tests.

## Implementation Plan

1. Create `ai-docs/manuals/` and `git mv` each of the 6 files from `ai-docs/ref/`
   into it, keeping the same basename:
   `skill-authoring.md`, `wsflow-mirroring.md`, `codex-integration.md`,
   `ws-mcp.md`, `windows-dogfood.md`, `ws-agent-runtime.md`.
2. Prepend the `---\nsummary: <row description>\n---\n\n` frontmatter block
   (verbatim descriptions listed in Codebase Findings) to each moved file,
   ahead of its existing `# Title` line.
3. Fix the 3 internal cross-references between moved docs, in their new
   locations (find by content, not stale line number):
   `manuals/skill-authoring.md` → `manuals/wsflow-mirroring.md`;
   `manuals/codex-integration.md` → `manuals/skill-authoring.md` and
   → `manuals/ws-mcp.md`.
4. Update `ai-docs/_index.md`: the 6 table path cells (`#L62-L67`, path column
   only — keep the description column text unchanged) and the 3 prose
   pointers (`#L75`, `#L79`, `#L113`) from `ai-docs/ref/<name>` to
   `ai-docs/manuals/<name>`.
5. Update `AGENTS.md` at `#L79`, `#L160`, `#L161`, `#L164`, `#L169` from
   `ai-docs/ref/<name>` to `ai-docs/manuals/<name>`. Leave `#L152`, `#L159`
   (generic `ref/` tier mentions) unchanged.
6. Update the 2 spec pointers: `ai-docs/spec/plugin-runtime.md#L112`
   (wsflow-mirroring) and `ai-docs/spec/workflow-skills.md#L218`
   (skill-authoring).
7. Fix the now-stale claim in `ai-docs/spec/documentation-system.md#L205-L211`:
   remove or rewrite "this tier ships with zero manuals in this repository
   until that migration lands" — the migration has landed as of this phase.
8. Update the 5 mental-model pointers: `ai-docs/mental-model/workflow-skills.md#L35,L117`,
   `ai-docs/mental-model/prompt-bundle.md#L75`,
   `ai-docs/mental-model/mcp-runtime.md#L125`,
   `ai-docs/mental-model/plugin-runtime.md#L41`.
9. Update the Go/test comment references (path text only, no logic change):
   `agents-plugin-tool/internal/mcp/session_state.go#L574`,
   `agents-plugin-tool/internal/wsdoc/tickets_sage.go#L100`,
   `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go#L14`,
   `agents-plugin-tool/internal/wsrsrc/skills_compose_test.go#L12`,
   `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L2767`.
10. Edit `agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md`'s routing table
    (`#L86-L96`, `## On: index health check` step 7): add a new row routing
    procedure/how-to content (has a one-line applicability description) to
    `ai-docs/manuals/` with a `summary:` frontmatter line equal to that
    description; keep the existing "Static reference material → `ai-docs/ref/`"
    row unchanged (true static/historical references still belong there — e.g.
    `ref/claude-home-legacy.md`). Do **not** hand-edit the
    `agents-plugin-wsflow/` mirror.
11. From `agents-plugin-tool/`, run the two mandatory regen commands in order
    (see Codebase Findings) to regenerate `agents-plugin/rsrc/manifest.json`,
    `agents-plugin/skills/manifest.json`, and the wsflow `lead-bootstrap.md`
    mirror.
12. Confirm `ai-docs/ref/` retains only: `agent-harness-capability-tiers.md`,
    `claude-home-legacy.md`, `design.md`, `verify-dashboard-archive-recovery.sh`,
    `worktree-ticket-scope.md`.
13. Sweep-grep for any remaining `ai-docs/ref/<moved-name>` across the repo
    (excluding `CHANGELOG.md`, `ai-docs/tickets/.done/`, `ai-docs/.plans/`, and
    other tickets' body text per Out of Scope) and confirm zero hits.

## Verification Plan

- `cd agents-plugin-tool && ~/.local/go-toolchain/go/bin/go build ./... && ~/.local/go-toolchain/go/bin/go vet ./... && ~/.local/go-toolchain/go/bin/go test ./...`
  — must be green, in particular `TestValidateRealTree`,
  `TestWsflowRsrcMirrorUpToDate`, `TestGenerateRealManifest`,
  `TestPlaybookPrintGoldenLeadBootstrap`.
- `python3 -m unittest discover agents-plugin-wsflow/tests` — required per
  `ai-docs/ref/wsflow-mirroring.md` (post-move: `ai-docs/manuals/wsflow-mirroring.md`)
  since this phase edits a full shared `agents-plugin/rsrc/` playbook.
- Manual: call `manuals.list` (or `ws-cli manuals list`) and confirm all 6
  entries with the exact `summary:` text from the `_index.md` rows; call
  `workflow_manual` (fresh-with-root or continue branch) and confirm the
  `# Manuals` block lists all 6 with no no-summary marker.
- `grep -rn "ai-docs/ref/skill-authoring\|ai-docs/ref/wsflow-mirroring\|ai-docs/ref/codex-integration\|ai-docs/ref/ws-mcp\|ai-docs/ref/windows-dogfood\|ai-docs/ref/ws-agent-runtime" --exclude-dir=.git --exclude=CHANGELOG.md .`
  restricted to non-historical paths returns nothing.
- Run `ws/spec_index.verify` (or equivalent doc-index check) — Phase 1's
  result note flagged a prior doc pass introducing duplicate anchors; re-check
  after editing `documentation-system.md` and `workflow-skills.md`.
- `ls ai-docs/ref/` shows only the 5 non-candidate files.

## Escalations

- None.
