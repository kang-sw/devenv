# Survey: 10-internal-procedures-playbook

## Reusable Components

- `agents-plugin/rsrc/explore/explore.md` — `explore` playbook: `kind: render`, `delegates: true`,
  declares `ExploreAgent` / `SpawnIdiom` / `ContinueIdiom`. Direct layout precedent for any of the
  9 migrated playbooks that spawn native subagents.
- `agents-plugin/rsrc/sample-playbook/sample-playbook.md` — `kind: print`, `includes:
  [sample-conventions]`, `variables: [WorktreeID]`. Canonical `kind: print` schema example with
  auto-include + variable substitution exercised by `TestPlaybookPrintGoldenSamplePlaybookNoDelegation`.
- `agents-plugin/rsrc/sample-conventions.md` — `kind: text` flat dep. Shows how a pure-text
  include dep is laid out (no frontmatter `kind: text` is fine per loader).
- `agents-plugin/rsrc/delegate-sample/` — additional golden fixture for `kind: render /
  delegates: true`; see playbook_tools_test.go golden tests (lines 639–845).
- `agents-plugin-tool/internal/wsrsrc/wsrsrc_test.go#L28-L63` — `buildMinimalTree` /
  `buildTreeWithIncludes` helpers: minimum valid tree shape, includes wiring, how to call
  `GenerateManifest` + `WriteManifest`.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L22-L44` — `buildTestRsrcTree` helper:
  creates a temp rsrc tree + fresh manifest; pattern to copy for new procedure golden tests.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L639-L845` — existing golden real-tree
  tests. New migrated-procedure tests should follow the same structure (real rsrc root, derive +
  hardcode assertions, check delegates:true tip and convention include text).

## Existing Patterns

- **Playbook directory layout**: one subdirectory per playbook (`<name>/<name>.md`), optional
  harness overlay at `<name>/<name>.<harness>.md`, flat text deps at `<name>.md` beside
  subdirectory. All files must be listed in `manifest.json`.
  See `agents-plugin/rsrc/explore/` and `agents-plugin/rsrc/sample-playbook/`.
- **Manifest regen workflow**: edit rsrc files → run
  `cd agents-plugin-tool && WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -run TestGenerateRealManifest -v`.
  Gate: `TestValidateRealTree` fails if manifest drifts.
  See `agents-plugin-tool/internal/wsrsrc/wsrsrc_test.go#L665-L689`.
- **Skill-to-internal-skill rewrite pattern**: current "invoke `ws:lead-<name>`" in skills maps to
  "call `ws/playbook.print(name: "lead-<name>")` and execute the returned procedure inline in
  this context." Wording in entry skills varies — some say `Invoke`, `Continue through`, or appear
  in routing tables; all references to the 9 as `ws:lead-<name>` need updating.
- **Lead-owned infra.read / convention.read inline pattern**: both survive as inline runtime calls
  inside playbook body text (not rsrc includes); see convention.read design below.

## Relevant Interfaces

- `agents-plugin-tool/internal/wsrsrc/wsrsrc_test.go#L665-L670` — `TestValidateRealTree`: CI gate
  that validates `agents-plugin/rsrc/` tree. Every new playbook added here must pass.
- `agents-plugin-tool/internal/wsrsrc/wsrsrc_test.go#L676-L689` — `TestGenerateRealManifest`:
  regen trigger; set `WSRSRC_REGEN=1` to regenerate `manifest.json` after tree edits.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L594-L633` — `TestPlaybookPrintMCPDispatch`:
  end-to-end dispatch test pattern; new procedure tests can set `WS_RSRC_ROOT` + call `callTool`.
- `agents-plugin-tool/internal/wsdoc/conventions.go#L11-L52` — `conventionFS go:embed` + `ReadConvention`.
  Convention files live at `agents-plugin-tool/internal/wsdoc/conventions/{ticket,spec,mental-model}-conventions.md`.
  Entirely separate from rsrc tree; `convention.read` resolves from this embedded FS, not `agents-plugin/rsrc/`.
- `agents-plugin-tool/internal/wsprompt/prompts.go#L18` — `go:embed prompts/*.md infra/*.md`.
  `infra.read` resolves from this embedded FS. Same separation story as convention.read.
- `agents-plugin-tool/internal/wsrsrc/wsrsrc_test.go#L403-L415` — `TestAutoIncludeConcatenated`:
  shows include text is appended to playbook body verbatim.

## Constraints

- `TestValidateUnlistedFileInTree` (`wsrsrc_test.go#L644-L656`): adding any `.md` to `rsrc/`
  without regenerating `manifest.json` causes CI failure. Regen must happen before committing.
- rsrc includes are **flat root-level only**: `includes: [foo]` resolves to
  `agents-plugin/rsrc/foo.md`. A name like `lead-write-spec/spec-conventions` is invalid; there is
  no subdirectory-relative include resolution (`prompt-bundle.md` §"rsrc includes are flat").
- Variables in the body must be declared in `variables:` frontmatter; undeclared use →
  `ErrUndeclaredVar` from validator. Reserved names (`ExploreAgent`, `SpawnIdiom`, `ContinueIdiom`,
  `LightModel`, `CoreModel`, `DeepModel`) are tool-injected; do not declare them unless the
  playbook actually uses them in its body.
- `ws/agents.register` is referenced in several of the 9 skill bodies. After Phase 2 these become
  playbook body text; they are live MCP call instructions, NOT placeholders. The body must remain
  semantically correct for the current runtime (agents.* still exists in the ws live server for
  Phase 2; M3 removes it). No stub or mock wording permitted.
- `lead-update-spec/SKILL.md#L24` contains a **file-path reference** `agents-plugin/skills/lead-write-spec/SKILL.md`
  that must be updated to the new rsrc location after migration.
- wsflow ships 8 of the 9 as mirrored skills (all except `lead-write-skeleton`). Phase 2 is
  OUT OF SCOPE for wsflow edits; the follow-up chore ticket
  (`260610-chore-wsflow-explore-playbook-mirroring`) owns that. Do not edit wsflow skills.
  The wsflow suite must stay green because Phase 2 does not touch wsflow source.

## Risk Signals

- `agents-plugin-tool/internal/wsdoc/conventions.go#L11-L48` — **Convention source is go:embed, not
  rsrc-tree.** Convention files live at `agents-plugin-tool/internal/wsdoc/conventions/` (embedded
  FS). The rsrc loader resolves `includes: [ticket-conventions]` to
  `agents-plugin/rsrc/ticket-conventions.md` — a different path. Copying conventions to rsrc is
  FORBIDDEN (split-brain); there is no symlink or bridge loader. The clean Phase 2 path is option
  (b): playbook bodies keep `ws/convention.read(name: ...)` as inline MCP calls — exactly what the
  skills do today. The anchor's "auto-include + read tools retained; first-pass timing open" means
  rsrc auto-include for convention files is a later-phase unification, not Phase 2 work. **No
  escalation to research needed; option (b) is the correct resolution.**
- `agents-plugin/skills/lead-update-spec/SKILL.md#L24` — `Read agents-plugin/skills/lead-write-spec/SKILL.md`.
  After migration, this path no longer exists. The playbook body for `lead-update-spec` must update
  this to `agents-plugin/rsrc/lead-write-spec/lead-write-spec.md`. Missed update → stale read
  instruction in production. Lead/planner should flag this in the rewrite.
- `agents-plugin/skills/lead-implement/SKILL.md#L60,L111` — `ws/infra.read(name: "impl-playbook")`
  and `ws/infra.read(name: "executor-wrapup")`. Both survive as inline calls in the migrated
  playbook body. infra.read is go:embed (same separate-FS situation as convention.read); no rsrc
  include path. No action required other than keeping the inline calls verbatim.
- `agents-plugin/skills/lead-workflow-manual/SKILL.md#L9` — Self re-invoke instruction: "After
  compaction, re-invoke `ws:lead-workflow-manual` when primitive names...". After migration, there is
  no `ws:lead-workflow-manual` Skill. This self-invocation line inside the playbook body must be
  rewritten to `ws/playbook.print(name: "lead-workflow-manual")` or removed. The instruction is
  caller-facing guidance; the lead must update it so the playbook body is internally consistent.
- `agents-plugin/skills/lead-write-skeleton/SKILL.md#L35-L36,L69-L88` — `ws/agents.register` /
  `ws/agents.call` for `skeleton-populator` and `skeleton-reviewer`. These are **bundled embedded
  prompt stems** (from `go:embed prompts/*.md`); they are NOT rsrc playbooks and are NOT being
  migrated. The playbook body for `lead-write-skeleton` (if any — see Constraints below) would
  carry these inline calls verbatim without rsrc includes.
- `agents-plugin/skills/lead-write-spec/SKILL.md#L15,L37` — Accuracy-check line uses
  `Spawn a native Explore-style subagent via the \`explore\` playbook (see \`lead-workflow-manual\`)`.
  After migration, both the accuracy-check in `lead-write-spec`'s playbook body AND any inline
  reference to `lead-workflow-manual` must use the playbook.print form. The `see lead-workflow-manual`
  cross-reference becomes a cross-playbook print call, not a Skill reference.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L232-L265` — **Continuity-tip injection**
  fires for `delegates: true` playbooks. Any procedure body that spawns agents must declare
  `delegates: true`; omitting it suppresses the tip that the tests assert for delegate playbooks.
  The inverse (`delegates: true` on a non-delegating procedure) adds spurious tip text. Get
  `delegates:` right per-playbook.

## The 9 Internal Skills — Per-Skill Reference Map

| Skill | SKILL.md path | Lines | delegates | infra.read calls | convention.read calls | Inter-procedure calls (→ other internal) |
|-------|--------------|-------|-----------|------------------|-----------------------|------------------------------------------|
| lead-implement | `agents-plugin/skills/lead-implement/SKILL.md` | 381 | **true** | `impl-playbook` (L60), `executor-wrapup` (L111) | — | → `lead-update-spec` (L73, L102) |
| lead-write-ticket | `agents-plugin/skills/lead-write-ticket/SKILL.md` | 275 | false | — | `ticket-conventions` (L12 invariant, L25 step-1) | → `lead-write-spec` (L158); self-separate (L73, L97) |
| lead-write-spec | `agents-plugin/skills/lead-write-spec/SKILL.md` | 130 | **true** (Explore accuracy check L37) | — | `spec-conventions` (L12 invariant) | → `lead-write-ticket` (L22, suggestion only) |
| lead-workflow-manual | `agents-plugin/skills/lead-workflow-manual/SKILL.md` | 241 | false | — | — | self re-invoke (L9, needs rewrite) |
| lead-check-blockers | `agents-plugin/skills/lead-check-blockers/SKILL.md` | 8 | false | — | — | — |
| lead-verify-design | `agents-plugin/skills/lead-verify-design/SKILL.md` | 169 | **true** (design-reviewer agent L49-L51) | — | — | → `lead-verify-discussion` (L34) |
| lead-verify-discussion | `agents-plugin/skills/lead-verify-discussion/SKILL.md` | 33 | **true** (Explore subagents L25) | — | — | — |
| lead-write-skeleton | `agents-plugin/skills/lead-write-skeleton/SKILL.md` | 190 | **true** (skeleton-populator/-reviewer L35-L88, Explore L45) | — | — | — |
| lead-update-spec | `agents-plugin/skills/lead-update-spec/SKILL.md` | 92 | false (explicit: "no subagent delegation") | — | `spec-conventions` (L13 invariant, L24 step-1) | reads `lead-write-spec/SKILL.md` path (L24, stale after migration) |

## Call-Site Inventory — Entry Skills → Internal Skills

All references to the 9 that need updating to `ws/playbook.print` + inline execution, or to
remove the ws: reference as a slash-command suggestion:

### lead-proceed/SKILL.md

| Line | Exact text | Invocation type |
|------|-----------|-----------------|
| 14 | `Invoke \`ws:lead-workflow-manual\` first when workflow primitives are not already in context.` | Skill-tool invoke |
| 87 | `Continue through \`ws:lead-write-ticket\`; capture \`Ticket:\` and re-route.` | Skill-tool invoke |
| 92 | `Continue through \`ws:lead-write-ticket\`; capture \`Ticket:\` and re-route.` | Skill-tool invoke |
| 93 | `Continue through \`ws:lead-write-ticket\`; capture \`Ticket:\` and re-route.` | Skill-tool invoke |
| 94 | `Continue through \`ws:lead-implement\`.` | Skill-tool invoke |
| 95 | `Continue through \`ws:lead-write-ticket\`; capture \`Ticket:\` and re-route.` | Skill-tool invoke |
| 96 | `Continue through \`ws:lead-implement\`.` | Skill-tool invoke |
| 103 | `NEXT: <ws:lead-discuss | ws:lead-write-ticket | ws:lead-implement | stop>` | Template label (update label text) |
| 132 | `invoke \`ws:lead-implement\` before any source inspection, planning, or editing.` | Skill-tool invoke |
| 136 | `If \`ws:lead-write-ticket\` ran, capture its \`Ticket:\` path before downstream routing.` | Reference (update name) |

### lead-salvage/SKILL.md

| Line | Exact text | Invocation type |
|------|-----------|-----------------|
| 29 | `Invoke \`ws:lead-workflow-manual\`.` | Skill-tool invoke |
| 83 | `Invoke \`ws:lead-write-ticket\` to create or update one research ticket...` | Skill-tool invoke |
| 84 | `invoke \`ws:lead-write-ticket\` to create or update one recovery epic.` | Skill-tool invoke |
| 85 | `invoke \`ws:lead-write-ticket\` separately for each child ticket.` | Skill-tool invoke |
| 87 | `invoke \`ws:lead-write-ticket\` separately for each approved rewrite...` | Skill-tool invoke |

### lead-discuss/SKILL.md

| Line | Exact text | Invocation type |
|------|-----------|-----------------|
| 32 | `Invoke \`ws:lead-workflow-manual\` via Skill tool (loads orchestration primitives reference).` | Skill-tool invoke |
| 79 | `Invoke \`ws:lead-write-ticket\` (Edit path) for the \`todo/\` -> \`ready/\` promotion.` | Skill-tool invoke |
| 80 | `\`ws:lead-write-ticket\` owns spec addressing, frontmatter population...` | Reference (update name) |
| 81 | `Stop this handler after \`ws:lead-write-ticket\` returns.` | Reference (update name) |
| 84 | `invoke \`ws:lead-write-spec\` to remove the 🚧 entry.` | Skill-tool invoke |
| 92 | `suggest \`ws:lead-write-spec\` as the next route` | User suggestion (ws: name is removed) |
| 94 | `invoke \`ws:lead-write-ticket\`.` | Skill-tool invoke |
| 95 | `invoke \`ws:lead-write-ticket\`, then append design notes...` | Skill-tool invoke |

### lead-sprint/SKILL.md

| Line | Exact text | Invocation type |
|------|-----------|-----------------|
| 34 | `Invoke \`ws:lead-workflow-manual\`.` | Skill-tool invoke |
| 88 | `Invoke \`ws:lead-update-spec\` with \`<episode-range>\`...` | Skill-tool invoke |

### lead-review/SKILL.md

| Line | Exact text | Invocation type |
|------|-----------|-----------------|
| 15 | `route through \`ws:lead-discuss\` and \`ws:lead-implement\`.` | User guidance (update ws:lead-implement reference) |

### lead-bootstrap/SKILL.md

| Line | Exact text | Invocation type |
|------|-----------|-----------------|
| 90 | `\`ws:lead-forge-spec\` or \`ws:lead-write-spec\`` | Table suggestion (update ws:lead-write-spec) |
| 94 | `\`ws:lead-write-ticket\`` | Table suggestion (update) |

### lead-forge-spec/SKILL.md

| Line | Exact text | Invocation type |
|------|-----------|-----------------|
| 194 | `note it as a split candidate for a follow-up \`ws:lead-write-spec\` invocation.` | Follow-up suggestion (update) |
| 253 | `Run \`ws:lead-write-spec\` for any domain surfaces...` | User guidance (update) |
| 264 | `note the file for a follow-up \`ws:lead-write-spec\` invocation.` | Follow-up suggestion (update) |

## Call-Site Inventory — Inter-Procedure (Internal → Internal)

These live in the 9 bodies being migrated; they are rewritten in the rsrc playbook source:

| File | Line | Exact text | Becomes |
|------|------|-----------|---------|
| `lead-implement/SKILL.md` | 73 | `invoke \`ws:lead-update-spec\`, then \`mental-model-updater\`` (task list) | `ws/playbook.print(name: "lead-update-spec")` + inline execution |
| `lead-implement/SKILL.md` | 102 | `Invoke \`ws:lead-update-spec\` with \`<commit-range>\`.` | `ws/playbook.print(name: "lead-update-spec")` + inline execution |
| `lead-write-ticket/SKILL.md` | 73 | `start a separate \`ws:lead-write-ticket\` invocation for child creation...` | `ws/playbook.print(name: "lead-write-ticket")` + inline execution |
| `lead-write-ticket/SKILL.md` | 97 | `start a separate \`ws:lead-write-ticket\` invocation for the child ticket.` | `ws/playbook.print(name: "lead-write-ticket")` + inline execution |
| `lead-write-ticket/SKILL.md` | 158 | `continue through \`ws:lead-write-spec\`...` | `ws/playbook.print(name: "lead-write-spec")` + inline execution |
| `lead-write-ticket/SKILL.md` | 228 | `continue through \`ws:lead-write-spec\` only when judge: contract-first-spec is yes.` | `ws/playbook.print(name: "lead-write-spec")` + inline execution |
| `lead-write-spec/SKILL.md` | 22 | `suggest \`ws:lead-write-ticket\`. Exit.` | Update name (suggestion, no Skill invocation needed) |
| `lead-update-spec/SKILL.md` | 24 | `Read \`agents-plugin/skills/lead-write-spec/SKILL.md\`.` | `Read \`agents-plugin/rsrc/lead-write-spec/lead-write-spec.md\`.` |
| `lead-verify-design/SKILL.md` | 34 | `invoking \`ws:lead-verify-discussion\` when available` | `ws/playbook.print(name: "lead-verify-discussion")` + inline execution |
| `lead-workflow-manual/SKILL.md` | 9 | `re-invoke \`ws:lead-workflow-manual\` when primitive names...` | `ws/playbook.print(name: "lead-workflow-manual")` + inline execution |

## Convention Auto-Inclusion — OPEN FORK Resolution

**The single-source path for Phase 2 is option (b): inline `ws/convention.read` calls.**

Evidence:
- `convention.read` is served by `go:embed` from `agents-plugin-tool/internal/wsdoc/conventions/`
  (`conventions.go#L11`). Files: `ticket-conventions.md`, `spec-conventions.md`,
  `mental-model-conventions.md`.
- The rsrc loader resolves `includes: [ticket-conventions]` to
  `agents-plugin/rsrc/ticket-conventions.md` — a **different tree** from the embedded FS.
- Bridging the two without duplication requires either moving the convention files to rsrc and
  updating the `go:embed` import path (cross-module Go refactor), or adding a bridge loader
  (new loader, FORBIDDEN by brief). Both exceed Phase 2 scope.
- The anchor explicitly records: "first-pass-vs-later timing remains an implementation-sequencing
  call" (260605 §"Convention loading via playbook"). Phase 2 is the first pass.
- Inline `ws/convention.read(name: ...)` calls remain in the playbook body text exactly as they
  appear in the skill bodies today. The playbook executor reads the convention text at execution
  time — identical behavior to the current Skill tool flow.
- `convention.read` and `infra.read` survive as standalone tools (anchor decision); they are NOT
  competing with execution-path auto-include. Using them inline IS the supported path.

**No escalation to research. Implement option (b).**

## Validate-Tree / Loader / Manifest Test Surface

Requirements for every new `kind: print` procedure playbook:

1. **Required base variant** (`TestValidateMissingRequiredVariant`, `wsrsrc_test.go#L560-L576`):
   `agents-plugin/rsrc/<stem>/<stem>.md` must exist.
2. **Declared variables** (`TestValidateUndeclaredVariable`, `wsrsrc_test.go#L578-L594`):
   every `{{.Var}}` in body must appear in `variables:` frontmatter.
3. **Flat includes resolve** (`TestValidateDanglingInclude`, `wsrsrc_test.go#L596-L611`):
   every `includes:` entry must have a corresponding `<name>.md` in the rsrc root.
4. **Manifest coverage** (`TestValidateUnlistedFileInTree`, `wsrsrc_test.go#L644-L656`):
   every file in the rsrc tree must be listed in `manifest.json`; add then regen.
5. **Hash consistency** (`TestValidateManifestHashDrift`, `wsrsrc_test.go#L613-L623`):
   editing a file after regen will fail; regen must be the last step before commit.

Manifest regen command:
```bash
cd agents-plugin-tool && WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -run TestGenerateRealManifest -v
```

CI gate: `TestValidateRealTree` (`wsrsrc_test.go#L665-L670`) validates the committed tree. Runs as
part of `go test ./...` from `agents-plugin-tool/`.

New `playbook_tools_test.go` tests to add (one per migrated procedure):
- Print test: `buildTestRsrcTree` with the migrated playbook body → `printPlaybook(...)` → assert
  body resolves (contains known non-trivial text from the procedure). If the playbook calls
  `ws/convention.read` inline, the print output itself will NOT contain convention text (inline
  calls are procedure instructions, not pre-resolved includes) — so assert procedure body text, not
  convention text.
- If any procedure uses `includes:` (not applicable for Phase 2 per convention decision), assert
  included text is present.

## lead-write-skeleton Disposition

**No rsrc playbook required. Confirmed: zero live call sites.**

Search result: `grep -rn "lead-write-skeleton" agents-plugin/skills/` returns only
`lead-write-skeleton/SKILL.md` itself (the `name:` frontmatter line). No entry skill invokes it.
No wsflow skill references it. The `lead-write-skeleton/SKILL.md#L162` text
`"Next: caller-owned when invoked by ws:lead-implement | ws:lead-implement when invoked standalone"`
is internal output text, not an external call.

Action: remove `SKILL.md` entry point (per Phase 2 disposition decision). No rsrc playbook
authored; document the deprecation disposition in the implementation commit's AI Context.

## lead-skill-authoring Audit Target Relocation

Current implied glob: `agents-plugin/skills/**/SKILL.md` (covers all live skills).

After migration, the 9 procedure bodies live at:
`agents-plugin/rsrc/lead-<name>/lead-<name>.md`

Updated target for fresh-reader audit:
- `agents-plugin/rsrc/lead-*/lead-*.md` (9 migrated procedure playbooks)
- `agents-plugin/skills/*/SKILL.md` (11 remaining entry skills)

The `lead-skill-authoring/SKILL.md` invariant-audit procedure and `## On: Fresh-Reader Audit`
wording should update its target description from "SKILL.md files" to "rsrc playbook sources and
entry SKILL.md files". The `## On: Downstream Consistency Sweep` surface list similarly broadens
to include rsrc.

File to update: `agents-plugin/skills/lead-skill-authoring/SKILL.md` (entry skill stays; only its
audit-target description changes).

## wsflow Mirror Surface (Phase 2 Non-Scope, for Awareness)

8 of the 9 internal skills are wsflow-mirrored:
- Included (8): `lead-workflow-manual`, `lead-write-spec`, `lead-write-ticket`, `lead-check-blockers`,
  `lead-implement`, `lead-verify-design`, `lead-verify-discussion`, `lead-update-spec`
- Excluded (1): `lead-write-skeleton`

Phase 2 does NOT edit wsflow skills. The wsflow suite (`python3 -m unittest discover
agents-plugin-wsflow/tests`) must stay green as a no-drift check. Run before committing
entry-skill rewrites to confirm none of the entry-skill changes introduced wsflow-banned references.

## Opinion

- The `lead-workflow-manual` self-re-invoke instruction (`SKILL.md#L9`) is the most easily missed
  internal inconsistency: once the skill is removed, the "re-invoke `ws:lead-workflow-manual`"
  instruction in the playbook body becomes a dead reference. It should be rewritten before
  review or it will surface as a fresh-reader finding.
- `lead-implement` at 381 lines is the largest body by a wide margin. Its delegated path still
  uses `ws/agents.register` / `ws/agents.call` for reference-discovery, implementer, reviewer,
  and mental-model-updater — all of which remain live in the Phase 2 runtime. The migration is
  text-only; no ws/agents.* calls need changing in Phase 2.
- The `lead-update-spec` reads `lead-write-spec/SKILL.md` directly (L24). This is the only
  internal file-path cross-reference among the 9 and is the most concrete migration coupling
  risk. Update it to the new rsrc path in the `lead-update-spec` playbook.
- Several entry skills reference the 9 as user-facing suggestions (e.g. "suggest
  `ws:lead-write-spec` as the next route" in `lead-discuss/SKILL.md:92`). After migration these
  names are not user-typeable slash commands. The brief's verification contract ("no call site
  references a removed ws:lead-<name> skill invocation") covers all such references, not just
  Skill-tool invocations.
