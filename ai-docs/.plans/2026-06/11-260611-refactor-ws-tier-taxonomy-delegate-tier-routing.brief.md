# Brief: 260611-refactor-ws-tier-taxonomy-delegate-tier-routing (Phase 1)

## Intent
Close 260609 Edition `379ff5e5` gaps 1+2: the render-minted child-key splice and
the `{{.*Model}}` model-alias variables exist in the runtime but never fire on
the shipped surface because **no shipped playbook declares `role:` or uses a model
var**. Ship real delegate playbook assets (implementer + reviewer) that exercise
both, recognize the first-class `tier:` frontmatter field (parse-only), regenerate
the rsrc manifest, and add a shipped-asset e2e that catches a missing/inert asset
(existing tests only use in-memory fixtures).

## Scope Boundary
Phase 1 only. IN: new shipped delegate playbooks under `agents-plugin/rsrc/`
(`implementer`, `reviewer`) with `role:` + `tier:` + a declared/used model-alias
var; forward-compat `Tier` field parse in `PlaybookMeta`; rsrc manifest regen +
an up-to-date guard test; shipped-tree render e2e (splice fires + model var
resolves from config). OUT (later phases): threading `tier:` into
`RegisterOptions.Tier` / mercenary model resolution (Phase 2); `oneShot` dead-code
removal (Phase 2); first-class vocabulary adoption + reviewer-allocation default
re-author + spec/mental-model migration (Phase 3); rewiring `lead-implement` to
render these assets instead of its inline templates / stale `register(prompts:)`
references (follow-up, not a Phase-1 gap).

## Caller-Visible Contract
- Two new shipped playbooks resolvable by `playbook.render`/`playbook.print`:
  `implementer` (role: implementer, tier: medium) and `reviewer` (role: reviewer,
  tier: large), `kind: render`, `delegates: true`.
- When a **lead** key renders either (mintRoot set), the existing credential
  block (`**Your ws session_key: \`<key>\`**` … delimited by `---`) is spliced at
  the top of the rendered body, and the minted child key is bound to mintRoot with
  delegate scope.
- The body references a model-alias var that resolves from `config.agents_tier`:
  implementer uses `{{.CoreModel}}` (alias `core` ↦ first-class medium), reviewer
  uses `{{.DeepModel}}` (alias `deep` ↦ first-class large).
- `PlaybookMeta.Tier` is parsed from frontmatter `tier:` (recognized, not dropped
  to `Extra`). Honoring it for routing is Phase 2 — Phase 1 only records it.

## Contract Instructions
- `agents-plugin-tool/internal/wsrsrc/wsrsrc.go` — add `Tier string` to
  `PlaybookMeta` (mirror the existing `Role` field + its doc comment listing
  first-class values `small|medium|large|xlarge`).
- `agents-plugin-tool/internal/wsrsrc/loader.go` — in `metaFromFrontmatter`, add a
  `tier:` scalar case mirroring the `role:` case (lines ~181-184).
- `agents-plugin/rsrc/implementer/implementer.md` and
  `agents-plugin/rsrc/reviewer/reviewer.md` — new delegate playbooks. Frontmatter
  per Caller-Visible Contract; declare exactly the model var(s) used in
  `variables:`. Bodies: concise, self-contained delegate guidance distilled from
  the `lead-implement` "Implementer spawn prompt" / "Reviewer prompt frame"
  templates; keep per-call specifics as `<angle>` fill markers (the lead fills
  them at delegation time), NOT template vars. Use `{{.CoreModel}}` /
  `{{.DeepModel}}` exactly once for the model-tier line.
- `agents-plugin/rsrc/manifest.json` — regenerate via the guard test below.
- Reuse the existing mint/render machinery: do NOT add new render code paths;
  `renderPlaybookBody` already mints+splices on `role:` and resolves model vars on
  declared `variables:`.
- Forbidden: hand-editing `manifest.json` hashes; adding a new `cmd/` tool;
  rewiring `lead-implement`; touching `config.agents_tier` resolution.

## Integration Test Instructions
- `agents-plugin-tool/internal/wsrsrc` — add `TestShippedManifestUpToDate`:
  `GenerateManifest` over the shipped tree (`../../../agents-plugin/rsrc`) and
  assert it equals the committed `manifest.json` (helpful failure: rerun with
  `WS_REGEN_MANIFEST=1`). Add a sibling regen path guarded by `WS_REGEN_MANIFEST=1`
  that `WriteManifest`s the shipped tree (used once to regen, then the guard
  protects it). This mitigates `260611-bug-rsrc-manifest-regen-missed`.
- `agents-plugin-tool/internal/mcp/shipped_delegate_asset_test.go` (new) — e2e on
  the **real shipped tree** (`filepath.Join("..","..","..","agents-plugin","rsrc")`),
  mirroring `TestRenderMintsChildKeyForLeadDelegatePlaybook` +
  `TestPlaybookPrintModelAliasFromConfig`: for `implementer` (core) and `reviewer`
  (deep), set a unique config model via `wsconfig.SetAgentsTierForHarness`, call
  `renderPlaybookBody(..., mintRoot, false)`, assert (a) `sessionKeyInBodyRe`
  matches + `s.sessions.lookup(key)` binds to mintRoot, and (b) the unique config
  model string appears in the body.
- Run: `cd agents-plugin-tool && go test ./internal/wsrsrc ./internal/mcp`.

## Implementation Strategy Decisions
- `tier:` is parse-only in Phase 1 (recognized field; no behavior). Frontmatter
  declares the FIRST-CLASS tier (`medium`/`large`), never backend/model/effort.
- Model-var line uses the alias that maps to the asset's first-class tier
  (core↦medium, deep↦large) to demonstrate the locked mapping.
- Manifest integrity handled by a guard test + env-gated regen, not a new CLI
  (the CLI/tooling gap is owned by `260611-bug-rsrc-manifest-regen-missed`).

## Rejected Alternatives
- Hand-editing manifest.json — brittle vs deterministic GenerateManifest.
- A `delegate-sample`-style throwaway fixture — must be a real shipped asset to
  honestly close the gap (the e2e proves the SHIPPED asset fires).
- Adding a `cmd/gen-rsrc-manifest` — out of Phase-1 scope; belongs to the regen bug.
- Mirroring to wsflow — wsflow is agentless (no rsrc/, no delegation axis), so
  delegate assets are a ws-only divergence per the epic decision.

## Constraints
- Shipped-rsrc edits MUST leave manifest.json consistent (guard test green).
- AI-authored playbook/test/doc content is English.
- Honor the phase boundary: no tier honoring, no vocab adoption, no oneShot removal.

## Out of scope
Phase 2 (tier routing, oneShot), Phase 3 (vocabulary + reviewer default + spec
migration), lead-implement rewiring, wsflow mirror.

## Verification Contract
- `go test ./internal/wsrsrc ./internal/mcp` passes, including the new
  `TestShippedManifestUpToDate` and `shipped_delegate_asset_test.go`.
- `go build ./...` clean.
- `git status` shows manifest.json updated with the two new playbook entries.

## References
<!-- [Must] read before starting. [Maybe] consult if uncertain. -->
- `agents-plugin-tool/internal/mcp/playbook_tools.go:235-298` [Must] - renderPlaybookBody mint/splice + model-var resolution.
- `agents-plugin-tool/internal/mcp/mercenary_surface_test.go` [Must] - mint-test mirror (sessionKeyInBodyRe, role fixtures).
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go:349-410,639-700` [Must] - model-alias-from-config + golden-real-tree patterns.
- `agents-plugin-tool/internal/wsrsrc/wsrsrc.go:17-33`, `loader.go:165-198` [Must] - PlaybookMeta + frontmatter mapping (Role mirror for Tier).
- `agents-plugin-tool/internal/wsrsrc/manifest.go:41-83` [Must] - GenerateManifest/WriteManifest.
- `ai-docs/mental-model/prompt-bundle.md` [Maybe] - playbook role/var model.
