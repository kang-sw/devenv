# Brief: 260609-feat-ws-playbook-surface-mvp (Phase 1)

## Intent

Build the call-time loading substrate the playbook surface sits on: a plain-text
`rsrc/` prompt/playbook tree (NOT `go:embed`), a Go loader that reads it from the
plugin path or a `WS_RSRC_ROOT` dev override, a declared-variable substitution
engine, a frontmatter `includes:` auto-include resolver, a `manifest.json` with
file hashes plus a playbook **schema version** (compatibility gated on
schema-version, not hash equality), and a CI tree validator. No MCP tool in this
phase — the loader/validator is exercised through Go tests and a CI tree check.

## Scope Boundary

In scope (Phase 1 only):
- `rsrc/` directory layout + fully-custom playbook frontmatter schema.
- Loader (plugin-path default OR `WS_RSRC_ROOT` override of the load root).
- Substitution engine (declared variables only).
- Auto-include resolver (frontmatter `includes:` pulled at load time).
- `manifest.json` (file hashes + schema version) + schema-version compatibility
  check; loud partial failure with no embedded fallback.
- CI tree validator (required harness variants present, declared substitution
  variables resolvable, declared `includes:` exist, manifest hashes in sync).
- 1–2 minimal representative sample playbooks to seed/exercise the tree (base +
  codex overlay + an include + a substitution variable). These are fixtures, NOT
  real skill-body migrations.

Explicitly deferred / out of this phase:
- `playbook.print` / `playbook.render` MCP tools and harness-aware *selection*
  (Phase 2). Phase 1 defines the overlay naming convention and can load a named
  variant, but the "detect harness → pick variant" wiring is Phase 2.
- Model-name config tables, delegation-tip injection, unknown-harness fallback
  rendering (Phase 2).
- Any skill-body conversion (M2), spawn/actor changes (M3), api.ask (M4).

## Caller-Visible Contract

Phase 1 ships no MCP tool, so "caller" = the Go package API consumed later by the
MCP layer, plus the on-disk `rsrc/` tree contract and the CI validator.

- A new internal Go package (proposed `internal/wsrsrc`) exposes:
  - A loader that, given a resolved root, loads a playbook by name: returns the
    rendered body (base or a requested harness variant), resolved auto-includes,
    declared variable set, and frontmatter (`kind`, `delegates`, `includes`,
    declared variables).
  - Root resolution: `WS_RSRC_ROOT` env when set and non-empty, else the
    plugin-path default. Phase 1 tests drive the loader via `WS_RSRC_ROOT` and
    explicit roots; the plugin-path default is a documented `ResolveRoot` function
    that leaves a Phase-2 stub comment for the `os.Executable()` →
    `filepath.Dir(exe)` → `../rsrc` derivation (do NOT implement that derivation
    in Phase 1; just leave the seam).
  - A manifest reader + schema-version compatibility check. Incompatible or
    missing manifest, missing manifest-listed files, or hash mismatch on a loaded
    file ⇒ a loud typed error (no fallback text). The package exposes a
    supported-schema-version constant.
  - An exported `Validate(root)` used by the CI validator and by a Go test that
    validates the committed `agents-plugin/rsrc/` tree.
- On-disk `rsrc/` tree contract (proposed, layout details at implementer's
  discretion within these invariants):
  - Tree root contains `manifest.json`.
  - One directory per playbook; base file `<name>.md`; optional harness overlay
    `<name>.codex.md` (claude/host-neutral uses the base; only `codex` overlays in
    the 2-harness set). Overlay naming convention is `<name>.<harness>.md`.
  - Frontmatter fields (at least): `kind: print|render`, `delegates: bool`,
    `includes: [<text-dep-name>]`, and a declaration of substitution variables
    (e.g. `variables: [<name>]`).
  - Auto-include text-deps resolve by bare name within the rsrc tree; dangling
    include ⇒ load error and CI failure.
  - `manifest.json`: `{ schema_version: <int>, files: { <relpath>: <sha256> } }`
    (exact shape implementer's choice, but must carry schema_version + per-file
    hashes).

## Contract Instructions

- New package: `agents-plugin-tool/internal/wsrsrc/` (loader, substitution,
  auto-include, manifest, validator). Do NOT extend `internal/wsprompt`
  (that is the go:embed bundle; rsrc is deliberately a distinct, call-time,
  filesystem-backed mechanism — keep them separate to avoid coupling the binary
  version to text changes).
- `rsrc/` tree lives at `agents-plugin/rsrc/` (ships with the plugin
  distribution alongside `skills/`; materialized into the plugin cache;
  `WS_RSRC_ROOT` points dogfood at the repo copy). This is an existing-deliverable
  subdir, not a new root module.
- Reuse, do not reinvent:
  - Frontmatter parsing: model the parser on `internal/wsdoc/frontmatter.go`
    (multi-type YAML-subset: scalars + `- item` lists + sub-maps), NOT on
    `wsprompt.stripFrontmatter` (scalar-only `map[string]string` would silently
    drop the list-valued `includes`/`variables`). `wsdoc.frontmatter` is
    unexported and path-based, so copy its logic adapted to take a string and
    return `(frontmatter, body)`; do not import it. No YAML dep exists in `go.mod`
    (only `modernc.org/sqlite` + transitive) — a hand-rolled minimal parser is the
    settled choice; do not add a YAML dependency.
  - `sha256` + `hex` hashing and `\r\n`→`\n` normalization mirror
    `wsprompt.ContentSHA256` / `normalizePromptHashContent` so hashes are stable
    cross-platform.
  - Path-escape guards mirror `wsprompt.isBareStem` (reject `/`, `\`, `..`) for
    playbook names and include names.
- Schema-version gate is the ONLY runtime compatibility decision. Hashes are for
  CI tree-sync and load-time integrity (partial-materialization detection), never
  for compatibility gating.
- Forbidden: embedded fallback copy of any rsrc text in the binary; silent
  degradation on manifest/schema mismatch; coupling the binary version to rsrc
  text edits (text-only edits must ship without a binary bump — so the schema
  version changes only when the schema shape changes, not on content edits).

## Integration Test Instructions

- Boundary: Go unit tests in `internal/wsrsrc` (this is pure logic + filesystem
  IO — per impl-playbook, pure parsing/compat logic gets tests-first; IO-bound
  loading gets implement-then-test).
- New test file(s): `internal/wsrsrc/*_test.go`.
- Required coverage:
  - Loader: base load, harness-variant load, declared-variable substitution
    (provided value substituted; undeclared variable in context ⇒ error;
    declared-but-unprovided ⇒ defined behavior, error or empty — pick and test).
  - Auto-include: declared include concatenated at load; dangling include ⇒ error;
    include cycle ⇒ error (if includes can nest).
  - Manifest/schema-version: compatible passes; incompatible fails loud; missing
    manifest fails loud; manifest-listed file missing ⇒ loud; loaded-file hash
    mismatch ⇒ loud.
  - `Validate(root)` on a crafted bad tree catches: missing required harness
    variant, undeclared substitution variable used in body, dangling include,
    manifest hash out of sync.
  - A test that runs `Validate` against the real committed `agents-plugin/rsrc/`
    tree and passes (this IS the CI tree check).
- Run: `cd agents-plugin-tool && go test ./internal/wsrsrc/... && go build ./...`
  Also run full `go test ./...` before reporting to catch cross-package breakage
  (expect none — additive package).

## Implementation Strategy Decisions

Settled — do not reopen (from the ticket Decisions / research / epic
Cross-Child Decisions):
- Plain-text tree loaded at call time; NOT go:embed, NOT Go raw literals.
- Two-command split is Phase 2; Phase 1 builds only the substrate.
- Manifest carries hashes + schema version; **compatibility = schema-version, not
  hash equality.**
- Loud, partial failure; no embedded fallback.
- `WS_RSRC_ROOT` overrides the load root (required in MVP for dogfood iteration).
- Fully custom schema (no agent/MCP-prompt standard binding).
- Harness set is claude + codex only (Gemini excluded); overlay files are the
  only structural-divergence mechanism; terminology/model tables are DATA (Phase
  2), never code.
- Frontmatter-declared auto-includes (atomicity), NOT caller-flag include
  selection.

## Rejected Alternatives

- `go:embed` for rsrc — rejected: couples binary version to text edits.
- Single unified tool with output-kind metadata — rejected (Phase 2 concern, but
  noted): risks full delegate prompts in lead context.
- Hash-equality compatibility gating — rejected: blocks text-only redistribution.
- Caller-flag include selection (`read(name, ["conventions"])`) — rejected: pushes
  the decision to the caller; auto-include gives authoring-time atomicity.
- Embedded fallback text on load failure — rejected: split-brain drift risk.
- Extending `wsprompt` — rejected: would entangle the call-time mechanism with the
  embedded bundle and its runtime.json hash coupling.

## Approach

- Define the on-disk schema + layout under `agents-plugin/rsrc/` with 1–2 sample
  playbooks (base + codex overlay + one include + one substitution variable) and a
  generated `manifest.json`.
- Build `internal/wsrsrc`: frontmatter parser → loader → substitution → include
  resolver → manifest reader + schema-version compat + integrity check → exported
  `Validate(root)`.
- Add a manifest (re)generation helper so editing the tree can refresh hashes
  deterministically (used by CI/dev; a Go test or small CLI under the package).
- Wire CI: CI already runs `go test ./...` from `agents-plugin-tool/`
  (`.github/workflows/ws-mcp-release.yml`), so the `Validate(agents-plugin/rsrc)`
  test hooks in automatically — no new script needed. Constraint: the
  `agents-plugin/rsrc/` tree + `manifest.json` MUST be committed in the same
  change as the validating test, or CI fails on the first push (the test needs a
  real tree to validate).

## Constraints

- Additive only: no MCP tool, no skill migration, no spawn/actor change.
- Cross-platform-stable hashing (normalize line endings).
- No new heavy dependencies without checking `go.mod` first.
- Keep `wsprompt` untouched.
- AI-authored content (sample playbooks, comments, manifest) is English-only.

## Out of scope

- `playbook.print` / `playbook.render` tools, harness selection, model config
  tables, delegation-tip injection, unknown-harness fallback (Phase 2).
- Real skill-body content migration (M2).
- The distribution-race prerequisites (`260523`, `260524`) — MVP proceeds against
  the dev override; note in the result if the Codex `rsrc/` cache materialization
  question can be confirmed now or stays open.

## Verification Contract

- `go test ./internal/wsrsrc/...` green, covering all cases in Integration Test
  Instructions.
- `go build ./...` and `go test ./...` green.
- `Validate(agents-plugin/rsrc)` passes on the committed tree.
- A deliberately broken fixture tree fails `Validate` for each of: missing
  variant, undeclared variable, dangling include, manifest hash drift, schema
  mismatch, missing manifest.
- Report whether Codex `rsrc/` cache materialization was confirmed or remains the
  open item carried from research.

## References
<!-- [Must] read before starting. [Maybe] consult if uncertain. -->
- `ai-docs/tickets/ready/260609-feat-ws-playbook-surface-mvp.md` — [Must] ticket
  Decisions/Constraints/Phase 1 (lead reads it; implementer reads THIS brief).
- `ai-docs/mental-model/prompt-bundle.md` — [Must] existing embedded-bundle
  mechanism to mirror style from and deliberately stay separate from.
- `ai-docs/mental-model/mcp-runtime.md` — [Maybe] for Phase 2 wiring context and
  runtime.json/version conventions.
- `agents-plugin-tool/internal/wsprompt/prompts.go` — [Must] reuse frontmatter /
  hashing / path-guard patterns.
- `agents-plugin/runtime.json` — [Maybe] semver-range/version metadata pattern.
- `ai-docs/spec/mcp-tools.md` (#260609-rsrc-playbook-distribution) — [Must]
  contract-first spec stem this phase implements.
