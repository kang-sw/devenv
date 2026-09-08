---
title: "Shipped ws surfaces carry devenv-only tickets, paths, and migration vocabulary"
parent: 260605-epic-ws-playbook-factory-pivot
spec:
  - 260505-planning-workflow-skills
  - 260513-proceed-ticket-freshness-gate
  - 260519-proceed-implementation-dispatch-precheck
  - 260625-session-state-tools
  - 260810-scope-announcement-idea-inclusion
  - 260830-review-policy-config-surface
related-mental-model:
  - workflow-skills
  - mcp-runtime
related:
  260605-research-ws-native-subagent-pivot: the devenv-only anchor the shipped Prep guardrail, lead-discuss, and spec name
  260611-chore-lead-discussion-gap-discipline: origin; its Phase 2 asked shipped playbooks to "honor the AGENTS.md migration-anchor read rule"
  260908-feat-implement-skip-survey-for-localized-ticket-target: its stub-plan rule is phrased against the Prep reads rather than the anchor because of this leak
  260908-feat-survey-plan-is-route-not-contract: block-depends, Phase 1; it rewrites the {#260519} paragraph and this ticket edits the rewritten text
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: d7ffdd0dbd5daee3
sage-review-completeness-reviewed: d7ffdd0dbd5daee3
---

# Shipped ws surfaces carry devenv-only tickets, paths, and migration vocabulary

## Background

Full audit on 2026-09-08 of every surface a downstream project receives:
`agents-plugin/rsrc/**` (53 files, read in full), both `skills/` trees,
`runtime.json` and manifests, `agents-plugin-wsflow/`, every string
`agents-plugin-tool/` emits to an agent, the embedded conventions, and
`ai-docs/spec/*.md` as the contract for that behavior. Nineteen sites
depend on something that exists only in this repository. A downstream
lead is told to read a ticket it does not have, to point users at a
research ticket that is not in its tree, to run a doctor check that
fails on every project without an `agents-plugin/` directory, and to
treat this repository's migration state as a generic ws rule.

Root cause, from `git log -S`: `AGENTS.md` gained a devenv session-start
"Migration anchor" bullet on 2026-05-05. Ticket
`260611-chore-lead-discussion-gap-discipline` Phase 2 (2026-06-15,
`c0f8b768`) asked the shipped lead playbooks to "honor the AGENTS.md
migration-anchor read rule", and the implementer copied the bullet, its
ticket path, and its four topics verbatim into `lead-discuss`,
`lead-proceed`, `lead-implement`, their wsflow mirrors, the spec, and the
mental model. When `11f52ae0` (2026-06-27) moved the lead-implement prose
into Go todo strings and dropped the sentence, the same-day review fix
`29bdfd69` restored it as a Go constant and pinned it in
`session_state_test.go`, turning a Markdown leak into test-locked runtime
output. Later diet passes generalized `lead-proceed.md` to the
`migration_anchor` fact but never touched the Go constant, `lead-discuss`,
or the spec. The other sites share the mechanism: authors writing shipped
text from inside devenv reach for the nearest artifact, a ticket stem, a
repo path, or a repo convention.

No guard fires on this. `fresh-reader-audit` checks internal consistency;
the `skill-authoring.md` "Context-free" criterion means readable without
the surrounding file, which a hardcoded ticket path satisfies; the
Downstream Consistency Sweep checks ws against wsflow byte equality only;
no test scans shipped text for ticket stems or devenv paths, and three
tests pin the leaks instead. `AGENTS.md` `## Architecture Rules` now
states the downstream-first rule in binding terms (landed with this
ticket's creation); this ticket removes the existing leaks and adds the
mechanical guard behind that rule.

### Sites

Migration-anchor family (one concept, one design decision):

| site | text | severity |
|---|---|---|
| `agents-plugin-tool/internal/mcp/session_state.go:529` guardrails const, pinned `session_state_test.go:17,:207` | "read the 260605 migration anchor when target touches plugin architecture, host-neutral migration, spawn-removal, or adapter boundaries" | breaking |
| `agents-plugin/rsrc/lead-discuss/lead-discuss.md:18` (+ wsflow mirror) | "read `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` before answering" | breaking |
| `ai-docs/spec/workflow-skills.md:1093-1096` | `lead-proceed` "reads the native-subagent pivot anchor ... stops when the anchor is missing" | breaking |
| `ai-docs/spec/workflow-skills.md:1107-1112` | `lead-implement` "loads the native-subagent pivot anchor ... listed as a `[Must]` reference" | breaking |
| `ai-docs/spec/workflow-skills.md:311-313` | `lead-discuss` topics "load the native-subagent pivot anchor" | misleading |
| `ai-docs/mental-model/workflow-skills.md:46` | source sentence the spec and playbook text were authored from | misleading |
| `agents-plugin/rsrc/lead-proceed/lead-proceed.md:58,103`; `proceed_resolver.go` `migration_anchor` fact and `anchor-discussion.migration-anchor-{missing,conflict}` routes | devenv term as generic gate, enum, and route names; degrades through `n/a` | cosmetic |

Point leaks (independent, mechanical):

| site | text | severity |
|---|---|---|
| `agents-plugin/rsrc/lead-tune/lead-tune.md:75` (+ mirror) | "point to research ticket `260611-research-ws-per-role-delegation-tuning-config`" | breaking |
| `agents-plugin/rsrc/lead-update-spec/lead-update-spec.md:22` (+ mirror) | "Read `agents-plugin/rsrc/lead-write-spec/lead-write-spec.md`" as a raw path where every other cross-playbook read uses `playbook.read` | breaking |
| `agents-plugin-tool/internal/wsdoc/doctor.go:23` | `ws-mcp doctor` and `smoke` require an `agents-plugin/` directory at the project root | breaking |
| `agents-plugin/rsrc/lead-review/lead-review.md:100-103` | built-in default Landing Lens: "repo conventions (AGENTS.md, skill-authoring, wsflow-mirroring where applicable)" | misleading |
| `agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md:20` | "do not reintroduce `claude-plugin/`" | misleading |
| `agents-plugin-tool/internal/mcp/scope_announcement.go:35`, pinned `scope_announcement_test.go:62` | banner "See ai-docs/ref/worktree-ticket-scope.md" | misleading |
| `agents-plugin/rsrc/lead-scope-worktree/lead-scope-worktree.md:14,15,30` (+ mirror) | three prose pointers to `ai-docs/ref/worktree-ticket-scope.md` (`## Unreproduced Hazard`, `## Cross-Scope git mv`), the banner's file again | misleading |
| `agents-plugin-tool/internal/wsdoc/legacy_marker.go:408`, pinned `legacy_marker_test.go:234` | advisory "being retired by 260726-refactor-retire-spec-planned-marker-mechanism" | misleading |
| `agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md:134-142` | Equivalence note citing commit `599fb453` and ticket `260825` | misleading |
| `agents-plugin-tool/internal/wsdoc/conventions/mental-model-conventions.md:91` | "`## Architecture Rules` in `CLAUDE.md`" analogy | cosmetic |
| `ai-docs/spec/mcp-tools.md:495`, `:169` | bare ticket-number citations "(260605 pivot constraint)", "(260617 obscurity ...)" | cosmetic |
| `agents-plugin-wsflow/skills/lead-revive/SKILL.md:8` | "Recover your ws `session_key`" inside the wsflow package | cosmetic |

Clean: every other rsrc playbook and loose document, both skills trees
beyond the sites above, both `runtime.json` and manifests, `bin/`, the ws
and wsflow `rsrc/` trees (byte-identical), the bootstrap `WORKFLOW.md` and
ws `AGENTS.template.md`, the embedded conventions beyond one line, all
test fixtures (synthetic stems), the 103 `server.go` tool descriptions,
and the plugin-runtime, named-agent-runtime, and claude-compatibility
specs.

## Decisions

- **A project declares its binding anchor in `AGENTS.md`; shipped text
  names the hook, never the anchor.** `AGENTS.md` gains a
  `### Binding Anchor` section holding two `key: value` lines,
  `anchor: <repo-relative path>` and `topics: <comma-separated phrases>`,
  parsed fail-open exactly as `### Review Policy` is
  (`{#260830-review-policy-config-surface}`).
  The Go Prep guardrail reads it and renders "read `<path>` when the
  target touches <topics>" only when both keys parse; with no section the
  clause is omitted, and the remaining guardrail sentence (mental-model
  lookup, ancestor reads, `infra.read("impl-playbook")`) is unchanged.
  `lead-discuss` and the three spec sentences say "the project's declared
  binding anchor" and point at the section. devenv declares `260605` with
  its four topics there, and the `## Project Memory` "Migration anchor"
  bullet points at the section instead of restating the path.
  - Rejected: a `config.list` key. `config.list` is user-scoped
    (`~/.ws/config.json` and scoped overrides), while the anchor is a
    tracked structural fact of the project, the same class as
    `review-track`; it belongs in the tracked file next to it.
  - Rejected: keeping the Go constant and only removing the stem. The
    four topics are devenv's migration vocabulary; without the stem the
    sentence still tells every project to read an anchor it never declared.
  - Rejected: a multi-anchor list. The proceed gate models one anchor
    today (`migration_anchor`); one declared anchor keeps the fact and the
    guardrail a single clause. A project needing more folds them into one
    document.
- **The proceed fact and its routes are renamed to the generic term.**
  `facts.gates.migration_anchor` becomes `binding_anchor` with the same
  enum, `anchor-discussion.migration-anchor-{missing,conflict}` become
  `anchor-discussion.binding-anchor-{missing,conflict}`, the Routing
  Verdict label `Migration Anchor` becomes `Binding Anchor`, and the
  resolver normalizes the fact to `n/a` when the project declares no
  section so no downstream project can hit `missing` by default. The
  lead still reports the fact as today when a section exists: `loaded`
  when the declared anchor was read, `missing` when the declared path
  does not exist or the target's topic is declared but the anchor holds
  no decision on it, `conflict` when the anchor's decision contradicts
  the requested route; the stop and `lead-discuss` routes for those two
  values keep their behavior.
  - Rejected: keeping the name because it degrades through `n/a`. The name
    is the leak; every downstream lead reading the fact table learns a
    devenv concept as a ws gate.
- **Point leaks are deleted or routed, never gated on detecting devenv.**
  `doctor` and `smoke` stop checking for `agents-plugin/`; the built-in
  `lead-review` Landing Lens says "the repo's own conventions (`AGENTS.md`
  and any authoring manual it names)" and devenv carries its
  `skill-authoring`/`wsflow-mirroring` clause in its own
  `ai-docs/_review.local.md` `## Landing Lens`, accepting that the file is
  gitignored and the clause is per-working-copy (re-added on a fresh
  clone) because `lead-review` reads no tracked lens home and adding one
  is not this ticket's problem; the scope banner and the
  `lead-scope-worktree` prose keep only the `git sparse-checkout` commands
  and state the hazard inline in one sentence each; the legacy-marker advisory drops
  the stem; `lead-tune` states "not a supported knob" with no pointer;
  `lead-update-spec` calls `playbook.read(name: "lead-write-spec")`;
  `lead-bootstrap` loses the `claude-plugin/` sentence; the wsflow template
  Equivalence note is deleted; the convention analogy names `AGENTS.md`; the two
  bare citations in `mcp-tools.md` are deleted; `lead-revive`
  in wsflow says wsflow.
  - Rejected: keeping the `agents-plugin/` doctor check behind a "source
    repository" detection. Detecting devenv inside shipped code is the same
    leak with a conditional around it; devenv's CI checks its own layout.
- **The guard fails on text that resolves only inside this repository,
  and on nothing else.** The criterion is the one `AGENTS.md` rule 4
  states: text an agent sees must not depend on something a downstream
  project does not have. A Python test in `agents-plugin/tests/` scans
  `agents-plugin/rsrc/**`, `agents-plugin/skills/**`, their
  `agents-plugin-wsflow/` equivalents, `agents-plugin-tool/internal/wsdoc/
  conventions/**`, and the string literals (double-quoted and backtick,
  comments excluded) of every non-test `.go` file under
  `agents-plugin-tool/`, and fails a line when:
  - a ticket-stem-shaped token (`26[0-9]{4}-[a-z][a-z0-9-]+`) resolves to a
    ticket in this repository under any status directory including
    `.done/` and `.dropped/`, or to a `{#...}` anchor in `ai-docs/spec/`;
    a token that resolves to neither is an example and passes;
  - a path under `ai-docs/` names a specific file that exists as a
    git-tracked file in this repository and is not among the files
    `lead-bootstrap` installs (the test derives that set from the bootstrap
    skill's scaffold list); a name that resolves only to a runtime-created,
    gitignored local file such as `lead-review`'s `_review.local.md` is a
    live config contract, not a devenv file, and passes;
    naming a directory (`ai-docs/tickets/`, `ai-docs/manuals/`,
    `ai-docs/ref/`) or a placeholder path (`<status>/<stem>.md`) passes;
  - a bare six-digit `26[0-9]{4}` token, not immediately followed by a
    `-[a-z]` stem tail (so it is a citation, not the head of a stem the
    first rule already judged), whose date prefix any ticket in this
    repository carries (the "(260605 pivot constraint)" citation form; the
    `260421-feature-name` example stems in the convention documents are
    stems, not bare citations, and pass);
  - the line contains a commit hash introduced by "commit", one of this
    repository's own names as a whole token (`agents-plugin`,
    `agents-plugin-tool`, `agents-plugin-wsflow`, `claude-plugin/`,
    `install.sh`, `skill-authoring`, `wsflow-mirroring`, each matched only
    when not preceded or followed by `[a-z0-9-]`, so the Go token
    `"lead-skill-authoring"` in `wsrsrc/skills_mirror.go` is a different
    name and passes), or one of the migration phrases (`migration anchor`,
    `native-subagent pivot`, `spawn-removal`, `host-neutral migration`,
    `adapter boundar`, `retired Claude tree`, `Codex-first`).
  The scan scope is exactly the text an agent reads or receives: the
  four trees and the Go string literals listed above, walked over
  git-tracked files only so gitignored local files such as
  `agents-plugin/.local-devenv-runtime` cannot change the verdict.
  `bin/`, `runtime.json`, and the plugin manifests are not scanned: the
  launcher's `agents-plugin/.runtime` lookup is the installed package's
  own directory, the launcher's comments are not agent text, and the
  manifests' "Codex-first" describes the plugin itself, which every
  installing project has. Each failure prints file, line, the token, and
  which rule matched. There is no allowlist; a line that must
  legitimately contain one of the literal names is a reason to reword the
  line. The two resolution rules are stated against this repository's
  current contents, so the result is history-dependent by design: a
  shipped example stem starts failing the day a real ticket takes that
  stem, and that is the correct outcome.
  - Rejected: forbidding the `ai-docs/` directory names or every
    stem-shaped token. Run over the shipped trees that list flags 59 lines
    that are ws conventions the downstream project has: the
    `ai-docs/tickets/<status>/<stem>.md` output contract, sparse-checkout
    patterns, the bootstrap-installed `ai-docs/manuals/` and `ai-docs/ref/`
    homes, and the example stems in the convention documents. Those are
    not devenv-only, so the guard must not flag them, and an allowlist to
    exempt them is where the next leak hides.
  - Rejected: a Go test under `wsrsrc`. The embedded loader sees only rsrc;
    the scan needs the skills trees and the Go sources too, and the Python
    suite already walks the repository.

## Constraints

- Phase order is fixed: the guard (Phase 3) fails on the tree until
  Phases 1 and 2 land; it is not added with an allowlist to land early.
- Landing order against siblings: Phase 1 edits the
  `{#260519-proceed-implementation-dispatch-precheck}` paragraph and the
  `implementPrepInstruction` constant after
  `260908-feat-survey-plan-is-route-not-contract` Phase 1 has rewritten
  that paragraph, and before or after
  `260908-feat-implement-skip-survey-for-localized-ticket-target`, which
  already declares that its stub clause follows this ticket's reworded
  text; whichever of the two lands second rebases its Prep-instruction
  and test-constant edits on the other's.
- Go touch points: `implementPrepInstruction` guardrails constant and
  `TestDeriveImplementTodoInstructionsPrepGuardrails` plus the
  `implementPrepGuardrails` test constant in `session_state_test.go`,
  and the plumbing the pure derivation lacks today: the parsed
  declaration rides `implementTodoVerdict` so `implementPrepInstruction`
  stays root-free; the typed path fills it from `record.Root`, and the
  legacy top-level `route.resolve_implement` path reads the record from the
  `sessionKey` already in scope before building the verdict, so both
  production call sites render the clause in a declaring project (the pure
  `deriveImplementTodos` helper has no session and stays unchanged); a test
  pins the legacy path rendering it;
  `proceed_resolver.go` `MigrationAnchor` field, `parseEnumFact` key,
  route strings, warning text, and their tests, plus the same plumbing
  on the proceed side: `resolveProceed` is pure, so the parsed
  declaration rides `proceedInput` from `handleEnterProceed`, and the
  `n/a` normalization of a lead-supplied `missing` or `conflict` applies
  only when no section parsed; `scope_announcement.go`
  and `TestScopeAnnouncementFiresOnWorkflowManual`; `wsdoc/legacy_marker.go`
  prefix string and `TestLegacyMarkerLinesIgnoreMechanismProseFile`;
  `wsdoc/doctor.go` checks table; the `### Binding Anchor` parser,
  which lives next to the review-policy parser in `wsreview` (or a sibling
  package if the dependency direction forbids it) and follows its
  fail-open contract and tests.
- Playbook touch points: `lead-discuss`, `lead-proceed` (fact table and
  the ask-first row), `lead-tune`, `lead-update-spec`, `lead-review`
  (built-in Landing Lens), `lead-bootstrap`; every rsrc edit is mirrored
  into `agents-plugin-wsflow/rsrc/` and the wsflow skill-bundle tests
  keep passing; wsflow `AGENTS.template.md` and `lead-revive/SKILL.md`.
- Spec addressing: `ai-docs/spec/workflow-skills.md`
  `{#260505-planning-workflow-skills}` (the "Migration topics such as ...
  load the native-subagent pivot anchor" sentence),
  `{#260513-proceed-ticket-freshness-gate}` (the "For migration-sensitive
  targets, `lead-proceed` reads the native-subagent pivot anchor ...
  `Migration Anchor`" sentence), `{#260519-proceed-implementation-dispatch-precheck}`
  (the "`lead-implement` also loads the native-subagent pivot anchor ...
  `[Must]` reference" sentences, and the "migration-anchor checks" clause in
  the playbook-ownership sentence at the tail of the same block),
  `{#260830-review-policy-config-surface}`
  (gains the `### Binding Anchor` section as a sibling declaration in the
  same home, per `## Spec Impact`); `ai-docs/spec/mcp-tools.md`
  `{#260625-session-state-tools}` ("conditional migration-anchor loading"
  becomes "binding-anchor loading when the project declares one", and the
  `route.resolve_proceed` route-vocabulary list entry `migration-anchor`
  becomes `binding-anchor`), and the scope-announcement sentence in
  `ai-docs/spec/mcp-tools.md` under `{#260810-scope-announcement-idea-inclusion}`
  (line 691, inside the `{#260626-workflow-manual-restoration-entry}`
  section) drops "pointing to `ai-docs/ref/worktree-ticket-scope.md`";
  the two bare citations at `mcp-tools.md` "(260605 pivot constraint)" and
  "(260617 obscurity, soft guard)" are deleted, since no `{#260605-...}`
  or `{#260617-...}` anchor exists to point at and the rules they annotate
  are self-contained. The `doctor` and legacy-marker spec
  sentences do not quote the removed text and are unchanged.
- Mental-model addressing: `ai-docs/mental-model/workflow-skills.md` line
  "Migration-sensitive workflow turns load `260605-...`" is reworded to
  the declared binding anchor and the `n/a` default; `mcp-runtime` gains
  no line unless the parser home is new.
- `ai-docs/manuals/skill-authoring.md` invariant checklist gains
  **Resolvable downstream** (resolves in a project holding only what
  bootstrap installs?) and "all six" becomes "all seven"; the Downstream
  Consistency Sweep is unchanged.
- `AGENTS.md` rule 4 states the criterion and the guard is its mechanical
  form; the test cites the rule and the rule is not re-stated elsewhere.
  Rule 4's first draft forbade the `ai-docs/tickets/`, `ai-docs/ref/`, and
  `ai-docs/manuals/` directory names outright; that was wrong for the
  reason Decision 4 gives and was corrected with this ticket.
- Out of scope: the `260605` anchor's content, the `migration_anchor`
  gate's behavior beyond the rename and `n/a` default, devenv's own
  `ai-docs/ref/` and `ai-docs/manuals/` documents, and the
  `kang-sw-devenv` literals in `wsstate/paths.go` (the cache directory
  name) and `wsagent/agent.go` (the Codex plugin-cache path segment).
  The cache-directory name does reach agents inside prompt paths and
  session records, but it is tool-created, so every downstream project
  has it, and the marketplace identity is the same for every project that
  installs from this marketplace (`.agents/plugins/marketplace.json` and
  `.claude-plugin/marketplace.json` declare it); neither depends on
  something a downstream project lacks. Renaming the cache directory would
  additionally orphan every existing installation's prompt paths,
  worktree registry, and session records. The audit recorded them; they
  fall outside the criterion.

## Spec Impact

`ai-docs/spec/workflow-skills.md`, appended under
`{#260830-review-policy-config-surface}` as a sibling declaration in the
tracked `AGENTS.md` home:

> `AGENTS.md` may declare one binding anchor under a `### Binding Anchor`
> section as `key: value` lines, parsed fail-open like `### Review Policy`:
> `anchor: <repo-relative path>` names a document whose settled decisions
> bind implementation on the declared topics, and `topics: <comma-separated
> phrases>` names them. When both parse, `lead-discuss` reads the anchor
> before stating a direction on a declared topic, `lead-proceed` reports
> `Binding Anchor` in the Routing Verdict from the `binding_anchor` gate
> fact, and the `route.resolve_implement` Prep instruction tells the lead
> to read the anchor when the target touches a declared topic. With no
> section the fact normalizes to `n/a`, no verdict line or Prep clause is
> rendered, and no shipped text names any anchor.
> {#260908-project-binding-anchor-declaration}

## Phases

### Phase 1: Declared binding anchor replaces the shipped constant

Depends on `260908-feat-survey-plan-is-route-not-contract` Phase 1 having
landed. Add the `### Binding Anchor` parser (fail-open, two keys) with tests for
missing file, missing section, one key only, and both keys. Rewrite the
Prep guardrail constant so the anchor clause is rendered from the parsed
declaration and omitted otherwise; update
`TestDeriveImplementTodoInstructionsPrepGuardrails` and the
`implementPrepGuardrails` test constant, and add the no-declaration case.
Rename the proceed fact, routes, warning, and verdict label per Decision
2, normalizing to `n/a` without a declaration, and update the resolver
tests. Rewrite `lead-discuss.md` and the `lead-proceed.md` fact table and
ask-first row to the generic term; mirror both into wsflow. Rewrite the
three workflow-skills sentences, the `{#260625}` clause, and the
mental-model line; add the `## Spec Impact` paragraph under `{#260830}`.
Declare devenv's anchor in `AGENTS.md` and point the `## Project Memory`
bullet at the section. Verification: `go test ./...` with `-count=1` in
`agents-plugin-tool/`; the wsflow skill-bundle tests; a render of
`route.resolve_implement` against a scratch root holding only `ai-docs/`
and a sectionless `AGENTS.md` whose Prep instruction contains no ticket
stem and none of the four devenv topics; the same render in devenv still
naming `260605` and its topics; `grep -rn 260605 agents-plugin
agents-plugin-wsflow agents-plugin-tool` returning nothing outside test
fixtures that exercise the parser, and `grep -rn 'migration.anchor'
ai-docs/spec agents-plugin agents-plugin-wsflow agents-plugin-tool`
returning nothing.

### Result (eceadf11) - 2026-09-09

Behavioral delta: shipped surfaces no longer carry devenv's migration
anchor. Projects declare a binding anchor in `AGENTS.md` under a new
`### Binding Anchor` section (two fail-open `key: value` lines, `anchor:`
and `topics:`), parsed by `wsreview.ReadAgentsBindingAnchor` exactly as
`### Review Policy` is parsed. The `route.resolve_implement` Prep guardrail
and the `route.resolve_proceed` gate render the anchor clause/route only
from that parsed declaration and omit it otherwise; a Go-only
`AnchorDeclared` flag forces the proceed `binding_anchor` fact to `n/a`
before the conflict warning and route selection whenever no section is
declared, even if the lead supplies `missing`/`conflict`. The
`migration_anchor` fact, routes, warning, condition, and verdict label were
renamed to `binding_anchor`/`Binding Anchor` across both resolvers,
`lead-discuss.md`, `lead-proceed.md`, their byte-identical wsflow mirrors,
the spec, and the mental model. devenv declares its own `260605` anchor
with its four topics in `AGENTS.md` and points the `## Project Memory`
bullet at the section, so its own behavior is unchanged while no shipped
string names that anchor.

Deviations from the phase plan: none material. Naming detail the ticket
left open was resolved in-scope: the new type is `wsreview.BindingAnchor`
(fit review noted it omits the precedent's `Agents` prefix — recorded
Minor, not changed) and the pre-rendered clause plumbing uses an
`AnchorDeclared` flag. The `## Spec Impact` paragraph was authored by the
delegated implementer from plan semantics (the installed implementer frame
still bans ticket reading), then reconciled by the lead against the
ticket's verbatim intent during the doc pre-pass (added the "settled
decisions bind implementation" framing); it carries the ticket's exact
anchor slug `{#260908-project-binding-anchor-declaration}`.

Review dispositions: partitioned (correctness opus/large, fit sonnet/medium,
test sonnet/medium). Correctness clean +1 Minor (legacy-path comment
imprecision; behavior correct). Fit clean +1 Minor (type name lacks
`Agents` prefix). Test non-clean review #1: one Important (the AGENTS.md-read
wiring at the three production call sites was never exercised end-to-end,
contradicting the phase constraint that a test pins the legacy path) plus
one Minor (no proceed `n/a`-override case). Relay #1 fixed both
[fixed]: three `t.TempDir()` integration tests now drive the real handlers
through `NewServer`/`callToolWithKey` and assert the rendered clause/route
at `session_state.go:1101`/`:1145`/`:1204`, and a `TestResolveProceedRoutes`
case proves force-to-`n/a` overrides a supplied non-`n/a` value. No
Critical, so no re-review; Minors recorded only. The reviewer-noted
`readState` empty-root fail-open edge is a pre-existing pattern, not
introduced here — left out of scope.

Verification: `go test ./... -count=1` in `agents-plugin-tool` green across
all 14 packages; wsflow python bundle 10/10; `diff -rq agents-plugin/rsrc
agents-plugin-wsflow/rsrc` byte-identical (manifest included); `grep -rn
260605 agents-plugin agents-plugin-wsflow agents-plugin-tool` hits only
parser test fixtures and the pre-existing wsflow epic comment; `grep -rn
'migration.anchor' ai-docs/spec agents-plugin agents-plugin-wsflow
agents-plugin-tool` clean. Phases 2 (point leaks) and 3 (guard) remain.

### Phase 2: Point leaks

Apply Decision 3 to the twelve point-leak sites and mirror every rsrc
change into `agents-plugin-wsflow/rsrc/`; add devenv's Landing Lens clause
to `ai-docs/_review.local.md` (gitignored, so the change is local and
noted in the Result). Update `TestScopeAnnouncementFiresOnWorkflowManual`
and `TestLegacyMarkerLinesIgnoreMechanismProseFile` to assert the generic
text, and drop the "pointing to `ai-docs/ref/worktree-ticket-scope.md`"
phrase from the scope-announcement sentence in `ai-docs/spec/mcp-tools.md`
under `{#260810-scope-announcement-idea-inclusion}`. Verification:
`go test ./...` with `-count=1`; both plugin test suites; `ws-mcp doctor
--root <scratch>` against a directory holding only `ai-docs/` and
`AGENTS.md` reporting OK; `diff -r agents-plugin/rsrc
agents-plugin-wsflow/rsrc` empty; `grep -rn worktree-ticket-scope
ai-docs/spec agents-plugin agents-plugin-wsflow` returns nothing.

### Phase 3: Guard

Add `agents-plugin/tests/test_shipped_surfaces_downstream_neutral.py` per
Decision 4, citing `AGENTS.md` rule 4 in its docstring, with a unit case
proving the Go string-literal extractor ignores comments. Add the
**Resolvable downstream** checklist item to `skill-authoring.md`.
Verification: the test passes on the post-Phase-2 tree; reinserting the
pre-Phase-1 guardrail sentence into `session_state.go` fails it; a
`// 260605` comment, the `ticket-conventions.md` example stem
`260115-feat-foo-bar`, and a line naming `ai-docs/manuals/` as a
directory each pass; the plugin suites pass.
