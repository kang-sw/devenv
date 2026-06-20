# Brief: config.prompt() self-documenting override listing (260619 Phase 2)

Ticket: `260619-feat-ws-config-prompt-tool-self-doc` (Phase 2).
Spec: `260620-config-prompt-override-tuning-tools` (the `config.prompt()` listing is
the `Planned 🚧` callout — promote it to implemented after this lands).
Branch: `implement/260619-config-prompt-tool` (worktree `/home/swkang/devenv-wt-config-prompt`).
Phase 1 (`config.prompt.set`) is already shipped on this branch (`24e7e0d1`).

## Goal

Add a no-arg `config.prompt()` MCP tool that returns a **data listing** (not a
manual): tree-scan the shipped rsrc playbook tree for declared override markers,
report each override-point's id + short `desc` + the current override values per
harness bucket and the scope each resolved from, and end with a one-line pointer to
the `ws:lead-tune` skill (which owns the how-to manual). No tuning manual is
rendered here — keep it a lean data surface.

## Exact marker grammar to scan (already in tree)

Open marker (the only thing this tool scans):
```
<!-- ws:override:<pointId> desc="..." -->
```
Close marker `<!-- ws:/override:<pointId> -->` is irrelevant to the listing.
Prefix constants exist: `overrideOpenPrefix = "<!-- ws:override:"` at
`agents-plugin-tool/internal/mcp/playbook_tools.go:293`. The existing
`parseOverrideMarkerPointId` (`playbook_tools.go:303`) extracts only the pointId and
**discards `desc`** — the listing needs `desc` too, so add a sibling parser; do NOT
change `parseOverrideMarkerPointId` (the render engine depends on it).

Real production marker: `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md:66`
(`DelegationSection desc="lead delegation eagerness and context-saving stance"`).
Mirror file: `agents-plugin-wsflow/rsrc/lead-workflow-manual/lead-workflow-manual.md`.

## Storage / resolution facts (from Phase 1, do not relitigate)

- Override key is `prompt.<pointId>.<harness>`; harness bucket is one of
  `claude | codex | all` (`*` is the caller-facing spelling of `all`).
- Read current value+scope with `wsconfig.Resolver.Get(sessionKey, key)` →
  `ResolvedValue{Value, Scope}` (`agents-plugin-tool/internal/wsconfig/resolver.go:76`).
  An unset key returns `Value:"" Scope:ScopeBuiltin` — treat empty `Value` as "no
  override set" and omit it from the listing.
- Build the resolver exactly as Phase 1 does (ambient Options, not root-aware):
  `adapter := sessionConfigAdapter{s: s.sessions}` then
  `wsconfig.NewResolver(wsconfig.Options{}, nil, adapter, adapter)`
  (`server.go:531-532`).
- Session-scope override values are only visible when a `session_key` is supplied —
  mirror `config.show` (`server.go:454`, schema `server.go:2101`), which takes an
  **optional** `session_key` and annotates resolved scopes when present.

## rsrc tree walk

- Resolve the rsrc root with `resolveRsrcRoot(rootOverride)`
  (`playbook_tools.go:499`) → `wsrsrc.ResolveRoot()` (honors `WS_RSRC_ROOT`).
- Walk with `filepath.Walk(root, ...)` (same pattern as
  `wsrsrc/manifest.go:43`); for each non-dir `.md` file, `os.ReadFile`, split on
  `\n`, and scan each trimmed line for the open marker. Lexical walk order makes
  dedup deterministic.
- Dedup by pointId across files (a point may appear in both the agents-plugin and
  the test tree, but within one tree it is unique); keep the first non-empty `desc`.

## Implementation

### 1. `agents-plugin-tool/internal/mcp/playbook_tools.go`

Add, near the marker engine:

- `func parseOverrideOpenMarkerDesc(trimmed string) (pointId, desc string, ok bool)`
  — reuse the `overrideOpenPrefix` + `-->` suffix checks; take the first
  whitespace token as `pointId` (same logic as `parseOverrideMarkerPointId`); then
  extract `desc="..."` from the remainder (find `desc="`, then the next `"`).
  Return `desc:""` when absent. `ok=false` for a non-marker line or empty pointId.
- `type overridePointDecl struct { PointId string; Desc string }`
- `func scanOverridePoints(rsrcRoot string) ([]overridePointDecl, error)` —
  `filepath.Walk` the root, scan `.md` files for open markers via the new parser,
  dedup into `map[string]string` (pointId→desc, first non-empty desc wins), and
  return a slice **sorted by PointId**. Propagate walk/read errors.

Keep these functions pure (root in, data out) so they unit-test without a Server.

### 2. `agents-plugin-tool/internal/mcp/server.go`

Add `case "config.prompt":` (place it adjacent to `config.prompt.set` at
`server.go:488`, after that case):

- `sessionKey, _ := params.Arguments["session_key"].(string)` (optional; trim).
- `rootOverride, _ := params.Arguments["root_override"].(string)`;
  `rsrcRoot, err := resolveRsrcRoot(strings.TrimSpace(rootOverride))`; on err →
  `toolTextResponse(req.ID, "", err)`.
- `points, err := scanOverridePoints(rsrcRoot)`; on err → error response.
- Build the resolver (ambient Options, as above). For each point, for harness in
  `[]string{"claude", "codex", "all"}`: `rv, _ := resolver.Get(sessionKey, "prompt."+p.PointId+"."+harness)`;
  when `strings.TrimSpace(rv.Value) != ""` record `(harness, rv.Scope)` (and the
  value, for the JSON form).
- Render **text** as the canonical output: one block per point — `pointId` line,
  the `desc`, then either `(no overrides set)` or one indented line per set
  override `harness=<h> scope=<scope>`. End the whole listing with a single
  pointer line, e.g.: `Tuning manual & how-to: run the ws:lead-tune skill.`
- Support optional `format: "json"` mirroring `config.show` (structured list of
  `{pointId, desc, overrides:[{harness, scope, value}]}`), so tests can assert
  structured fields. Text is the default.
- No extra role check — the `config.*` prefix gate in `roleAllowsTool`
  (`server.go:2704,2706`) already blocks delegate/leaf keys, and the keyed gate at
  `server.go:340-346` fires only when a non-lead key is presented (a keyless caller
  passes, exactly like `config.show`).

Add the schema entry in `tools()` (next to `config.prompt.set` at
`server.go:2126`):
```go
{
    "name":        "config.prompt",
    "description": "List every declared prompt override-point (id + description) found in the shipped playbook tree, with any current override values and the scope each resolved from. Read-only; lead-only via the config.* prefix gate. Points to the ws:lead-tune skill for the tuning how-to.",
    "inputSchema": map[string]any{
        "type": "object",
        "properties": map[string]any{
            "session_key":   stringProperty("Optional lead session key. When supplied, session-scope overrides are included and annotated."),
            "format":        stringProperty(`Optional output format. Use "json" for structured output.`),
            "root_override": stringProperty("Optional rsrc root override (test/advanced use); when omitted the shipped rsrc tree is scanned."),
        },
    },
},
```

### 3. `agents-plugin/runtime.json` AND `agents-plugin-wsflow/runtime.json`

Add `"config.prompt": ">=0.30.0-dev <0.31.0"` to the `tools` map in **both** files
(next to `config.prompt.set`). This is REQUIRED: `cmd/ws-mcp/main_test.go`
`TestRuntimeCapabilitiesCommandReportsLauncherContractSurface` asserts the
`runtime.capabilities` tool set EXACTLY equals the runtime.json `tools` keys, and
`LeadToolNames()` auto-derives from `tools()`. Omitting either entry breaks the
build/test.

### 4. Visibility

Do NOT add `config.prompt` to `noAgentHiddenTool` (`server.go:2777`). Prompt
overrides are mode-neutral, so the listing is visible in both full-ws and
agentless wsflow — same stance Phase 1 took for `config.prompt.set`.

## Tests (`agents-plugin-tool/internal/mcp/prompt_override_test.go`)

Reuse existing infra: `useLeadProfile(t)`, `buildOverrideTestTree(t)` (declares two
points with desc — `SeedSection` "a seeded override point" and `ExtSlot` "an empty
extension slot", at `prompt_override_test.go:49-56`), `t.Setenv("WS_RSRC_ROOT", rsrcRoot)`,
`callLogin`/`parseLoginResponse` for a lead key, `callToolOnce`, `toolText`.
Model the setup on `TestOverrideProductionPath` (`prompt_override_test.go:383`).

- `TestConfigPromptListEnumeratesDeclaredPoints`: seed one override via the
  resolver (`prompt.SeedSection.claude`, ScopeSession, the lead key — as at
  `prompt_override_test.go:407-414`), then `callToolOnce(... "config.prompt", {session_key})`.
  Assert the text contains: `SeedSection`, `a seeded override point`, `ExtSlot`,
  `an empty extension slot`, the seeded override's `claude` + `session` annotation,
  and the `lead-tune` pointer. Assert `ExtSlot` shows no overrides set.
- Unit-test `scanOverridePoints` / `parseOverrideOpenMarkerDesc` directly against a
  small temp tree (desc present, desc absent, non-marker line, dedup across files).
- Add `config.prompt` to the gating closure in
  `TestCapabilityScopedKeyGatesTools` (`session_auth_test.go`, the `assertGateError`
  set): a delegate-scoped key calling `config.prompt` must return `-32601`.

## Verification

- `cd agents-plugin-tool && go build ./...`
- `go test ./internal/mcp/... ./cmd/ws-mcp/...` (the cmd package exercises the
  runtime.json exact-equality contract — must stay green).

## Out of scope

- Orphan overrides (a `prompt.*` value with no declared marker) are NOT listed; the
  listing is keyed on declared markers per the spec. Do not add orphan discovery.
- No tuning manual text is rendered here (it belongs to 3b `ws:lead-tune`).
