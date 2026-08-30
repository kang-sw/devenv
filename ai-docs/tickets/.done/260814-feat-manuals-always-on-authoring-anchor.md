---
title: Make # Manuals an always-on authoring anchor
related-mental-model:
  - mcp-runtime
sage-review-completeness: completed
sage-review-design: completed
completed: 2026-08-14
---

# Make # Manuals an always-on authoring anchor

## Background

The `ai-docs/manuals/` tier is asymmetrically under-exposed as an *authoring*
convention. An audit (this session) found:

- The consumed schema is deliberately a single optional field, `summary:`
  (`agents-plugin-tool/internal/wsdoc/manuals.go:10-16,49`).
- There is **no `manuals-conventions.md`** returned by `ws/convention.read`
  (`agents-plugin-tool/internal/wsdoc/conventions.go:14-18`), unlike
  spec/ticket/mental-model tiers. An agent reaching for the canonical
  authoring-discovery path finds nothing for manuals.
- Authoring guidance exists only inside the `lead-bootstrap` skill surface and
  the ambient nudge — not on a generally-discoverable channel.

Meanwhile the ambient `# Manuals` block
(`agents-plugin-tool/internal/mcp/manuals_announcement.go`) is injected only
into **lead** `workflow_manual` output (subagents cannot bootstrap
workflow_manual — `workflow_manual.go:31-33`), and it early-returns `""` when
zero manuals exist, so the block vanishes exactly when a project has not yet
started writing manuals — the moment authoring guidance would help most.

Goal: turn `# Manuals` into an **always-present authoring anchor** for the lead
session that (a) teaches where shared project procedures live and how to
summarize them, and (b) teaches the local/tracked split so credentials, IPs,
and other machine-local details are written to a gitignored `*.local.md`
sibling rather than committed into a tracked manual. This is deliberately a
different treatment from `# Notes` (which is presence-gated): `# Manuals` should
be an ever-present convention anchor, not a memory dump.

## Decisions

Resolved:

- **Always render `# Manuals`.** Drop the `len(manuals)==0 -> ""` early return
  in `computeManuals`; the block renders header + a fixed authoring-guidance
  paragraph unconditionally, then either the manual list or a `- (none yet)`
  placeholder. Cost is lead-bootstrap-only (not per-subagent), so the standing
  overhead is small and lands on the right audience (the lead curates manuals).
- **Channel is the ambient block, not `convention.read`.** Per user direction,
  the guidance lives in the always-on `# Manuals` block. A
  `manuals-conventions.md` `convention.read` doc is explicitly out of scope for
  this ticket (the consumed schema is one field; the ambient anchor covers the
  authoring need for the lead audience).

Resolved (user's call, this session):

- **`.local.md` are listed, with summary rendering skipped entirely.** Because
  workflow_manual is lead-local, showing a local manual to the session that owns
  it is useful, not a leak — so local manuals stay in the ambient `# Manuals`
  list. But they render as a bare `- <path>` line: no summary, **and no
  "(no summary: …)" nag** (the `.local.md` suffix already implies local; a
  nag to add frontmatter to a gitignored creds/IP file is wrong). No `(local)`
  marker — the suffix carries the meaning. Tracked manuals keep the existing
  `- <path> — <summary>` (with the no-summary nag) behavior unchanged.
  `manuals.list`/`manuals.find` behavior is unchanged (this rule is
  ambient-block-only).

Confirmed guidance text (English, ambient block):

```
# Manuals
Shared project procedures (build, deploy, env setup, …) live here: one markdown
file per procedure under `ai-docs/manuals/`, each opening with a one-line
`summary:` frontmatter. Keep machine-local details (credentials, IPs, hostnames)
out of tracked manuals — write them to a sibling `*.local.md` (gitignored).
- <path> — <summary>
```

## Prior Art

- `computeNotes` / `wsnote.Compute` (`agents-plugin-tool/internal/wsnote/inject.go`)
  — the presence-gated sibling block; `# Manuals` intentionally diverges by
  being always-on.
- `.gitignore:16` already carries `ai-docs/**/*.local.md`, so the `*.local.md`
  convention has gitignore backing (existing `ai-docs/_index.local.md`,
  `_continue.local.md`).

## Spec Impact

The manuals contract spans two spec files; both phases must update both, or the
committed spec set will contradict the landed code. Exact anchors:

- `ai-docs/spec/mcp-tools.md`
  - `{#260807-manuals-ambient-injection}` (~L717) — the `# Manuals`
    ambient-block contract. **Phase 1**: state always-on for lead sessions,
    fixed authoring-guidance paragraph, `- (none yet)` placeholder when empty,
    and the `.local.md` bare-path (no summary, no nag) rule.
  - `{#260807-manuals-discovery-tools}` (~L1561) — the `manuals.list`/
    `manuals.find` tool contract. **Phase 2**: remove this section; record that
    the always-on ambient `# Manuals` block is the manuals discovery surface.
- `ai-docs/spec/documentation-system.md`
  - `{#260807-manuals-document-system}` (~L204-222). **Phase 1**: amend the
    "no-summary manual is still announced with an explicit no-summary marker"
    universal claim (~L216) to carve out the `.local.md` no-summary/no-nag
    exception. **Phase 2**: remove/redirect the "`ws/manuals.list` and
    `ws/manuals.find` expose manual path and summary" statement (~L220) to name
    the ambient block as the discovery surface.

The tier keeps its single `summary:` schema; only the two query tools are
retired.

## Phases

### Phase 1: Always-on # Manuals authoring anchor

Goals:

- Remove the empty-list early return in `computeManuals`; render header +
  fixed guidance paragraph + list-or-placeholder.
- Apply the resolved `.local.md` rule: list `*.local.md` as bare `- <path>`
  lines in the ambient block (no summary, no nag, no marker); tracked manuals
  keep `- <path> — <summary>` with the existing no-summary nag.
- Update tests: `manuals_announcement`-level rendering (empty, non-empty,
  no-summary, `.local.md` handling) and the `note_workflow_manual` /
  workflow_manual assembly tests that assert the `# Manuals` block.
- Update the spec behavior contract per `## Spec Impact`.

Constraints:

- Do not add a `manuals-conventions.md` `convention.read` doc in this ticket.
- Keep the change scoped to `computeManuals` (+ `ManualsList` only if the
  chosen `.local.md` rule requires it); do not alter `# Notes` behavior.
- Guidance text is AI-authored English.

### Result (525064f) - 2026-08-14

Implemented directly (user chose direct implementation, not delegated). Landed
in `525064f4`.

- `computeManuals` (`agents-plugin-tool/internal/mcp/manuals_announcement.go`)
  now renders the always-on anchor: `# Manuals` header + the new
  `manualsAuthoringGuidance` const paragraph + list-or-`(none yet)`. It returns
  `""` only on a genuine `ReadDir` error; the empty tier (`ManualsList` →
  `(nil, nil)`) renders `- (none yet)`. `ManualsList` was left untouched — the
  `.local.md` rule is a render-branch-only suffix check (`strings.HasSuffix`),
  emitting a bare `- <path>` line with no summary and no nag.
- Tests: inverted `TestComputeManualsReturnsEmptyWhenNoManualsExist` →
  `TestComputeManualsRendersAnchorWhenNoManualsExist` and
  `TestWorkflowManualManualsBlockAbsentWhenNoManualsExist` →
  `...IsAlwaysOnWhenNoManualsExist`; added
  `TestComputeManualsSkipsSummaryForLocalManual`. All manuals tests green;
  `go build`/`go vet`/`gofmt` clean; spec index ok.
- Spec: updated `mcp-tools.md {#260807-manuals-ambient-injection}` and
  `documentation-system.md {#260807-manuals-document-system}` per Spec Impact.

Deviations: none of substance. Note captured for Phase 2 and closeout — the two
pre-existing mcp failures (`TestWorkflowManualCarriesNotesBlockOnFreshAndContinuePositionedAfterSessionState`,
`TestWorkflowManualNotesBlockAbsentWhenNoNotesExist`) are unrelated: both stem
from a naive `strings.Index(body, "# Notes")` that matches the prose
`### Notes / durable memory` section (added in `fbec365f`), not from this change.

### Phase 2: Retire manuals.list / manuals.find

Rationale: the manuals tier carries a deliberately minimal single-field
(`summary:`) schema with no applicability predicate, unlike specs/mental-models
whose rich frontmatter justifies query tooling. `manuals.list`/`manuals.find`
were added in `d2c82584` purely "for discovery parity"; that parity is
superficial. Once Phase 1 makes the ambient `# Manuals` block always-on, the
block fully subsumes `manuals.list`'s discovery role for the lead (the only
audience that could bootstrap it), so the MCP pair becomes dead surface. This
is API-surface removal ("Always ask" tier) and is deliberately sequenced after
Phase 1 so the replacement lands before the removal.

Goals:

- Remove the `manuals.list` and `manuals.find` MCP tools from
  `server.go` (schema/registration/dispatch) and the CLI mirror
  (`ws-mcp manuals list|find`).
- Remove now-dead Go surface: `ManualsFind` and `formatManuals` (only the
  removed tool/CLI used them). **Keep `ManualsList`** — `computeManuals` (the
  ambient block) still depends on it.
- Reverse the manifest/contract wiring `d2c82584` added: drop the manuals
  entries from `toolSchemaRequiresSessionKey`, the LeadToolNames session-key
  list, `runtimeCapabilityCommandNames()`, and both `runtime.json`
  tools+commands sections (ws and wsflow).
- Regenerate the wsflow `rsrc/` mirror if the removal touches any mirrored
  surface (regen only; never hand-edit the byte-identical mirror).
- Update the exact-match runtime-capability tests
  (`TestRuntimeCapabilitiesCommandReportsLauncherContractSurface` and its
  wsflow counterpart) and any manuals-tool tests.
- Update the spec per `## Spec Impact` Phase 2 scope: remove
  `mcp-tools.md {#260807-manuals-discovery-tools}` and redirect the
  `documentation-system.md {#260807-manuals-document-system}` discovery-surface
  statement to the ambient `# Manuals` block.

Constraints:

- Do not remove `manuals.list`/`manuals.find` before Phase 1 lands (the ambient
  always-on block is the replacement that justifies removal).
- Keep `ManualsList` and the manuals doc tier itself; only the two MCP
  tools/CLI mirror are retired, not the tier.
- Version bump is the lead's merge-time step, not part of either phase.

### Result (68f691c6) - 2026-08-14

Delegated implementation on `impl/develop/manuals-retire-list-find` (survey plan
`97ee12da`). The `manuals.list`/`manuals.find` MCP tools (schema + dispatch),
their `ws-mcp manuals list|find` CLI mirror, and the now-dead Go surface
(`ManualsFind`, `formatManuals`) are removed; `ManualsList` and the manuals doc
tier are kept — `computeManuals` (the Phase 1 ambient block) still consumes
`ManualsList`. Reversed the `d2c82584` wiring: the two tokens in
`toolSchemaRequiresSessionKey`, the two `runtimeCapabilityCommandNames()`
entries, and the tools+commands entries in both `agents-plugin/runtime.json` and
`agents-plugin-wsflow/runtime.json`. Spec: deleted
`mcp-tools.md {#260807-manuals-discovery-tools}` and redirected the
`documentation-system.md {#260807-manuals-document-system}` discovery-surface
paragraph to name the ambient `# Manuals` block (plus trimmed an adjacent stale
"or discovery tools" phrase).

Deviations from the ticket text, all found by the survey and all narrowing the
work rather than expanding it:

- `LeadToolNames()` needed no direct edit — it derives names dynamically from
  `tools()`, so dropping the two schema entries removes them from
  `runtime.capabilities` automatically. The ticket's "drop the LeadToolNames
  session-key list" described the effect, not a separate edit site.
- The two runtime-capability contract tests
  (`TestRuntimeCapabilitiesCommandReportsLauncherContractSurface` and
  `...WsflowContractSurface`, `cmd/ws-mcp/main_test.go`) read their expected sets
  from the two `runtime.json` files, so they self-corrected — no test-code edit.
- No wsflow `rsrc/` regen was required: a full grep of `agents-plugin*/rsrc` and
  `agents-plugin*/skills` found zero references to either tool name.
- Extra dead surface the ticket did not name but the survey traced: the exported
  `FormatManuals` wrapper (`internal/mcp/format.go`, only CLI callers), two
  `ManualsFind` unit tests (`internal/wsdoc/manuals_test.go`), a stale
  `ManualsList` doc comment naming `formatManuals`, and the two manuals tokens in
  `internal/mcp/server_test.go`'s tool-name assertion lists (build follow-through).

Verification: `go build`/`go vet`/`gofmt -l` (touched files) clean;
`internal/mcp`, `internal/wsdoc`, `cmd/ws-mcp` tests pass except two known
pre-existing failures unrelated to this phase
(`TestWorkflowManualCarriesNotesBlockOnFreshAndContinuePositionedAfterSessionState`,
`TestWorkflowManualNotesBlockAbsentWhenNoNotesExist`), confirmed still failing at
pre-Phase-1 commit `6e12c9a2` — their cause is a naive
`strings.Index(body, "# Notes")` colliding with the prose heading
`### Notes / durable memory` (`fbec365f`), not the manuals surface. wsflow tests
10/10; `spec_index.verify` ok. Correctness and test reviewers both returned
clean (no findings).

Follow-up (deferred, not blocking): capture an `idea/` ticket for the two
pre-existing `# Notes` substring-collision test failures so the naive
`strings.Index` matcher gets a real fix.
