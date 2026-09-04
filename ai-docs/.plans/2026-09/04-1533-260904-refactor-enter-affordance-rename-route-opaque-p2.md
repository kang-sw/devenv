# Plan: enter.* affordance rename — route.resolve_* + full-opaque published params — Phase 2: hollow published schema to opaque params + move contract to skills

## Relevant Ticket Contract

- Decision C (full opaque): client-visible input schema becomes `params: object`
  plus a skill pointer only — no residual documented hint fields (no target/
  facts/policy sub-schema, no enums, no per-field descriptions).
- Two tools kept (L2 rejected): `route.resolve_proceed` and
  `route.resolve_implement` each publish their own opaque `params` — no `mode`
  union.
- Internal auditability preserved: the Go decoder's full typed struct parse +
  validation stays internal and behaviorally unchanged; "no behavior change to
  routing."
- Acceptance (ticket, Phase 2): (a) decoder still parses/validates the typed
  struct, existing resolver/verdict tests green, routing output byte-unchanged;
  (b) relocated skill-body contract covers every field the resolver reads,
  cross-checked field-by-field against the typed struct so none is dropped; (c)
  a mis-shaped direct call surfaces the opaque pointer / redirect-guard message
  rather than a silent `status=unknown`.
- Single authoring pass on `lead-proceed`/`lead-implement` — this ticket holds
  the pen; layer ④'s token deltas are already applied (Phase 1 landed).
- Redirect guard (status=unknown ticket-path target → point at `ws:lead-proceed`)
  is NOT owned here — owned by companion ticket
  `260901-bug-enter-proceed-misplaced-facts-silent-unknown-status`. This phase
  only needs pointer-text/naming consistency with that future guard.
- Spec Impact: `mcp-tools.md` `{#260625-session-state-tools}` (rewrite published
  input-schema prose to opaque `params` + skill-pointer; typed validation stays
  internal) and `workflow-skills.md`
  `{#260505-proceed-routing-pipeline}`/`{#260505-implementation-workflow-skills}`
  (skill bodies now carry the real input contract) — edits to existing anchors
  only, no `{#slug}` change.

## Out of Scope

- Implementing the `status=unknown` redirect guard itself (companion bug ticket
  owns it) — only keep pointer text/skill-name phrasing consistent for it to
  reference later.
- Any change to `resolveProceed`/`resolveImplement`, `normalizeProceedFacts`,
  branch observation, or any routing/verdict computation — Phase 2 touches only
  the advertised `inputSchema` (server.go) and documentation (skill bodies,
  specs). The wire/runtime argument shape callers must send (top-level
  `session_key`/`target`/`facts`/`policy`/`format` keys) is **unchanged** — see
  Codebase Findings below for why.
- `proceed_resolver.go` / `implement_resolver.go` parse functions
  (`parseProceedInput`, `parseImplementInput`, and their sub-parsers) — no
  edits; they already implement exactly the contract being relocated to the
  skill bodies.
- `runtime.json` (already renamed in Phase 1, tool-name-only, no field data).
- Mental-model prose drift (`mcp-runtime.md` etc.) — already deferred by Phase 1
  to a follow-up idea ticket; not re-opened here.
- Phase 1's rename mechanics — already landed at 7e35db10.

## Codebase Findings

### Schema is pure documentation — no JSON-schema validator in the runtime

- `agents-plugin-tool/internal/mcp/proceed_resolver.go` and `implement_resolver.go`
  (`parseProceedInput` L102-123, `parseImplementInput` L180-205) parse
  `args map[string]any` by hand (manual type assertions + `parseEnumFact`/
  `parseObjectString`), reading `args["target"]`, `args["facts"]`,
  `args["policy"]`, `args["format"]` **at the top level** — no wrapping "params"
  key.
- No `jsonschema`/`gojsonschema` validation library is used anywhere in
  `agents-plugin-tool/internal/mcp/` (`grep -rn jsonschema` returns nothing) —
  the published `inputSchema` in `tools/list` responses is purely advisory
  documentation for the client/model; it is never used to validate an incoming
  tool call.
- Confirmed by existing test fixtures: `proceedReadyArgs`/`implementReadyArgs`/
  `proceedArgs` (`session_state_test.go` L1502-1624) build tool-call argument
  maps with `target`/`facts`/`format` as **top-level** keys, and
  `callToolWithKey` (L1693) sends that map directly as the MCP call's
  `arguments`.
- **Conclusion (high confidence, not escalating):** hollowing the schema to
  `params: object` is a metadata-only change. `params` in the new schema is a
  symbolic placeholder property name — it does not correspond to any argument
  key the decoder reads. Real callers keep sending top-level `session_key` /
  `target` / `facts` / `policy` / `format`, exactly as today; the skill body is
  the only place that now documents this. This is consistent with the ticket's
  explicit framing: "the Go decoder keeps the full typed struct parse +
  validation" (decoder untouched) and Phase 1's Result note ("the published
  `inputSchema`... left untouched — Phase 2 owns the schema hollowing" — scoped
  to the *published* schema only).

### Current published schema to hollow

- `agents-plugin-tool/internal/mcp/server.go#L3471-L3559` — `route.resolve_implement`
  tool registration. Full current field surface to remove from the published
  schema (all present in the internal typed struct — cross-checked field-by-field
  against `implement_resolver.go` below, nothing added or dropped):
  - `target`: `kind` (`ticket|inline|unknown`), `label`, `ticket_stem`,
    `ticket_path`, `scope_label`, `scope_slug` (all nullable strings/enums).
  - `facts.scope`: `span` (`single-file|multi-file|unknown`), `surface`
    (`internal|public-interface|cross-module|unknown`), `new_public_symbol`
    (`yes|no|unknown`), `new_type_contract` (`yes|no|unknown`), `test_surface`
    (`none|existing|new-files|unknown`), `explicit_delegation_request`
    (`yes|no|unknown`), `explicit_direct_edit_request` (`yes|no|unknown`).
  - `facts.complexity`: `change_points` (`clear|partially-known|unknown`),
    `reuse_points` (`confirmed|unconfirmed|not-applicable|unknown`),
    `strategy_shape` (`single-obvious|multiple-viable|unknown`),
    `side_effect_risk` (`low|moderate|high|unknown`), `cold_context`
    (`yes|no|unknown`).
  - `facts.risk`: `correctness`, `fit`, `test`, `security_or_contract` (all
    `low|moderate|high|unknown`).
  - `policy`: `low_ceremony_if_safe` (`yes|no|unknown`); `branch.merge_target`
    (string), `branch.allow_rename` (`yes|no|unknown`), `branch.merge_confirm`
    (`skip|ask|unknown`); `review.override`
    (`auto|lead-only|single|partitioned`); `docs.mode`
    (`standard|skip-with-reason|unknown`), `docs.reason` (string).
  - `format`: `text|json`.
  - `required`: `["session_key", "target"]`.
- `agents-plugin-tool/internal/mcp/server.go#L3560-L3615` — `route.resolve_proceed`
  tool registration. Full current field surface (cross-checked against
  `proceed_resolver.go` below):
  - `target`: `kind` (`ticket-path|inline|unknown`), `label`, `ticket_stem`,
    `ticket_path`.
  - `facts.ticket`: `ticket_missing` (`yes|no|unknown`), `has_ticket`
    (`yes|no|unknown`), `status`
    (`idea|todo|ready|done|dropped|unknown|n/a`), `category`
    (`epic|workset|other|n/a|unknown`), `actionable` (`yes|no|unknown`),
    `freshness`
    (`current|missing-settled-decisions|uncertain|n/a|unknown`), `phase`
    (string).
  - `facts.gates`: `discussion_needed` (`yes|no|unknown`), `needs_ticket`
    (`yes|no|n/a|unknown`), `scope_blocked`
    (`none|container-ticket|multiple-explicit-phases|too-broad|no-unfinished-phase|phase-already-complete|unknown`),
    `migration_anchor` (`loaded|n/a|missing|conflict|unknown`).
  - `facts.work`: `category`
    (`implementation|ticket_write|discussion|status_report|unknown`), `slice`
    (string).
  - `format`: `text|json`.
  - `required`: `["session_key", "target"]`.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L14-L81` — the typed
  Go structs (`implementInput`, `implementTargetInput`, `implementFactsInput`,
  `implementScopeFactsInput`, `implementComplexityFactsInput`,
  `implementRiskFactsInput`, `implementPolicyInput`,
  `implementBranchPolicyInput`, `implementReviewPolicyInput`,
  `implementDocsPolicyInput`) and their `parse*` functions (L180-418) — the
  exact field/enum source of truth for the checklist above; **do not edit**.
- `agents-plugin-tool/internal/mcp/proceed_resolver.go#L9-L48` — the typed Go
  structs (`proceedInput`, `proceedTargetInput`, `proceedFactsInput`,
  `proceedTicketFactsInput`, `proceedGateFactsInput`, `proceedWorkFactsInput`)
  and `parse*` functions (L102-280) — source of truth for the checklist above;
  **do not edit**.

### Existing schema tests that must be replaced (not extended)

- `agents-plugin-tool/internal/mcp/session_state_test.go#L1302-L1325` —
  `TestEnterProceedSchemaAdvertisesNullableFacts` asserts the current
  richly-nested nullable schema for `route.resolve_proceed`. This test's
  premise (nested `target`/`facts` properties exist) becomes false after
  hollowing; replace it with an opacity assertion (see Implementation Plan).
- `agents-plugin-tool/internal/mcp/session_state_test.go#L1327-L1365` —
  `TestEnterImplementSchemaRequiresTargetAndAdvertisesNullableFacts` — same
  situation for `route.resolve_implement`; replace.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L1367-L1405` — helper
  functions `objectProperties` and `assertNullableSchema`. `grep -n
  "objectProperties(\|assertNullableSchema("` shows every call site is inside
  the two tests above (10 and 11 hits respectively, all within L1302-1365) —
  once those two tests are replaced, both helpers become dead code; delete them
  too (no other caller in the package).
- All other tests that exercise `route.resolve_proceed`/`route.resolve_implement`
  behavior (`TestResolveProceedRoutes` L953, `TestProceedNextInstructions`
  L1222, `TestProceedInputRejectsNonStringFactTypes` L1293,
  `TestEnterProceedStoresVerdictAgendaAndTodos` L1407,
  `TestEnterProceedJSONIncludesRawVerdict` L1444,
  `TestEnterProceedWarningsAndErrors` L1469, `TestServeStdioEnterImplementVerdictLabels`
  L1768, `TestEnterImplementNewSchemaReturnsVerdictAndStoresAgenda` L1844, and
  the other `TestEnterImplement*`/`TestDeriveImplementTodo*` tests) call
  through `parseProceedInput`/`parseImplementInput`/`resolveProceed`/
  `resolveImplement` with top-level `target`/`facts`/`policy` argument maps —
  **do not change these test fixtures**; per the wire-format finding above they
  remain valid unchanged and must stay green (this is the "routing output
  byte-unchanged" acceptance bar).

### Skill-authoring doctrine tension (flag, not blocker)

- `ai-docs/manuals/skill-authoring.md` Layer Model: "Layer 1 — MCP schema:
  Input field names, types, enums... Model-accessible? Yes — via ToolSearch
  before call... Delete from playbook? Yes — restatement drifts." This is the
  general rule against restating tool schemas in playbooks.
- This ticket's Decision C deliberately breaks the Layer 1 premise for these
  two tools: after hollowing, the field contract is **no longer
  model-accessible via ToolSearch** (the published schema is opaque). The
  ticket's own text ("Relocate the real input contract... into the
  lead-proceed/lead-implement skill bodies") is a ticket-authorized, explicit
  exception to the general Layer 1 deletion rule for exactly these two tools —
  not a doctrine violation. Note this in the implementation commit's `## AI
  Context` so a future skill audit does not "fix" it by deleting the relocated
  contract as apparent Layer 1 restatement drift.

### Skill pointer text convention

- `agents-plugin-tool/internal/mcp/server.go#L2032` already uses the literal
  `ws:lead-tune` naming convention for a Go-side skill pointer ("Tuning manual
  & how-to: run the ws:lead-tune skill."). Reuse this exact convention
  (`ws:lead-proceed`, `ws:lead-implement`) in both the new tool `description`
  and the opaque `params` property `description`, so the companion bug
  ticket's future redirect-guard message can reference the same skill-name
  string without drift.

### Skill bodies (relocation targets)

- `agents-plugin/rsrc/lead-proceed/lead-proceed.md` (72 lines) — step 5 of `##
  On: invoke` currently reads `Call
  {{.McpNamespace}}/route.resolve_proceed(session_key: <key>, target: ...,
  facts: ...)` with no field detail (consistent with the wire-format finding —
  it never named a "params" wrapper). Add a new `## Fact Contract` section
  (see Implementation Plan) with the full `target`/`facts.ticket`/
  `facts.gates`/`facts.work`/`format` field table.
- `agents-plugin/rsrc/lead-implement/lead-implement.md` (216 lines) — `##
  Invariants` → `Execution` and `### 1. Route` (L38-58) already carry judgment
  guidance ("Policy rules:" L49-54) but no field/type/enum table. Add a new
  `## Fact Contract` section with the full `target`/`facts.scope`/
  `facts.complexity`/`facts.risk`/`policy.*`/`format` field table. Keep the
  existing "Policy rules:" bullets (L49-54) — they are Layer 3 judgment
  ("when/how to set"), complementary to the new field/type table, not
  redundant with it.
- Both files have wsflow mirrors at
  `agents-plugin-wsflow/rsrc/lead-proceed/lead-proceed.md` and
  `agents-plugin-wsflow/rsrc/lead-implement/lead-implement.md` — **never
  hand-edit**; regenerate via the commands in Implementation Plan step 7.

### Spec anchors to touch

- `ai-docs/spec/mcp-tools.md#L271-L296` (implement paragraph, `##
  Session State Tools {#260625-session-state-tools}`) — the sentence "It
  accepts `session_key`, a required `target` object, optional grouped
  `facts.scope` / `facts.complexity` / `facts.risk` objects, a small `policy`
  object, and optional `format: text|json`." (L279-282) documents the
  published-schema shape; rewrite it to state the published schema is opaque
  (`session_key` + `params: object` + skill pointer to `ws:lead-implement`)
  while the Go decoder still parses/validates that same field set internally
  — do not delete the field-set enumeration itself from the spec (spec prose
  is not the MCP schema), just reframe which layer publishes it.
- `ai-docs/spec/mcp-tools.md#L393-L406` (proceed paragraph) — same treatment
  for the sentence "It accepts `session_key`, a required `target` object,
  optional grouped `facts.ticket` / `facts.gates` / `facts.work` objects, and
  optional `format: text|json`." (L394-396), pointer to `ws:lead-proceed`.
- `ai-docs/spec/workflow-skills.md#L747-L748` (`##
  Implementation Workflow Skills {#260505-implementation-workflow-skills}`,
  first `route.resolve_implement` mention) — append one clause noting the
  published schema is opaque and `lead-implement`'s new `Fact Contract` section
  is now the authoritative field list.
- `ai-docs/spec/workflow-skills.md#L1008-L1017` and `#L1105` (`## Proceed
  Routing Pipeline {#260505-proceed-routing-pipeline}`, first/main
  `route.resolve_proceed` mentions) — same one-clause treatment pointing at
  `lead-proceed`'s new `Fact Contract` section.
- No `{#slug}` renames anywhere (ticket constraint) — edits are prose-only
  within existing anchors.

### Migration anchor check

- `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` — concerns
  spawn-machinery removal, harness-neutral prompt-factory reframing, and
  durable-subagent assumptions. Phase 2 touches only one MCP tool pair's
  published schema and two skill bodies' documented contract; no spawn/agent
  machinery or harness-detection code is touched. **No conflict** — if
  anything, relocating the input contract into the playbook aligns with the
  anchor's "prompt factory" / contracts-in-playbooks direction.

## Implementation Plan

1. **`agents-plugin-tool/internal/mcp/server.go` — hollow `route.resolve_implement`
   (L3471-3559).** Replace the `inputSchema` block's `properties` with exactly:
   ```go
   "properties": map[string]any{
       "session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
       "params": map[string]any{
           "type":        "object",
           "description": "Opaque implementation-routing input, constructed by ws:lead-implement. Inner step of that skill; not a direct entry point — see ws:lead-implement's Fact Contract for the full field set.",
       },
   },
   "required": []string{"session_key"},
   ```
   Update the tool `description` (L3473) to lead with the pointer, e.g.:
   `"Inner step of ws:lead-implement; not a direct entry point — params are constructed by that skill. Resolves normalized implementation facts and observed Git branch state into one deterministic implementation verdict, stores the 'implement' agenda blob, and replaces the todo list with the derived implement checklist."`
   Do not touch anything below the `inputSchema` block (dispatch switch,
   `handleEnterImplement`, resolver, tests referenced elsewhere stay wired to
   the same top-level `target`/`facts`/`policy`/`format` argument keys).

2. **`server.go` — hollow `route.resolve_proceed` (L3560-3615).** Same pattern:
   ```go
   "properties": map[string]any{
       "session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
       "params": map[string]any{
           "type":        "object",
           "description": "Opaque routing input, constructed by ws:lead-proceed. Inner step of that skill; not a direct entry point — see ws:lead-proceed's Fact Contract for the full field set.",
       },
   },
   "required": []string{"session_key"},
   ```
   Tool `description` (L3562) becomes:
   `"Inner step of ws:lead-proceed; not a direct entry point — params are constructed by that skill. Resolves deterministic proceed facts into one route verdict, stores the 'proceed' agenda blob, and replaces the todo list with the lead-proceed checklist."`

3. **`session_state_test.go` — replace the two schema tests (L1302-1365).**
   Replace `TestEnterProceedSchemaAdvertisesNullableFacts` and
   `TestEnterImplementSchemaRequiresTargetAndAdvertisesNullableFacts` with two
   opacity tests, e.g. `TestRouteResolveProceedSchemaIsOpaque` and
   `TestRouteResolveImplementSchemaIsOpaque`, each asserting via
   `toolPropertiesByName`/`callToolsList`:
   - `properties` has exactly the keys `session_key` and `params` (assert
     `target`/`facts`/`policy` are absent — the direct opacity proof the
     verification boundary requires).
   - `properties["params"]["type"] == "object"` and `properties["params"]` has
     no nested `"properties"` key (no residual hint fields).
   - `properties["params"]["description"]` (and the tool's own top-level
     `description`) contains the literal substring `ws:lead-proceed` (resp.
     `ws:lead-implement"`) — the pointer-text/naming-consistency proof for
     future coordination with the companion redirect-guard ticket.
   - `required == ["session_key"]`.
   Then delete the now-unused `objectProperties`/`assertNullableSchema` helpers
   (L1367-1405) — confirm with `grep -n
   "objectProperties(\|assertNullableSchema("` that no other test in the
   package still calls them before deleting.

4. **Leave `proceed_resolver.go`/`implement_resolver.go` untouched.** No parse
   function, struct, or enum list changes — this is the "byte-unchanged
   routing" acceptance bar. Confirm by re-reading the diff before commit: these
   two files should show zero changes.

5. **`agents-plugin/rsrc/lead-proceed/lead-proceed.md` — add `## Fact
   Contract`.** Insert a new top-level section (after `## Judgments`, before
   `## Doctrine`, matching the file's existing section order) containing:
   ```markdown
   ## Fact Contract

   `{{.McpNamespace}}/route.resolve_proceed`'s published schema is opaque
   (`params: object`); this table is the authoritative field contract the
   resolver reads. Send `session_key`, `target`, `facts`, and optional `format`
   as top-level call arguments.

   `target`
   | Field | Type | Notes |
   |-------|------|-------|
   | `kind` | `ticket-path\|inline\|unknown` | |
   | `label` | string\|null | Short target label; defaults from path/stem/kind when omitted. |
   | `ticket_stem` | string\|null | |
   | `ticket_path` | string\|null | |

   `facts.ticket`
   | Field | Enum |
   |-------|------|
   | `ticket_missing` | `yes\|no\|unknown` |
   | `has_ticket` | `yes\|no\|unknown` |
   | `status` | `idea\|todo\|ready\|done\|dropped\|unknown\|n/a` |
   | `category` | `epic\|workset\|other\|n/a\|unknown` |
   | `actionable` | `yes\|no\|unknown` |
   | `freshness` | `current\|missing-settled-decisions\|uncertain\|n/a\|unknown` |
   | `phase` | string (free text) |

   `facts.gates`
   | Field | Enum |
   |-------|------|
   | `discussion_needed` | `yes\|no\|unknown` |
   | `needs_ticket` | `yes\|no\|n/a\|unknown` |
   | `scope_blocked` | `none\|container-ticket\|multiple-explicit-phases\|too-broad\|no-unfinished-phase\|phase-already-complete\|unknown` |
   | `migration_anchor` | `loaded\|n/a\|missing\|conflict\|unknown` |

   `facts.work`
   | Field | Enum |
   |-------|------|
   | `category` | `implementation\|ticket_write\|discussion\|status_report\|unknown` |
   | `slice` | string (free text) |

   `format`: `text` (default) \| `json`. All fields are optional; unknown/null
   values are normalized by the resolver.
   ```
   Apply the invariant checklist qualitatively (context-free, non-redundant,
   doctrine-aligned) — this is a data table, not an invariant list, so it is
   exempt from the "one line" rule (same precedent as the existing `Reviewer
   table` in `lead-implement.md`).

6. **`agents-plugin/rsrc/lead-implement/lead-implement.md` — add `## Fact
   Contract`.** Insert after `## Invariants`, before `## On: invoke` (or as the
   first subsection the `### 1. Route` step-5 call can point back to):
   ```markdown
   ## Fact Contract

   `{{.McpNamespace}}/route.resolve_implement`'s published schema is opaque
   (`params: object`); this table is the authoritative field contract the
   resolver reads. Send `session_key`, `target`, `facts`, `policy`, and
   `format` as top-level call arguments.

   `target`
   | Field | Type | Notes |
   |-------|------|-------|
   | `kind` | `ticket\|inline\|unknown` | |
   | `label` | string\|null | |
   | `ticket_stem` | string\|null | |
   | `ticket_path` | string\|null | |
   | `scope_label` | string\|null | Selected implementation scope label. |
   | `scope_slug` | string\|null | Kebab-case branch suffix; ignored with a warning for ticket targets (deterministic word-key is authoritative). |

   `facts.scope`
   | Field | Enum |
   |-------|------|
   | `span` | `single-file\|multi-file\|unknown` |
   | `surface` | `internal\|public-interface\|cross-module\|unknown` |
   | `new_public_symbol` | `yes\|no\|unknown` |
   | `new_type_contract` | `yes\|no\|unknown` |
   | `test_surface` | `none\|existing\|new-files\|unknown` |
   | `explicit_delegation_request` | `yes\|no\|unknown` |
   | `explicit_direct_edit_request` | `yes\|no\|unknown` (overrides all other scope facts to direct-edit when `yes`) |

   `facts.complexity`
   | Field | Enum |
   |-------|------|
   | `change_points` | `clear\|partially-known\|unknown` |
   | `reuse_points` | `confirmed\|unconfirmed\|not-applicable\|unknown` |
   | `strategy_shape` | `single-obvious\|multiple-viable\|unknown` |
   | `side_effect_risk` | `low\|moderate\|high\|unknown` |
   | `cold_context` | `yes\|no\|unknown` |

   `facts.risk`
   | Field | Enum |
   |-------|------|
   | `correctness` | `low\|moderate\|high\|unknown` |
   | `fit` | `low\|moderate\|high\|unknown` |
   | `test` | `low\|moderate\|high\|unknown` |
   | `security_or_contract` | `low\|moderate\|high\|unknown` |

   `policy`
   | Field | Enum/Type |
   |-------|-----------|
   | `low_ceremony_if_safe` | `yes\|no\|unknown` |
   | `branch.merge_target` | string; required only while already on an implementation branch |
   | `branch.allow_rename` | `yes\|no\|unknown` (defaults to allowed) |
   | `branch.merge_confirm` | `skip\|ask\|unknown` (defaults to ask) |
   | `review.override` | `auto\|lead-only\|single\|partitioned` |
   | `docs.mode` | `standard\|skip-with-reason\|unknown` |
   | `docs.reason` | string; required when `docs.mode=skip-with-reason` |

   `format`: `text` (default) \| `json`. Groups and fields are optional;
   unknown/null values are normalized by the resolver. MCP observes Git branch
   state itself — do not pass observed branch facts as caller policy.
   ```
   Keep the existing `Policy rules:` bullets (`### 1. Route`, current
   L49-54) unchanged — they state judgment (when/how to set a value), which
   this new table does not restate.

7. **Regenerate the wsflow mirror and both manifests (never hand-edit the
   mirror).** From `agents-plugin-tool/`:
   ```
   WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -run TestGenerateRealManifest -v -count=1
   WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror
   ```
   Only these two are needed — the changed files
   (`agents-plugin/rsrc/lead-proceed/lead-proceed.md`,
   `agents-plugin/rsrc/lead-implement/lead-implement.md`) live under `rsrc/`,
   not `skills/`, so `WSRSRC_REGEN_SKILLS=1` is not required unless a
   `skills/` tree file also changes (it should not, for this phase). Confirm
   `agents-plugin-wsflow/rsrc/lead-proceed/lead-proceed.md`,
   `agents-plugin-wsflow/rsrc/lead-implement/lead-implement.md`, and both
   `manifest.json` files changed as a result, and that they were not hand-edited.

8. **`ai-docs/spec/mcp-tools.md` — rewrite the two published-schema sentences.**
   At L279-282 (implement) and L394-396 (proceed), replace the "It accepts
   `session_key`, a required `target` object, optional grouped ... " sentences
   with prose stating: the published `inputSchema` is opaque (`session_key` +
   `params: object` + a pointer to `ws:lead-implement`/`ws:lead-proceed`); the
   Go decoder still internally parses and validates the same field set
   described in that skill's `Fact Contract` (name it), unchanged in behavior.
   Do not touch surrounding prose describing routing/verdict semantics
   (unaffected by this phase).

9. **`ai-docs/spec/workflow-skills.md` — one-clause pointer updates.** At
   L747-748 and at L1008-1017/L1105, append a short clause noting the tool's
   published schema is now opaque and the named skill's `Fact Contract` section
   is the authoritative field list. Do not restate the field table itself here
   (it lives in the skill body per the ticket's relocation decision).

10. **`references.trace` sweep.** Run the project's reference-trace check (or a
    targeted `grep -rn "route\.resolve_proceed\|route\.resolve_implement"
    ai-docs/spec/`) to confirm no other spec anchor names these tools with
    schema-field assumptions that also need updating.

11. **Commit `## AI Context` note.** Record the skill-authoring Layer 1
    exception rationale (finding above) in the commit message so a future skill
    audit does not misclassify the relocated Fact Contract sections as
    restatement drift.

## Verification Plan

- `cd agents-plugin-tool && go test ./... -count=1` — must be fully green,
  including `TestShippedManifestUpToDate` and `TestWsflowRsrcMirrorUpToDate`
  (drift guards over the regenerated mirror/manifest from step 7), and every
  existing `TestResolveProceedRoutes` / `TestEnterProceed*` /
  `TestEnterImplement*` / `TestDeriveImplementTodo*` test unchanged and passing
  (proves routing output byte-unchanged — none of those fixtures were touched).
- The two new/replaced schema tests
  (`TestRouteResolveProceedSchemaIsOpaque`/`TestRouteResolveImplementSchemaIsOpaque`,
  step 3) pass and directly assert: `target`/`facts`/`policy` are absent from
  `properties`, `params` is a bare `object` with no nested `properties`, and
  `required == ["session_key"]` — this is the "advertised schema is now opaque
  params" proof the caller's verification boundary requires.
- Pointer-text consistency proof (in place of the not-yet-implemented redirect
  guard, which is explicitly out of scope here): the same two tests assert the
  tool `description` and `params.description` contain the literal skill-name
  string `ws:lead-proceed`/`ws:lead-implement`. This is the achievable-in-Phase-2
  half of "surfaces the pointer... message" — the runtime
  status=unknown-mis-shaped-facts redirect itself remains owned by
  `260901-bug-enter-proceed-misplaced-facts-silent-unknown-status` and is not
  testable here since it does not exist yet; do not attempt to simulate it.
  Existing hard-decode-error tests (`TestEnterProceedWarningsAndErrors`
  L1469-1500 invalid-enum case, `TestProceedInputRejectsNonStringFactTypes`
  L1293-1300) already prove structurally invalid calls surface a loud Go error
  rather than a silent pass-through — confirm these stay green unchanged as
  the "not silent" half of the criterion that Phase 2 can actually own.
  Confirm no new test needed to prove the *field-level* silent-drop case (a
  well-formed-but-misplaced flat `facts` payload silently defaulting to
  `unknown`) is unchanged in Phase 2 — that remains the companion ticket's
  scope, unmodified here.
- Field-by-field cross-check: before committing, diff the new skill-body Fact
  Contract tables (steps 5-6) against the struct field lists in
  `implement_resolver.go#L14-L81` and `proceed_resolver.go#L9-L48` one more
  time — every `json:"..."` tag must appear in the corresponding table, and
  vice versa (no invented fields).
- `git diff --stat -- agents-plugin-tool/internal/mcp/proceed_resolver.go
  agents-plugin-tool/internal/mcp/implement_resolver.go` should show no changes
  — an explicit, mechanical proof of "decoder untouched."
- Manual read-through of `agents-plugin/rsrc/lead-proceed/lead-proceed.md` and
  `agents-plugin/rsrc/lead-implement/lead-implement.md` against
  `ai-docs/manuals/skill-authoring.md`'s Fresh-Reader Audit bar is recommended
  but not required by this ticket's acceptance criteria; if run, apply only
  `fix` findings and do not remove the Fact Contract sections as apparent
  Layer 1 drift (see the flagged tension above).

## Escalations

- None.
