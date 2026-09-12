# Plan: 260908-bug-shipped-surfaces-carry-devenv-only-content — Phase 1: Declared binding anchor replaces the shipped constant

## Relevant Ticket Contract

- Add a `### Binding Anchor` parser (fail-open, two keys `anchor:` and
  `topics:`) to `AGENTS.md`, parsed exactly like `### Review Policy`
  (Decision 1); render the clause "read `<path>` when target touches
  `<topics>`" only when both keys parse, otherwise omit it.
- Rename the proceed fact/routes/warning/verdict label
  `migration_anchor`/`MigrationAnchor` → `binding_anchor`/`BindingAnchor`
  everywhere (Decision 2); resolver normalizes to `n/a` when the project
  declares no section, even if the lead supplies `missing`/`conflict`.
- Rewrite `lead-discuss.md` and `lead-proceed.md` (fact table + ask-first
  row under `judge: discussion-needed`) to the generic term, and mirror
  into `agents-plugin-wsflow/rsrc/`.
- Rewrite the three `workflow-skills.md` sentences, the `{#260625}` clause
  and route-vocabulary entry in `mcp-tools.md`, and the mental-model line;
  add the `## Spec Impact` paragraph under `{#260830-review-policy-config-surface}`.
- Declare devenv's own anchor (`260605-research-ws-native-subagent-pivot.md`,
  four topics) in `AGENTS.md`'s new `### Binding Anchor` section; point the
  `## Project Memory` "Migration anchor" bullet at that section instead of
  restating the path/topics.
- Verification (ticket-specified): `go test ./...` (`-count=1`) in
  `agents-plugin-tool/`; wsflow skill-bundle tests; a `route.resolve_implement`
  render against a scratch root with a sectionless `AGENTS.md` showing no
  ticket stem/topics in the Prep instruction; the same render in devenv still
  naming `260605` and its topics; `grep -rn 260605 agents-plugin
  agents-plugin-wsflow agents-plugin-tool` clean outside parser test
  fixtures; `grep -rn 'migration.anchor' ai-docs/spec agents-plugin
  agents-plugin-wsflow agents-plugin-tool` clean.
- Depends on `260908-feat-survey-plan-is-route-not-contract` Phase 1 —
  already landed (`.done/260908-feat-survey-plan-is-route-not-contract.md`,
  merged via `fe205f69`/`313a74d9`); the `{#260519}` paragraph in
  `workflow-skills.md` is already in its post-rewrite form, confirmed by
  reading it directly.

## Out of Scope

- Phase 2 (twelve point-leak sites: `lead-tune`, `lead-update-spec`,
  `doctor.go`, `lead-review` Landing Lens, `lead-bootstrap`,
  `scope_announcement.go`, `lead-scope-worktree.md`, `legacy_marker.go`,
  wsflow `AGENTS.template.md`, `mental-model-conventions.md`,
  `mcp-tools.md` bare citations, wsflow `lead-revive/SKILL.md`).
- Phase 3 (the Python downstream-neutrality guard test and the
  `skill-authoring.md` "Resolvable downstream" checklist item).
- The `260605` anchor's own content, the `agentId continuity — tip-only`
  `260605` citation at `ai-docs/mental-model/workflow-skills.md:38` (not
  one of the audited sites; leave untouched), and the `kang-sw-devenv`
  cache-directory/plugin-cache literals (explicitly out of scope per the
  ticket's Constraints).
- `proceed_resolver.go`'s `Migration Anchor` route/enum comment style and
  any other proceed fact beyond the anchor rename.

## Codebase Findings

- `agents-plugin-tool/internal/wsreview/agents_config.go` — the exact
  precedent to model the new parser on: `reviewPolicySectionRE` isolates a
  `### <Heading>` body up to the next `#{1,3} ` heading or EOF, per-field
  regexes extract `key: value` lines, and `ReadAgentsReviewPolicy(root)`
  never errors, defaulting every field when the file, section, or field is
  absent/malformed. Reuse this shape for a new
  `agents-plugin-tool/internal/wsreview/binding_anchor.go` (no existing
  binding-anchor file in the package) with `anchor: <path>` and
  `topics: <comma-separated phrases>`, both required to render.
- `agents-plugin-tool/internal/wsreview/agents_config_test.go` — the
  precedent test shape (`mustWriteAgentsMD` helper, one case per: absent
  file, absent section, absent fields, all fields present, fail-open on
  malformed value) to mirror for the four required Phase 1 test cases
  (missing file, missing section, one key only, both keys).
- `agents-plugin-tool/internal/mcp/session_state.go:381-391` —
  `implementTodoVerdict` struct; add a field here (e.g.
  `BindingAnchorClause string`, pre-rendered and empty when undeclared) so
  `implementPrepInstruction` (line 525-540) stays root-free/pure: it
  interpolates the field instead of reading `AGENTS.md` itself. The
  hardcoded guardrails constant at line 529 (`read the 260605 migration
  anchor when target touches plugin architecture, host-neutral migration,
  spawn-removal, or adapter boundaries`) is the exact string to replace
  with the conditional interpolation.
- `agents-plugin-tool/internal/mcp/session_state.go:1062-1108` (typed
  `route.resolve_implement` path) — `record.Root` is already read at line
  1063 before verdict construction at line 1086; fill the new verdict field
  there via a `wsreview` read of `record.Root`.
- `agents-plugin-tool/internal/mcp/session_state.go:1111-1135` (legacy
  top-level `route.resolve_implement` path) — has no `record` in scope
  today; per the ticket's Go-touch-points constraint, add
  `s.sessions.readState(sessionKey)` here (mirroring the typed path) before
  building the verdict at line 1128, so both call sites render the clause
  in a declaring project while the pure `deriveImplementTodos` helper
  (line 396-404, used by `TestDeriveOtherEnterTodos`/simple non-implement
  enters) stays unchanged and root-free.
- `agents-plugin-tool/internal/mcp/proceed_resolver.go` — full inventory of
  the rename (only file besides the two above matching
  `migration_anchor|MigrationAnchor|migration-anchor` repo-wide, confirmed
  by grep): `proceedGateFactsInput.MigrationAnchor` (line 42, JSON key
  `migration_anchor`), `normalizedProceedFacts.MigrationAnchor` (line 92),
  `parseProceedGateFacts` enum parse (line 264), `normalizeProceedFacts`
  default (line 375: `factOr(g.MigrationAnchor, "n/a")`), the
  conflict-forces-discussion warning (line 441-442), the two routes (line
  487-491: `anchor-discussion.migration-anchor-{missing,conflict}`), and
  the rendered condition (line 523: `"migration-anchor=" + n.MigrationAnchor`).
  All become `binding_anchor`/`BindingAnchor`/`binding-anchor` verbatim,
  same enum (`loaded|n/a|missing|conflict|unknown`), same route shape.
- `agents-plugin-tool/internal/mcp/proceed_resolver.go:9-13` (`proceedInput`)
  and `:314` (`resolveProceed`, pure) — per the ticket's plumbing
  constraint, add a Go-only field to `proceedInput` (not part of the
  lead-facing JSON contract), e.g. `AnchorDeclared bool`, and use it in
  `normalizeProceedFacts` to force `n.BindingAnchor = "n/a"` whenever
  `!input.AnchorDeclared`, regardless of the supplied
  `facts.gates.binding_anchor` value.
- `agents-plugin-tool/internal/mcp/session_state.go:1171-1210`
  (`handleEnterProceed`) — currently reads `record` (for the
  review-watermark nudge) only *after* calling `resolveProceed(input)` at
  line 1185/1200. This ordering must change: read
  `s.sessions.readState(sessionKey)` before `parseProceedInput`/
  `resolveProceed`, so `input.AnchorDeclared` can be set from a
  `wsreview` read of `record.Root` before resolving. The existing
  `wsreview.CheckpointNudge(context.Background(), record.Root)` call at
  line 1201 can reuse the same `record` lookup (no duplicate read needed).
- `agents-plugin-tool/internal/mcp/session_state_test.go` — every site the
  rename/no-declaration-case work touches:
  - Line 17: `implementPrepGuardrails` test constant hardcodes the old
    sentence; becomes a base sentence (no anchor clause) plus a
    separately-composed declared-anchor fixture for the one test that
    exercises the declared path.
  - Lines 156-174 (`TestDeriveImplementTodoInstructionsDelegatedSurvey`)
    and line 1994/1999 (`TestEnterImplementNewSchemaReturnsVerdictAndStoresAgenda`)
    concatenate `implementPrepGuardrails + ...`; these verdict literals
    don't set the new field, so after the change they exercise the
    no-declaration (clause-omitted) path — update the constant accordingly
    rather than these call sites.
  - Lines 176-214 (`TestDeriveImplementTodoInstructionsPrepGuardrails`) —
    the loop currently asserts `"260605 migration anchor"` is present
    (line 207) for all three depths without setting any anchor field on
    the verdict literal (line 199-205); per the ticket, update this to
    assert the generic base sentence and add a fourth explicit
    "no declaration" case (or split into a declared-fixture case plus a
    no-declaration case) so both branches of the new conditional are
    covered.
  - Lines 1077-1099 (`TestResolveProceedRoutes` cases "migration anchor
    missing stops" / "migration anchor conflict routes to discussion") —
    these build `proceedInput` via `parseProceedInput(tc.args)` then call
    `resolveProceed(input)` directly (line 1198-1202); once
    `AnchorDeclared` defaults `false` and forces `n/a`, these two cases
    need `input.AnchorDeclared = true` set on the parsed input before
    calling `resolveProceed`, or the test loop needs a new per-case
    `anchorDeclared bool` (default `true`) threaded into `input` between
    parse and resolve. Rename `migration_anchor` → `binding_anchor` in the
    args maps and `migration-anchor=` → `binding-anchor=` in the expected
    route/reason/condition strings for these two cases and the
    `TestProceedInputRejectsNonStringFactTypes`/route-table coverage if any
    other case sets the key.
  - Line 1104 (`"migration_anchor": "n/a"` case "discussion needed routes
    to discussion") — key rename only; behavior unaffected since it's
    already `n/a`.
  - Line 1612 (`proceedReadyArgs` shared fixture) — rename key
    `migration_anchor` → `binding_anchor`.
  - All other `t.TempDir()`-rooted tests in this file that go through
    `handleEnterProceed`/`handleEnterImplement` via a real `NewServer`
    write no `AGENTS.md`, so they get `AnchorDeclared=false` and the
    no-declaration defaults already implied by their existing fixtures —
    no fixture changes expected there, but re-run the full suite to catch
    any I missed.
- `agents-plugin-tool/internal/mcp/implement_resolver_test.go:484-491` —
  also constructs `implementTodoVerdict{...}` directly (no anchor field)
  and calls `implementPrepInstruction`; its assertions (line 492) don't
  check anchor text, so it is unaffected by the rename as long as the
  default (undeclared) verdict renders without an anchor clause.
- `agents-plugin/rsrc/lead-proceed/lead-proceed.md:58` — the ask-first row
  "Migration-anchor conflicts with the requested route" under
  `judge: discussion-needed`; `:103` — the `facts.gates` table row
  `` `migration_anchor` | `loaded\|n/a\|missing\|conflict\|unknown` ``.
  Both rewrite to the generic term; byte-identical mirror confirmed at
  `agents-plugin-wsflow/rsrc/lead-proceed/lead-proceed.md` (empty diff).
- `agents-plugin/rsrc/lead-discuss/lead-discuss.md:18` — "Architecture/
  migration/spawn-removal/adapter-boundary topics → read
  `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` before
  answering." Rewrite to name the hook, not the anchor, e.g. "Topics
  matching the project's declared `### Binding Anchor` (`AGENTS.md`) →
  read the declared anchor path before answering." Byte-identical mirror
  confirmed at `agents-plugin-wsflow/rsrc/lead-discuss/lead-discuss.md`.
- `ai-docs/spec/workflow-skills.md` — four sentences to reword (line
  numbers may drift ±1-2 after each edit; re-grep after each change):
  - Lines 311-313: "Migration topics such as plugin architecture,
    host-neutral migration, spawn-removal, or adapter boundaries load the
    native-subagent pivot anchor before the lead states a direction."
  - Lines 1115-1118: "For migration-sensitive targets, `lead-proceed`
    reads the native-subagent pivot anchor as an artifact-only check,
    reports `Migration Anchor` in the Routing Verdict, stops when the
    anchor is missing, and treats absent binding anchor decisions as
    missing settled decisions." — note this sentence already says
    "binding anchor decisions" mid-sentence; the rewrite should be fully
    consistent (no more "Migration Anchor"/"native-subagent pivot") and
    add the "with no declared anchor the fact normalizes to n/a" qualifier
    per the `## Spec Impact` text.
  - Lines 1129-1133: "`lead-implement` also loads the native-subagent
    pivot anchor before editing when the target touches plugin
    architecture, host-neutral migration, spawn-removal, or adapter
    boundaries. ... when the migration anchor is read, binding
    implementation constraints from the anchor are copied into the plan
    and the anchor is listed as a `[Must]` reference ..."
  - Line 1150: "...conversation freshness, migration-anchor checks, and
    user-facing discussion." → "...binding-anchor checks, ...".
  - `### Review Policy Config Surface {#260830-review-policy-config-surface}`
    starts at line 1239; the bullet list documenting the three
    `key: value` fields ends at line 1267, and the section's closing
    anchor is `{#260513-review-workflow-skill}` at line 1280 (a different,
    pre-existing anchor for that whole section — do not confuse with the
    new `{#260908-project-binding-anchor-declaration}` anchor the ticket's
    `## Spec Impact` paragraph carries). Insert the ticket's verbatim
    `## Spec Impact` paragraph after line 1267 (or after line 1272, before
    the unrelated `lead-review` depth-scaling paragraph at line 1274) as a
    sibling declaration in the same `AGENTS.md`-config-surface home.
- `ai-docs/spec/mcp-tools.md:336` — "conditional migration-anchor
  loading" (inside the Prep-guardrails sentence) → "binding-anchor loading
  when the project declares one". `:419` — route-vocabulary list entry
  `` `migration-anchor` `` → `` `binding-anchor` ``. (The two bare
  `260605`/`260617` citations at other lines in this file are Phase 2
  scope — leave untouched.)
- `ai-docs/mental-model/workflow-skills.md:46` — "Migration-sensitive
  workflow turns load `260605-research-ws-native-subagent-pivot` before
  answering or implementing; `lead-proceed` treats a missing anchor as a
  stop, and delegated `lead-implement` copies binding anchor constraints
  into the implementation plan before executor dispatch." Reword to the
  declared-anchor hook plus the `n/a` default, keeping both anchor tags
  (`{#260513-proceed-ticket-freshness-gate} {#260505-implementation-workflow-skills}`).
  Line 38 in the same file also cites `260605` (the "agentId continuity —
  tip-only" decision) but is not one of the ticket's audited sites — leave
  it untouched (see Out of Scope).
- `AGENTS.md:84-136` (this repo's own root file) — `## Workflow` currently
  holds `### Approval Protocol` (86), `### Branch Policy` (98),
  `### Review Policy` (107-124), `### Commit Rules` (126). Add a new
  `### Binding Anchor` subsection here (sibling to `### Review Policy`,
  matching the Spec Impact's "same tracked `AGENTS.md` home"), e.g.:
  ```
  ### Binding Anchor

  anchor: ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md
  topics: plugin architecture, host-neutral migration, spawn-removal, adapter boundaries
  ```
  Then reword `## Project Memory` item 3 (lines 32-34, currently "Migration
  anchor - read `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md`
  (under epic `260605-epic-ws-playbook-factory-pivot`) when the task
  touches plugin architecture, host-neutral migration, spawn-removal, or
  adapter boundaries.") to point at the new section instead of restating
  the path/topics, e.g. "**Binding anchor** - read the project's declared
  binding anchor (`## Workflow` → `### Binding Anchor`) when the task
  touches its declared topics." This is devenv's own declaration, not
  shipped text, so keeping the concrete `260605` path/topics inside the
  new section body (not in prose elsewhere) is correct and matches the
  ticket's own worked example under Decision 1.

## Implementation Plan

1. `agents-plugin-tool/internal/wsreview/binding_anchor.go` (new file):
   add `BindingAnchor` (or similarly named) struct with `Anchor string`,
   `Topics string`, and a `Declared bool` (or equivalent all-fields-present
   check), plus `ReadAgentsBindingAnchor(root string) BindingAnchor`
   modeled on `ReadAgentsReviewPolicy` in `agents_config.go`: a
   `### Binding Anchor` section regex (reuse the same "next `#{1,3} ` or
   EOF" stop pattern) and `anchor:`/`topics:` line regexes; both keys must
   parse non-empty for `Declared` to be true; any missing file/section/key
   fails open to `Declared: false`.
2. `agents-plugin-tool/internal/wsreview/binding_anchor_test.go` (new
   file): four cases per the ticket — missing file, missing section, one
   key only (anchor without topics, and topics without anchor), and both
   keys present — mirroring `agents_config_test.go`'s `mustWriteAgentsMD`
   helper pattern.
3. `agents-plugin-tool/internal/mcp/session_state.go`:
   a. Add a field to `implementTodoVerdict` (line 381-391) carrying the
      pre-rendered anchor clause (empty string when undeclared).
   b. Rewrite `implementPrepInstruction` (line 525-540) to build the
      guardrails sentence by conditionally splicing in the clause from the
      verdict field instead of the hardcoded `260605` sentence; keep the
      rest of the sentence (mental-model lookup, ancestor reads,
      `infra.read("impl-playbook")`) unchanged.
   c. In the typed `route.resolve_implement` path (line 1062-1108): after
      `record` is read (line 1063), call
      `wsreview.ReadAgentsBindingAnchor(record.Root)` and render the
      clause into the verdict passed at line 1086.
   d. In the legacy top-level `route.resolve_implement` path
      (line 1111-1135): add `s.sessions.readState(sessionKey)` (sessionKey
      already in scope from line 1054), render the same clause, and pass
      it into the verdict at line 1128.
   e. In `handleEnterProceed` (line 1171-1210): move the
      `s.sessions.readState(sessionKey)` call (currently line 1200) to
      before `parseProceedInput`/`resolveProceed`; read
      `wsreview.ReadAgentsBindingAnchor(record.Root).Declared` and set it
      on `input` before calling `resolveProceed(input)`; reuse the same
      `record` for the existing review-watermark nudge call.
4. `agents-plugin-tool/internal/mcp/proceed_resolver.go`: rename
   `MigrationAnchor`/`migration_anchor`/`migration-anchor` to
   `BindingAnchor`/`binding_anchor`/`binding-anchor` at every site listed
   in Codebase Findings (struct fields, JSON tag, enum parse, default,
   warning text, both route strings, condition string); add the
   `AnchorDeclared` field to `proceedInput` and the forced-`n/a`
   normalization in `normalizeProceedFacts` keyed on it.
5. `agents-plugin-tool/internal/mcp/session_state_test.go`: apply the
   rename (`migration_anchor` → `binding_anchor` in args maps,
   `migration-anchor=` → `binding-anchor=` in expected strings) at lines
   1080, 1083, 1085-1086, 1092, 1095, 1097, 1104, 1612; set
   `AnchorDeclared`/equivalent to true for the two anchor-missing/conflict
   `TestResolveProceedRoutes` cases (lines 1077-1099) so they still
   exercise the gate; update the `implementPrepGuardrails` constant (line
   17) and `TestDeriveImplementTodoInstructionsPrepGuardrails` (lines
   176-214) to add the no-declaration case and a declared-anchor fixture
   case, per Codebase Findings.
6. `agents-plugin/rsrc/lead-discuss/lead-discuss.md:18` and
   `agents-plugin/rsrc/lead-proceed/lead-proceed.md:58,103`: reword to the
   generic "project's declared binding anchor" phrasing; copy both edited
   files byte-for-byte into
   `agents-plugin-wsflow/rsrc/lead-discuss/lead-discuss.md` and
   `agents-plugin-wsflow/rsrc/lead-proceed/lead-proceed.md` (confirmed
   currently byte-identical, so a straight copy preserves that invariant).
7. `ai-docs/spec/workflow-skills.md`: reword the four sentences at
   (current) lines 311-313, 1115-1118, 1129-1133, 1150; insert the
   ticket's verbatim `## Spec Impact` paragraph (ending
   `{#260908-project-binding-anchor-declaration}`) into
   `### Review Policy Config Surface` after the three-bullet field list
   (current lines 1249-1267), before the unrelated `lead-review`
   depth-scaling paragraph and the section's `{#260513-review-workflow-skill}`
   closing anchor.
8. `ai-docs/spec/mcp-tools.md`: reword line 336 ("conditional
   migration-anchor loading" → "binding-anchor loading when the project
   declares one") and the `migration-anchor` list entry at line 419 →
   `binding-anchor`.
9. `ai-docs/mental-model/workflow-skills.md:46`: reword to the
   declared-anchor hook plus `n/a` default, keeping the existing anchor
   tags; leave line 38 untouched.
10. `AGENTS.md`: add `### Binding Anchor` under `## Workflow` (sibling to
    `### Review Policy`) declaring devenv's own `260605` anchor and four
    topics; reword `## Project Memory` item 3 to point at the new section
    instead of restating the path/topics.

## Verification Plan

- `cd agents-plugin-tool && go test ./... -count=1` (covers the new
  `wsreview` parser tests, the `session_state.go`/`proceed_resolver.go`
  renames, and the no-declaration/declared Prep-guardrail cases).
- Run the wsflow skill-bundle test suite (`agents-plugin-wsflow/tests/`,
  e.g. `pytest agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`) to
  confirm the mirrored `lead-discuss.md`/`lead-proceed.md` copies stay in
  parity.
- Manually render `route.resolve_implement`'s Prep instruction (via a
  focused Go test or a temporary `main`/test harness) against a scratch
  root holding only `ai-docs/` and a sectionless `AGENTS.md`; confirm the
  instruction contains no ticket stem and none of the four devenv topics.
- Render the same path against the devenv repo root itself (with the new
  `### Binding Anchor` section in place); confirm it still names `260605`
  and the four topics.
- `grep -rn 260605 agents-plugin agents-plugin-wsflow agents-plugin-tool`
  — expect matches only inside the new parser's test fixtures (and the
  pre-existing, out-of-scope `test_wsflow_skill_bundle.py` epic-number
  comment).
- `grep -rn 'migration.anchor' ai-docs/spec agents-plugin
  agents-plugin-wsflow agents-plugin-tool` — expect no matches.

## Escalations

- None.
