# Survey: 09-260609-feat-ws-playbook-surface-mvp

## Reusable Components

- `agents-plugin-tool/internal/wsprompt/prompts.go#L140-142` — `normalizePromptHashContent`:
  one-liner `\r\n`→`\n` replacement; copy verbatim into `wsrsrc` for cross-platform-stable hashing.

- `agents-plugin-tool/internal/wsprompt/prompts.go#L200-211` — `isBareStem`:
  path-escape guard rejecting `/`, `\`, `..`, empty, `.`; copy verbatim and apply to both playbook
  names and include names at load and validate time.

- `agents-plugin-tool/internal/wsprompt/prompts.go#L121-138` — `ContentSHA256` pattern:
  sha256 over sorted filepath list, NUL-separated path+content chunks, hex-encoded. Mirror this
  loop for per-file hash generation in the manifest writer; identical normalization ensures
  manifest hashes are stable cross-platform.

- `agents-plugin-tool/internal/wsdoc/frontmatter.go#L8-80` — `frontmatter()` (unexported):
  **already implements list-valued YAML-subset parsing** (scalar, `- item` lists, `key: value`
  sub-maps, quote stripping, inline-comment stripping via `cleanScalar`). Returns
  `map[string]any` where list fields come back as `[]string`. The function is unexported and in
  `wsdoc` — copy the logic into `wsrsrc` (do not depend on `wsdoc` internals). This is the
  correct starting model for the rsrc frontmatter parser.

## Existing Patterns

- **Env-gated root resolution**: see `agents-plugin-tool/internal/wsstate/paths.go#L84-95` —
  `CacheRoot` checks `WS_CACHE_HOME` env, then falls back to computed default. Mirror this
  two-path shape for `WS_RSRC_ROOT`: check env, then call the Phase 2 plugin-path resolver
  (stub for Phase 1). Env name `WS_RSRC_ROOT` is referenced in the brief and matches the
  `WS_CACHE_HOME` naming convention.

- **`t.TempDir()` fixture trees**: all internal packages (`wsconfig`, `wsstate`, `wsstore`,
  `wsprompt`) build ephemeral fixtures programmatically inside tests via `t.TempDir()` +
  `os.WriteFile`. No `testdata/` subdirectories exist anywhere. Use the same pattern for rsrc
  loader tests: write fixture trees in-test, not as committed testdata.

- **Schema-version constant in metadata structs**: see
  `agents-plugin-tool/internal/wsstate/paths.go#L22` — `const schemaVersion = 1` plus
  `SchemaVersion int json:"schema_version"` in both `ProjectMetadata` and `WorktreeMetadata`.
  Mirror this pattern: export `SupportedSchemaVersion` as a package-level constant and embed
  `SchemaVersion` in the manifest struct.

- **Relative path from test to committed asset**: see
  `agents-plugin-tool/internal/wsprompt/prompts_test.go#L293-295` —
  `os.ReadFile(filepath.Join("..", "..", "..", "agents-plugin", "runtime.json"))` to validate a
  committed file from inside a test. Use the same relative-path pattern in the
  `Validate(root)`-against-real-tree test:
  `filepath.Join("..", "..", "..", "agents-plugin", "rsrc")`.

- **MCP runtime info dispatch**: `agents-plugin-tool/cmd/ws-mcp/main.go#L38-40` — `runtime`
  subcommand already dispatches via `os.Args`. The Phase 2 MCP tool will slot into the `serve`
  path; no new top-level dispatch case is needed for Phase 1.

## Relevant Interfaces

- `agents-plugin-tool/internal/wsprompt/prompts.go#L23-38` — `Source`, `Resolved`, `BundleInfo`:
  do NOT extend or reuse; these are the embedded-bundle API. Define separate structs in `wsrsrc`
  (`PlaybookMeta`, `LoadedPlaybook`, `Manifest`) with no coupling to `wsprompt` types.

- `agents-plugin-tool/internal/wsprompt/prompts.go#L213-234` — `stripFrontmatter`:
  parses only `map[string]string` (scalar-only). Do NOT reuse for rsrc; use the `wsdoc`
  frontmatter pattern instead (see Reusable Components). The signature difference is the risk:
  callers of `stripFrontmatter` get flat strings; rsrc needs `[]string` for `includes` and
  `variables`.

- `agents-plugin-tool/go.mod#L1-21` — module `github.com/kang-sw/devenv`, go 1.23. Confirmed:
  **no YAML library** in `require` or `require` indirect blocks. Only `modernc.org/sqlite` and
  its transitive deps. Hand-rolled minimal parser confirmed as the correct call.

## Constraints

- `go.mod` has no YAML parser; the hand-rolled `wsdoc`-style frontmatter parser is the right
  approach (no new deps).

- CI (`ws-mcp-release.yml#L42`) runs `go test ./...` from `working-directory:
  agents-plugin-tool` — the new `internal/wsrsrc/` package is automatically covered; no new
  script or CI step is required. The `Validate(agents-plugin/rsrc)` test will be picked up by
  the existing gate as-is.

- `isBareStem` in `wsprompt` would accept `name.codex` (dots are not excluded). This is correct
  for playbook base names, but the overlay filename construction (`<name>.<harness>.md`)
  requires that the harness string also passes through `isBareStem` separately. The stem
  validation should be applied to the name and harness components independently, not to the
  combined overlay filename.

- `wsdoc.frontmatter()` uses `\n`-split without normalizing `\r\n` first; rsrc frontmatter
  parser should normalize `\r\n`→`\n` (as `wsprompt.stripFrontmatter` does) before splitting.

## Risk Signals

- `agents-plugin-tool/internal/wsdoc/frontmatter.go#L8` — Possible **reuse risk**: the
  function is unexported; `wsrsrc` cannot import it. The brief does not mention this file.
  If the implementer needs to diverge from `wsprompt.stripFrontmatter` for list-valued fields,
  this file is the reference model to copy — but copying is required, not importing. Low risk
  if noted upfront.

- `agents-plugin-tool/internal/wsprompt/prompts.go#L213-234` — Possible **contract risk**:
  `stripFrontmatter` returns `map[string]string` (scalars only). The brief says "mirror
  `wsprompt.stripFrontmatter` conventions" — taken literally this could lead to a scalar-only
  parser that silently drops `includes: [...]` list values. The `wsdoc.frontmatter` pattern
  (same repo, `map[string]any`) is the correct model for list-valued fields. Lead/implementer
  should confirm the brief means "mirror the STYLE" (sentinel delimiters, normalization) not
  the exact type.

- `.github/workflows/ws-mcp-release.yml#L42-43` — Possible **CI hook risk**: the `Run tests`
  step (`go test ./...`) runs BEFORE the `Validate plugin release contract` step, which checks
  `runtime.json` hashes. If the `Validate(root)` test added in `wsrsrc` reads
  `agents-plugin/rsrc/manifest.json` and that file doesn't exist yet at first commit, the test
  will fail CI. Phase 1 requires the committed `agents-plugin/rsrc/` tree (including
  `manifest.json`) to be present in the same commit as the test. Lead should confirm the
  commit ordering: rsrc tree + manifest generated first, test added in same or subsequent
  commit.

- `agents-plugin-tool/internal/wsagent/agent.go#L242-246` — Phase 2 seam only (noted, not a
  Phase 1 risk): `SelfWorkerStarter.StartAsyncCall` calls `os.Executable()` to locate the
  binary; `filepath.Dir(exe)` would yield `agents-plugin/bin/`. The rsrc default root at
  Phase 2 would resolve as `filepath.Join(filepath.Dir(exe), "..", "rsrc")` →
  `agents-plugin/rsrc/`. No existing helper abstracts this pattern; Phase 1 `ResolveRoot`
  should leave a documented stub comment naming this derivation for Phase 2 to fill.

## Opinion

- `wsdoc.frontmatter()` is a near-complete implementation of what rsrc needs and is in the
  same module. The brief doesn't mention it. Calling this out as it eliminates a non-trivial
  implementation decision and reduces risk of a subtle parser regression.

- No `testdata/` convention exists in the repo; the brief's fixture approach (`t.TempDir()` +
  programmatic writes) is the idiomatic choice.

- The CI gate is simple and already sufficient — adding `internal/wsrsrc` is purely additive
  and no workflow changes are needed beyond the committed `agents-plugin/rsrc/` tree and tests.
