---
title: "Shipped ws surfaces carry devenv-only tickets, paths, and migration vocabulary"
related:
  260605-research-ws-native-subagent-pivot: the devenv-only anchor the shipped Prep guardrail, lead-discuss, and spec name
  260611-chore-lead-discussion-gap-discipline: origin; its Phase 2 asked shipped playbooks to "honor the AGENTS.md migration-anchor read rule"
  260908-feat-implement-skip-survey-for-localized-ticket-target: its stub-plan rule is phrased against the Prep reads rather than the anchor because of this leak
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
| `agents-plugin-tool/internal/wsdoc/legacy_marker.go:408`, pinned `legacy_marker_test.go:234` | advisory "being retired by 260726-refactor-retire-spec-planned-marker-mechanism" | misleading |
| `agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md:134-142` | Equivalence note citing commit `599fb453` and ticket `260825` | misleading |
| `agents-plugin-tool/internal/wsstate/paths.go:18`, `internal/wsagent/agent.go:284` | `kang-sw-devenv` marketplace identity hardcoded | cosmetic |
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

## Phases

### Phase 1: Project-declared binding anchor replaces the shipped constant

Remove the devenv anchor from the migration-anchor family. The Prep
guardrail, `lead-discuss`, and the three spec sentences tell the lead to
read the project's declared binding anchors when the target touches their
declared topics; devenv declares `260605` with its four topics as its own
project memory. Decide where the declaration lives: the `AGENTS.md`
"Migration anchor" paragraph already states it for humans and leads, and a
`config.list` key would make it runtime-readable by the Go instruction;
pick one and record why the other lost. Rename the `migration_anchor`
proceed fact and its `anchor-discussion.migration-anchor-*` routes to the
generic name, or record why the name stays. Rewrite
`ai-docs/mental-model/workflow-skills.md:46` so it names the generic hook
and states devenv's declaration as this repository's own convention.
Update the `session_state_test.go` pins and the wsflow mirror.
Verification: a downstream-shaped fixture with no declared anchor renders
a Prep instruction that names no ticket stem and no devenv topic; devenv's
render still names `260605` with its topics through the declaration.

### Phase 2: Point leaks

Fix the twelve point-leak sites in the table: delete the `lead-tune`
ticket pointer, replace the `lead-update-spec` raw path with
`playbook.read(name: "lead-write-spec")`, drop the `agents-plugin/` check
from `doctor` and `smoke` (or gate it on the source repository), give
`lead-review` a generic built-in Landing Lens and move the
`skill-authoring`/`wsflow-mirroring` clause into devenv's own
`ai-docs/_review.local.md`, delete the `claude-plugin/` sentence from
`lead-bootstrap`, drop the `ai-docs/ref/` path from the scope banner and
the ticket stem from the legacy-marker advisory (update both pins), delete
the wsflow template Equivalence note, derive or document the
`kang-sw-devenv` identity, reword the convention analogy to `AGENTS.md`,
replace the bare ticket-number citations with spec anchors or drop them,
and fix the wsflow `lead-revive` wording. Mirror every rsrc change into
`agents-plugin-wsflow/rsrc/`. Verification: `go test ./...` in
`agents-plugin-tool/`, the plugin test suites, and `ws-mcp doctor` run
against a scratch directory holding only `ai-docs/` and `AGENTS.md`
reporting OK.

### Phase 3: Guard

Make the `AGENTS.md` downstream-first rule mechanical. Add a test in
`agents-plugin/tests/` that scans `agents-plugin/rsrc/**`,
`agents-plugin/skills/**`, the wsflow equivalents, the embedded
conventions, and every non-test `.go` string literal under
`agents-plugin-tool/internal/` for `26[0-9]{4}-[a-z]` ticket stems,
`ai-docs/tickets/` paths that name a ticket, and the devenv-only path and
vocabulary list the `AGENTS.md` rule enumerates, failing on any hit with no
allowlist; the spec-conventions anchor form `{#26xxxx-...}` inside
`ai-docs/spec/` is out of scope because the spec is not shipped. Add a
"Resolvable downstream" item to the `ai-docs/manuals/skill-authoring.md`
invariant checklist: a shipped sentence must resolve in a project that has
only what bootstrap installs. Verification: the test passes on the tree
after Phases 1 and 2 and fails when the `session_state.go` guardrail
sentence from before Phase 1 is reinserted.
