# Plan: 260904-refactor-enter-affordance-rename-route-opaque — Phase 1: rename enter.proceed/enter.implement -> route.resolve_*

## Relevant Ticket Contract

- Frozen rename (Decisions/Naming A, settled 2026-09-04):
  `enter.proceed` -> `route.resolve_proceed`, `enter.implement` ->
  `route.resolve_implement`. The `route.*` namespace keeps the intent noun
  out of the leaf entirely.
- **One-shot hard cut** (Constraints, epic invariant): no alias/transition
  window; every old-name consumer ships in-package and is rewritten
  atomically.
- **Routing behavior unchanged.** This phase is name/text only — the Go
  decoder, resolver logic, verdict output, and todo/agenda derivation are
  untouched.
- **Phase 2 is explicitly out of scope for this plan.** Phase 1 does not
  touch the published `inputSchema` shape (`target`/`facts`/`policy` fields
  stay exactly as they are) and does not move any contract prose into the
  skill bodies — only the tool `"name"` field and prose that names the tool
  by its old literal string change.
- Two tools stay two tools (L2 collapse already rejected) — no schema
  merging here.
- Spec Impact (ticket's own list): `ai-docs/spec/mcp-tools.md`
  `{#260625-session-state-tools}` and `ai-docs/spec/workflow-skills.md`
  `{#260505-proceed-routing-pipeline}` / `{#260505-implementation-workflow-skills}`
  — edits to existing anchors only, no new spec stem, no `{#slug}` heading
  changes (so no `renamed-spec` commit trailer).
- Migration-anchor check (`260605-research-ws-native-subagent-pivot`): **no
  conflict**. That anchor governs spawn-machinery removal / native-subagent
  routing; this ticket is an MCP tool-surface rename orthogonal to it — the
  pivot doc never names `enter.proceed`/`enter.implement`.

## Out of Scope

- **Phase 2** (opaque `params` schema hollowing, contract relocation into
  `lead-proceed`/`lead-implement` skill bodies) — companion phase in the same
  ticket, not this plan.
- **Redirect-guard behavior** for `status=unknown` — owned by companion bug
  ticket `260901-bug-enter-proceed-misplaced-facts-silent-unknown-status`.
- **`ai-docs/mental-model/mcp-runtime.md` and `ai-docs/mental-model/workflow-skills.md`**
  prose mentions of `enter.implement`/`enter.proceed`/`ws.enter.implement`
  (verified present, e.g. `mcp-runtime.md:52,54,188`,
  `workflow-skills.md:41,49,74,81,89,132`). Not in the ticket's Spec Impact
  list, and not in the prompt's grep-sweep acceptance boundary (code, specs,
  runtime.json, playbooks) — deferred to a follow-up doc-drift pass, same
  precedent as sibling layer ④'s Phase 1
  (`ai-docs/.plans/2026-09/04-1320-260903-refactor-mcp-verb-vocabulary-unification.md`,
  Out of Scope). Executor may opportunistically sweep these as a low-risk
  token swap but it is not required for acceptance.
- **`CHANGELOG.md`** (6 hits, lines 887-1146) — historical release notes
  describing past versions; not retroactively renamed, same treatment as
  `.done`/`.dropped` ticket prose.
- **`ai-docs/tickets/**`** (all statuses, including this ticket's own family
  `260901-research-enter-tool-direct-call-affordance-rename`,
  `260901-bug-enter-proceed-misplaced-facts-silent-unknown-status`, and
  `ai-docs/.plans/**`) — decision-record / point-in-time text describing what
  was true when written; not renamed.
- **Go handler identifiers** (`handleEnterImplement`, `handleEnterProceed`,
  `handleEnter`) — internal camelCase Go symbol names, not literal MCP
  tool-name strings. The verification grep targets dotted (`enter\.proceed`)
  and underscore (`enter_proceed`) forms, which these identifiers do not
  match. Leaving them keeps the diff surgical; renaming is optional cosmetic
  follow-up, not required for acceptance.
- **`agents-plugin/skills/*/SKILL.md` and `agents-plugin-wsflow/skills/*/SKILL.md`**
  — verified (`grep -rl` empty result) that no skill shim references
  `enter.proceed`/`enter.implement` literally; `lead-proceed/SKILL.md` only
  calls `ws/playbook.read(name: "lead-proceed")`. No skills-tree edit is
  needed, so `WSRSRC_REGEN_SKILLS`, `WS_REGEN_COMPOSED_SKILLS`, and
  `WS_REGEN_WSFLOW_SKILLS` are **not** required for this phase (only the rsrc
  manifest/mirror regen below is).
- `agents-plugin-tool/internal/mcp/session_state_test.go:1245`'s
  `wantText` literal and any other test string that is not itself a
  tool-name token but merely a comment describing behavior — swept
  mechanically along with the literal occurrences, no special handling.

## Codebase Findings

- `agents-plugin-tool/internal/mcp/server.go` — 4 literal sites: dispatch
  `switch params.Name` cases `case "enter.implement":` (L552) and
  `case "enter.proceed":` (L554); tool registration `"name"` fields at L3472
  (`enter.implement`) and L3561 (`enter.proceed`). Registration derives
  `runtime.json`/`tools/list` dynamically (`LeadToolNames()`), so once these
  two `"name"` fields change, downstream advertised-name surfaces follow
  automatically.
- `agents-plugin-tool/internal/mcp/session_state.go` — 10 literal sites:
  `const tool = "enter.implement"` (L1030, used in `fmt.Errorf("%s: ...", tool, ...)`
  — behavior-visible error-prefix), three legacy-branch literal error strings
  at L1087/L1091/L1095 (`fmt.Errorf("enter.implement: %w", err)`), the legacy
  dispatch call `s.handleEnter(id, "enter.implement", "implement", args, todos)`
  (L1107), `const tool = "enter.proceed"` (L1144, same error-prefix pattern),
  and four branch-action instruction strings that literally say "do not call
  enter.implement again." (L513, L515, L517, L519) — all behavior-visible
  text returned to the caller as part of the resolved verdict/next-instruction,
  not comments.
- `agents-plugin-tool/internal/mcp/proceed_resolver.go:355` — behavior-visible
  `next_instruction` text: `"... rebuild route context and rerun %s/enter.proceed
  for that ticket. ..."` inside `proceedNextInstruction`. Must become
  `%s/route.resolve_proceed`.
- `agents-plugin-tool/internal/mcp/implement_resolver.go:786` (comment,
  "Used by enter.implement's preflight...") and `:926` (behavior-visible
  `implementNextInstruction` text: `"... then re-invoke enter.implement.
  Suspected prior owner ..."`) — both need the token swap; L926 is caller-facing.
- Test files (mechanical literal-token rename, no logic change): 
  `agents-plugin-tool/internal/mcp/session_state_test.go` (36 occurrences —
  call-tool sites plus one `t.Fatal("ws.enter.implement schema not found")`
  message and one `wantText` assertion embedding
  `wsflow/enter.proceed` at L1245), 
  `agents-plugin-tool/internal/mcp/review_watermark_checkpoint_test.go` (13),
  `agents-plugin-tool/internal/mcp/implement_resolver_test.go` (1, comment),
  `agents-plugin-tool/internal/mcp/playbook_tools_test.go` (6 — these assert
  against printed `lead-proceed`/`lead-implement` playbook prose, so they
  must track whatever the rsrc-file edit in step 5 below produces, e.g. the
  literal backtick-quoted `` `enter.proceed` `` / `` `enter.implement` ``
  substrings at L2559, L2747, L2749, L2752, L2946, L2950).
- `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go:166,173` — a
  **coincidental, unrelated** occurrence: `TestSubstitutionMirrorRespectsWordBoundaries`
  uses the literal fixture string `ws/enter_implement` /
  `wsflow/enter_implement` purely as synthetic filler to prove the
  `ws:`/`ws/` mirror-substitution regex respects word boundaries around
  underscores — it is not a real tool-name consumer. Flagged as a risk
  signal: left as-is it would make the acceptance grep sweep report a false
  positive on `enter_implement`. Low-risk fix: rename the fixture token to a
  neutral placeholder (e.g. `sample_tool`) in both the `source` and `want`
  strings so the sweep is genuinely clean.
- `agents-plugin/runtime.json:17-18` and `agents-plugin-wsflow/runtime.json:20-21`
  — `"tools"` section keys `"enter.implement": ">=0.44.4-dev <0.45.0"` and
  `"enter.proceed": ">=0.44.4-dev <0.45.0"` in both files (curated, not
  generated); version-range value string is unchanged, only the key renames.
- `agents-plugin/rsrc/lead-proceed/lead-proceed.md` — 3 occurrences: L14
  ("Treat an `enter.proceed` verdict as authoritative..."), L24 ("...return
  without calling `enter.proceed`."), L32 ("Call
  `{{.McpNamespace}}/enter.proceed(session_key: <key>, target: ..., facts: ...)`.").
- `agents-plugin/rsrc/lead-implement/lead-implement.md` — 5 occurrences: L20,
  L21, L44, L47, L58 (all `{{.McpNamespace}}/enter.implement` calls or
  backtick-quoted `` `enter.implement` `` prose references).
- **Never hand-edit `agents-plugin-wsflow/rsrc/lead-proceed/lead-proceed.md`
  or `agents-plugin-wsflow/rsrc/lead-implement/lead-implement.md`** — verified
  byte-identical to the canonical `agents-plugin/rsrc/` copies today, guarded
  by `TestWsflowRsrcMirrorUpToDate`
  (`agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go:54`, `bytes.Equal`
  comparison against `shippedRsrcRoot()`). After editing the two canonical
  files above, regenerate in order: `WSRSRC_REGEN=1 go test
  ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest` (updates the
  file-hash entries for `lead-implement/lead-implement.md` and
  `lead-proceed/lead-proceed.md` in `agents-plugin/rsrc/manifest.json`,
  currently `683a7873...` / `db9290c3...`, guarded separately by
  `TestShippedManifestUpToDate` in `manifest_shipped_test.go`), then
  `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run
  TestRegenerateWsflowRsrcMirror` (byte-copies the whole canonical tree,
  including the refreshed `manifest.json`, into `agents-plugin-wsflow/rsrc/`).
  Both `-count=1` flags are mandatory — the test cache otherwise skips the
  write side effect and the regen silently no-ops.
- `ai-docs/spec/mcp-tools.md` — primary anchor `{#260625-session-state-tools}`
  (heading at L235): "Enter (typed mode switches)" paragraph (L271) and the
  per-mode subsections at L278 (`implement`) and L393 (`proceed`). Additional
  cross-reference token occurrences under other anchors that must get the
  same plain token swap (no prose rewrite, no heading change): L1274
  (`{#260830-review-watermark-checkpoint-nudge}`), L2005-2006 and L2038
  (`{#260830-review-watermark-ledger-tools}` /
  `{#260830-review-watermark-checkpoint-nudge}`). Do **not** rewrite the
  field-accepting-schema prose in L278-L393 beyond the tool-name token — that
  content (accepted `target`/`facts`/`policy` fields) is accurate through
  Phase 1 and only becomes stale once Phase 2 hollows the published schema.
- `ai-docs/spec/workflow-skills.md` — primary anchors
  `{#260505-implementation-workflow-skills}` (heading L743; occurrences
  L748, L772, L805) and `{#260505-proceed-routing-pipeline}` (heading L1008;
  occurrences L1014, L1105, L1117, L1125). Additional cross-reference
  occurrences under other anchors needing the same token swap: L574
  (`{#260505-planning-workflow-skills}`) and L1194-1195
  (`{#260513-review-workflow-skills}`).
- `agents-plugin/tests/test_skill_dispatch_contracts.py:17,18,31,35` — real
  in-package Python consumer (not historical): `assertIn` checks against
  `{{.McpNamespace}}/enter.proceed(session_key:`, `` "Follow `Next:` from
  `enter.proceed` exactly" `` , `{{.McpNamespace}}/enter.implement`, and the
  "`enter.implement` returns a `direct-edit` verdict" sentence — must be
  updated to the new tokens to match the rsrc-file edits in step 5.
  **Risk signal (pre-existing, unrelated)**: running this file today
  (`python3 -m unittest tests.test_skill_dispatch_contracts -v`) already
  FAILS at L15 —
  `self.assertIn("Route only; do not implement or plan here.", text)` — this
  sentence does not exist in the current `lead-proceed.md` content at all
  (confirmed by direct read); the assertion is stale from an earlier text
  revision, unrelated to this rename. Do not attempt to fix it as part of
  this phase; it will fail identically before and after the rename, so it
  must not be read as evidence the rename broke something.
- `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py` — verified no
  `enter.*` literal references (only skill-name-string assertions); no edit
  needed.

## Implementation Plan

1. Freeze the two literal token pairs: `enter.implement` ->
   `route.resolve_implement`, `enter.proceed` -> `route.resolve_proceed`.
2. Script a literal-string rename of the two pairs across:
   `agents-plugin-tool/internal/mcp/server.go` (4 sites),
   `agents-plugin-tool/internal/mcp/session_state.go` (10 sites),
   `agents-plugin-tool/internal/mcp/proceed_resolver.go` (1 site),
   `agents-plugin-tool/internal/mcp/implement_resolver.go` (2 sites — 1
   comment, 1 behavior-visible), and the four sibling `_test.go` files
   (`session_state_test.go` 36, `review_watermark_checkpoint_test.go` 13,
   `implement_resolver_test.go` 1, `playbook_tools_test.go` 6). No handler
   logic changes — string literals only.
3. Update `agents-plugin/runtime.json` and `agents-plugin-wsflow/runtime.json`
   `"tools"` section keys (2 each; version-range values unchanged).
4. Update `ai-docs/spec/mcp-tools.md` and `ai-docs/spec/workflow-skills.md`:
   token-swap the tool name at all listed occurrences (primary anchors plus
   the review-watermark/planning/review cross-references); no `{#slug}`
   heading edits, no rewrite of the accepted-field prose.
5. Update the two canonical playbook files
   `agents-plugin/rsrc/lead-proceed/lead-proceed.md` (3 sites) and
   `agents-plugin/rsrc/lead-implement/lead-implement.md` (5 sites). Then run,
   in order: `cd agents-plugin-tool && WSRSRC_REGEN=1 go test
   ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest` and
   `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run
   TestRegenerateWsflowRsrcMirror`. Do not hand-edit
   `agents-plugin-wsflow/rsrc/lead-proceed/` or `lead-implement/`.
6. Update `agents-plugin/tests/test_skill_dispatch_contracts.py:17,18,31,35`
   to the new tokens (leave the unrelated pre-existing L15 failure alone).
7. Rename the coincidental non-tool fixture token in
   `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go:166,173`
   (`ws/enter_implement` / `wsflow/enter_implement`) to a neutral placeholder
   (e.g. `ws/sample_tool` / `wsflow/sample_tool`) in both the `source` and
   `want` strings of `TestSubstitutionMirrorRespectsWordBoundaries`, so the
   final grep sweep has no incidental false positive.
8. Run the verification sweep (below) and confirm clean.

## Verification Plan

- `cd agents-plugin-tool && go test ./... -count=1` — must be green (covers
  Go unit/integration tests including `main_test.go` runtime-contract checks
  and `wsrsrc`'s manifest/mirror drift guards
  `TestShippedManifestUpToDate` / `TestWsflowRsrcMirrorUpToDate`).
- Grep sweep for zero residual old-name references, scoped to
  `agents-plugin-tool/`, `agents-plugin/` (excluding `CHANGELOG.md` and
  `ai-docs/tickets/**`, `ai-docs/.plans/**`, `ai-docs/mental-model/**`,
  `ai-docs/ref/**`), and `agents-plugin-wsflow/`:
  `grep -rnE 'enter\.(proceed|implement)|enter_proceed|enter_implement' agents-plugin-tool agents-plugin/runtime.json agents-plugin/rsrc agents-plugin/tests ai-docs/spec agents-plugin-wsflow/runtime.json agents-plugin-wsflow/rsrc`
  must return no hits.
- Optional (not required for acceptance, but should be run to confirm no
  *new* Python failure): `cd agents-plugin && python3 -m unittest
  tests.test_skill_dispatch_contracts -v` — expect exactly the one
  pre-existing unrelated failure (`test_proceed_keeps_implementation_route_only`,
  stale "Route only; do not implement or plan here." assertion); a failure
  naming `enter.proceed`/`enter.implement` tokens would indicate step 6 was
  missed.
- Manual/tool-level: start `ws-mcp` and confirm `tools/list` shows
  `route.resolve_implement` and `route.resolve_proceed`, with no
  `enter.implement`/`enter.proceed` entries.

## Escalations

- None.
