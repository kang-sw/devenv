# Brief: 260620-capability-config — Phase 2 (tier-derived native model hint)

## Intent

Collapse the fixed three-var model-hint surface (`{{.LightModel}}` /
`{{.CoreModel}}` / `{{.DeepModel}}`) that delegate playbooks hand-pick into a
single `{{.RoleModel}}` var resolved from the playbook's own `tier:`
frontmatter. After Phase 1 the model config is keyed by the capability
vocabulary, so a playbook's declared `tier:` (`small`/`medium`/`large`/`xlarge`)
resolves directly to a concrete per-harness model. This removes the
author-picks-an-alias-name step: the playbook declares one role-model var and the
tool derives the concrete model from the tier it already declares.

## Scope Boundary

**Phase 2 only.** In scope: the `playbook_tools.go` render-var layer, the 9
delegate playbook sources under `agents-plugin/rsrc/`, the regenerated
`agents-plugin-wsflow/rsrc/` mirror, and the affected Go tests.

**Explicitly deferred to Phase 3** (do NOT touch): caller-facing prose in
`lead-tune`, `lead-workflow-manual`, `mcp-tools.md`, the `🚧` spec callout, and
the `named-agent-runtime` / `mcp-runtime` mental models. Phase 1 already deferred
the mental-model updates here on purpose (the vocabulary is only fully settled
after Phase 3). Phase 2 changes code + playbook template vars + tests only.

## Caller-Visible Contract

- Delegate playbooks expose exactly one model var, `{{.RoleModel}}`, resolved to
  the concrete model string for the playbook's `tier:` on the current harness.
- For every existing model-var playbook the resolved model is **unchanged** from
  today (verified correspondence below), so `playbook.render` /
  `playbook.print` output for these playbooks is byte-identical except the var
  name in the source and the (still-identical) substituted value.
- `recommended-tier: <tier>` continues to be appended by `withRecommendedTier`
  exactly as before (no change to that path).
- `{{.LightModel}}` / `{{.CoreModel}}` / `{{.DeepModel}}` are no longer injected
  or reserved; a playbook still referencing them must fail loudly as an
  undeclared/unprovided var (existing `substitutePlaybookVars` behavior). No
  read-compat shim for the old var names is wanted — they are internal authoring
  surface, not persisted user data.

## Contract Instructions

All Go changes are in `agents-plugin-tool/internal/mcp/playbook_tools.go` unless
noted.

1. **`resolveModelVars` → tier-derived single-var resolver.** Replace the fixed
   three-entry resolver (lines ~85-104) with one that resolves a single
   `RoleModel` var from a capability `tier`:
   - New shape, e.g. `resolveRoleModelVar(harness, tier string, configOpts wsconfig.Options) map[string]string`.
   - Resolve via `wsconfig.ResolveAgentForHarnessConfig(configOpts, tier, "", "", harness)`
     (Phase 1 makes this accept capability vocabulary directly). On error or
     empty model, set `RoleModel` to `""` (same graceful-empty behavior the old
     resolver had per-alias).
   - Return `map[string]string{"RoleModel": model}`.
   - Update the doc comment (and the `playbookTerminologyTable` comment at
     lines ~22-23 that names `resolveModelVars`) to describe the single
     tier-derived var.

2. **Thread `tier` into `buildPlaybookVars`.** Add a `tier string` parameter
   (ticket-specified). In Layer 3 (lines ~166-171) call the new resolver with
   that tier instead of the old fixed resolver. Keep the layer ordering and the
   "declared-only injection" rule unchanged.

3. **Pass `pb.Meta.Tier` at the call site.** In `renderPlaybookBody`
   (line ~604) pass `recommendedTier` (which is `pb.Meta.Tier`, already read at
   line ~602) as the new `tier` arg to `buildPlaybookVars`. Do not re-read or
   re-derive the tier elsewhere.

4. **`reservedToolVarNames` (lines ~58-61).** Remove the `LightModel` /
   `CoreModel` / `DeepModel` entries; add `set["RoleModel"] = true`. Keep the
   surrounding terminology + implicit-var population unchanged.

5. **Migrate the 9 delegate playbook sources** under `agents-plugin/rsrc/`. For
   each, change the `variables:` list entry from its current model var to
   `RoleModel`, and change the body line `Alias model for this role: {{.XModel}}.`
   to `Alias model for this role: {{.RoleModel}}.` Do NOT change any `tier:`
   value — each is already correct (table below). Do NOT change any other line.

   | playbook | current var | tier: (unchanged) |
   |----------|-------------|-------------------|
   | `reference-discovery` | LightModel | small |
   | `implementer` | CoreModel | medium |
   | `mental-model-updater` | CoreModel | medium |
   | `plan-populator-survey` | CoreModel | medium |
   | `code-review-fit` | CoreModel | medium |
   | `code-review-test` | CoreModel | medium |
   | `plan-populator-research` | DeepModel | large |
   | `reviewer` | DeepModel | large |
   | `code-review-correctness` | DeepModel | large |

   Correspondence is exact (small↔light, medium↔core, large↔deep), so the
   resolved concrete model is preserved for all 9.

6. **Regenerate the wsflow mirror.** After editing the `agents-plugin/rsrc/`
   sources, run:
   `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`
   from `agents-plugin-tool/`. This rewrites the byte-identical
   `agents-plugin-wsflow/rsrc/` copies. Do not hand-edit the mirror.

## Integration Test Instructions

Boundary: the `mcp` package render path plus the `wsrsrc` mirror drift guard.

- Update `internal/mcp/playbook_tools_test.go`:
  - The reserved-var completeness assertion (~line 1123) lists
    `LightModel, CoreModel, DeepModel` — replace with `RoleModel`.
  - The synthetic fixture playbook (~lines 137-141) declaring `CoreModel` /
    `{{.CoreModel}}` — migrate to `RoleModel` and give the fixture a `tier:` so
    the derivation is exercised; assert `{{.RoleModel}}` substitutes the model
    resolved from that tier.
- Update `internal/mcp/mercenary_surface_test.go` (~lines 443-477): the comments
  reference `CoreModel`/`DeepModel`; the assertions check the rendered body
  surfaces `sonnet`/`gpt-5.5` (implementer, tier medium) and `opus` (reviewer,
  tier large). The substituted model strings are unchanged, so the assertions
  hold — update only the var-name wording in comments to `RoleModel` and keep
  the model-string assertions.
- The `wsrsrc` mirror drift guard (`wsflow_mirror_test.go`) must pass on a normal
  run after regeneration.

Run (from `agents-plugin-tool/`):
`go build ./... && go test ./internal/mcp/... ./internal/wsrsrc/... ./internal/wsconfig/...`
All must pass. Read full output before claiming success.

## Implementation Strategy Decisions

(Settled — do not reopen.)

- The model is derived from `pb.Meta.Tier`, not from a new caller-supplied arg.
  The tier is already in scope at the injection site; no new render param.
- No tier values change — the existing per-playbook `tier:` already matches the
  alias each playbook hand-picked, so the migration is behavior-preserving.
- One var name only: `RoleModel`. Do not keep the old three as deprecated
  aliases — internal authoring surface, loud failure is correct.
- Empty/absent tier resolves through `ResolveAgentForHarnessConfig`'s own
  empty-tier behavior (Phase 1: falls back to the `medium` slot). All 9
  playbooks declare a tier, so this is only a safety fallback, not a live path.

## Rejected Alternatives

- **Caller passes the model var in `context`**: rejected — the tier is the
  playbook's own declared property; deriving it keeps the author from
  hand-mapping tier→alias→var and is the whole point of the collapse.
- **Keep `{{.CoreModel}}` etc. as read-compat aliases**: rejected — these are
  authoring-time template vars in ws-owned playbook sources, not persisted user
  config; a drift shim adds surface for no user-data-safety benefit.

## Approach

1. Edit `playbook_tools.go`: new tier-derived resolver, thread `tier` through
   `buildPlaybookVars`, pass `pb.Meta.Tier` at the call site, fix
   `reservedToolVarNames` and the stale comments.
2. Migrate the 9 `agents-plugin/rsrc/` playbook sources (var + body line only).
3. Regenerate the wsflow mirror via `WS_REGEN_WSFLOW_RSRC=1`.
4. Update the three test spots; build; run the package tests; iterate to green.
5. Commit logical checkpoints on `implement/260620-capability-config`.

## Constraints

- Stay within Phase 2 scope; leave all Phase 3 prose/docs/mental-models/spec
  untouched.
- Do not change `tier:` values, `withRecommendedTier`, the terminology/namespace
  layers, or the layer-ordering invariant.
- Playbook source edits are prompt edits: apply the `lead-skill-authoring`
  invariant checklist to any changed Invariants/Constraints line. (None are
  expected — only `variables:` and the model-hint body line change — but honor
  the gate.)
- All authored content English. Commit `## AI Context` per repo rules.

## Out of scope

- Phase 3 docs/skills/spec/mental-model edits.
- Any `wsconfig` resolution-logic change (Phase 1 already keyed it by
  capability; Phase 2 only consumes it).
- `config.agents_tier` tool surface (already done in Phase 1).

## Details

- `ResolveAgentForHarnessConfig` signature (from `resolveModelVars` call site):
  `(opts wsconfig.Options, tier, _, _, harness string) (..., model string, ..., err error)`.
  Pass the capability tier as the second arg.
- `pb.Meta.Tier` is the capability tier string from playbook frontmatter;
  `pb.Meta.Variables` is the declared var-name list driving injection + substitution.
- Per-harness expected models (for test assertions, default config): medium →
  claude `sonnet`, codex `gpt-5.5`; large → claude `opus`.

## Verification Contract

- `go build ./...` exit 0.
- `go test ./internal/mcp/... ./internal/wsrsrc/... ./internal/wsconfig/...` all `ok`.
- `playbook.render`/`print` of each of the 9 playbooks substitutes a non-empty
  `{{.RoleModel}}` equal to the model the old alias var produced; no
  `{{.LightModel}}`/`{{.CoreModel}}`/`{{.DeepModel}}` remain in any rsrc source
  or mirror.
- wsflow mirror drift guard passes on a normal (non-regen) run.

## References
<!-- [Must] read before starting. [Maybe] consult if uncertain. -->
- `agents-plugin-tool/internal/mcp/playbook_tools.go` — [Must] render-var layer; the only Go file to edit.
- `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go` — [Must] regen command + drift guard.
- `agents-plugin/skills/lead-skill-authoring/SKILL.md` — [Must] invariant checklist for prompt/playbook edits.
- `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` — [Maybe] migration anchor: "model-name tables live in config", native hint preserved (Evidence 4); confirms direction, no binding constraint beyond what's above.
- `ai-docs/mental-model/named-agent-runtime.md` — [Maybe] model-alias/effort/backend-affinity invariants (unchanged by Phase 2).
