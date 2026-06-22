---
title: ws ticket status-transition MCP tools (close/drop/promote/demote)
related:
  260622-feat-sage-review-ticket-gate: sage-review pre-condition hook and downward demotion tip are required additions to Phase 1 of this ticket
plans:
  phase-1: 2026-06/22-260620-ticket-status-tools.brief
---

# ws ticket status-transition MCP tools (close/drop/promote/demote)

## Background

Ticket status transitions — closing to `.done/`/`.dropped/` and promoting
`idea/`→`todo/`→`ready/` — are frequent, rule-shaped edits: a frontmatter change
(`completed:`/`dropped:` date), a directory move (`git mv`), and for closes an
appended `## Resolution (<date>)` body section. Today the lead does this by hand
with `Edit` + `git mv`, and the two steps are not atomic: `git mv` stages only
the HEAD version of the file under the new path and silently drops an
immediately-preceding unstaged `Edit`, so the frontmatter/Resolution edit lands
in the working tree but not in the commit. This footgun hit twice in a single
bug-backlog sweep (`260620`; both commits had to be amended after the fact).

The `ws` MCP already mutates files and git (`git.commit`), while `tickets.*` is
otherwise read-only (`tickets.list/find/status`). A small write surface that
performs the frontmatter edit + move (+ optional Resolution append) as one
atomic, convention-checked operation removes the staging footgun by construction
and turns ticket-convention rules into enforced guards instead of lead memory.

## Decisions

Confirmed with the user (2026-06-20):

- **Scope = close + drop + promote + demote.** Two tools:
  - `tickets.close(stem, status, resolution?)` — `status ∈ {done, dropped}`.
    Writes the dated frontmatter field (`completed:` for done, `dropped:` for
    dropped, both = today), moves the file to `.done/`/`.dropped/`, and when
    `resolution` is supplied appends a `## Resolution (<today>)` body section.
  - `tickets.move(stem, to)` — moves along the `idea ↔ todo ↔ ready` axis in
    both directions. Upward: promotion. Downward: demotion (e.g. `ready → idea`
    to reopen design). On downward move from `ready/`, the tool returns a tip:
    "This ticket had spec entries; clear `spec:`, `spec-remove:`, and review
    `## Spec Impact` before re-promoting." Spec cleanup is not automatic.
  - Out of scope: `tickets.set` (general frontmatter such as `parent:` tagging),
    `tickets.create`, and any full-CRUD surface. Those stay manual `Edit`.

- **sage-review pre-condition on upward moves.** When the `sage_review` config
  is `auto | ask`, `tickets.move` for upward transitions checks the ticket's
  `sage-review` frontmatter field: fails the move if value is `pending | blocked`.
  Value `completed | skipped` (or field absent, for tickets predating the feature)
  passes. When `sage_review: off`, the field is ignored entirely.

- **git boundary = mutation only.** The tools mutate files (frontmatter + move +
  optional Resolution append) and stage that change set; they do **not** commit.
  The lead commits with `ws/git.commit`, because the commit `## AI Context` /
  `## Ticket Updates` message is a lead judgment the tool must not author. The
  footgun is fully removed by atomic mutation alone.

- **Rules become guards.** The tools enforce ticket conventions rather than
  trusting the caller: reject an unknown stem or invalid target status; preserve
  the immutable `YYMMDD` date prefix; reject re-closing an already-closed ticket;
  apply the status-specific dated field (done→`completed`, dropped→`dropped`).

- **Usage guidance lives in three layers, not per-playbook.** Transition is
  directed from ~8 scattered playbooks (`lead-write-ticket`, `lead-proceed`,
  `lead-discuss`, `lead-forge-spec`, `lead-salvage`, ...), so usage is not copied
  into each. The `ticket-conventions` convention doc — the single source every
  transition-directing playbook already reads via `convention.read` — owns the
  transition *rule* and names the tools as canonical with `git mv` as fallback;
  `lead-workflow-manual` registers them in the primitive *catalog*; individual
  playbooks keep only "when to transition" and reference the convention rather
  than repeating `git mv`/frontmatter steps. One convention edit then propagates
  to every transition site.

## Spec Impact

- **Target spec**: `mcp-tools` (`ai-docs/spec/mcp-tools.md`) — ticket tools section
- **Expected caller-visible change**: adds `tickets.close(stem, status, resolution?)` and `tickets.move(stem, to)` to the MCP tool surface in both the full `ws` and `wsflow` runtime contracts; both tools write frontmatter, stage atomically, and enforce ticket-convention guards at the MCP layer.
- **Contract-first spec**: no — tool schemas are finalized during Phase 1 implementation; the `mcp-tools.md` update is a Phase 1 deliverable, not a pre-implementation contract.

## Phases

### Phase 1: tickets.close + tickets.move

Implement both tools in the native MCP tooling tree (`agents-plugin-tool`,
`internal/mcp` over the existing `tickets.*` handlers and the `wsdoc` ticket
layer), sharing one frontmatter read-modify-write + atomic-move helper so the
close, promote, and demote paths cannot diverge on staging behavior.

Surface both as MCP tools, and as CLI commands if the namespace ships CLI
parity. Update `ai-docs/spec/mcp-tools.md` and both `runtime.json` contracts
(full + wsflow). The wsflow agentless contract cross-check test added by
`260619-bug-wsflow-runtime-contract-uncrosschecked` will catch a missed wsflow
manifest update; `TestRuntimeCapabilitiesCommandReportsLauncherContractSurface`
covers the full surface.

Settled design points (resolved in discussion):
- **`tickets.move(stem, to=ready)` is permitted.** The spec-address gate lives in
  `lead-write-ticket` as a playbook-layer concern, not an MCP-layer hard lock.
  The tool does not enforce `spec:`/`spec-remove:`/`## Spec Impact` presence;
  that validation stays in the playbook.
- **Both tools land in the wsflow surface as well as the full surface.** wsflow
  diverges from the full ws surface only for mercenary and exec (external-process)
  features. Ticket lifecycle tools are not in that category and belong in both
  contracts. This boundary rule should be documented in the wsflow surface mental
  model (currently implicit; a doc update is a follow-on from this ticket).
- **`sage_review` config absent → no-op.** If the `sage_review` config key is not
  yet registered (260622 Phase 3 registers it), the pre-condition check treats it
  as `off` and allows the move. Same logic as `sage-review` field absent passing
  the gate.

Verification:
- An atomic close/move/demote leaves frontmatter + directory move + (on close)
  the Resolution section all in one staged change set, with no unstaged remainder.
  Implementation must stage via: write file → `git add <old-path>` → `git mv
  <old-path> <new-path>` to preserve the edit in the index before rename.
- Downward move from `ready/` returns the spec-cleanup tip in the tool response.
- Upward move with `sage_review` config on: fails on `sage-review: pending |
  blocked`; passes on `completed | skipped | absent`. Config key absent → passes
  (no-op, treated as `off`).
- Convention guards reject: unknown stem, invalid target status, re-close of an
  already-closed ticket, and any date-prefix mutation.
- `go test ./...` green, including both runtime-contract cross-checks: full
  surface (`TestRuntimeCapabilitiesCommandReportsLauncherContractSurface`) and
  wsflow surface (`TestRuntimeCapabilitiesCommandReportsWsflowContractSurface`).
- `ai-docs/spec/mcp-tools.md` updated with new tool entries under the ticket
  tools section, using `{#YYMMDD-slug}` anchor convention.

### Result (735acfe4) - 2026-06-22

Both tools implemented in full on branch `implement/260620-ticket-status-tools`.

**Delivered:**
- `agents-plugin-tool/internal/wsdoc/tickets_mutate.go` (new) — `TicketsClose`,
  `TicketsMove`, `findTicketPath`, `writeFrontmatterField`, `atomicGitMove`,
  `checkSageReview`; local `GitRunner` interface (no `wsgit` import).
- `agents-plugin-tool/internal/wsdoc/tickets_mutate_test.go` (new) — all 14
  brief test cases pass.
- `agents-plugin-tool/internal/mcp/server.go` — dispatch cases, JSON schemas,
  both tools added to `rootAwareToolSchemaRequiresSessionKey`, inline
  `wsconfig.Resolver` for `sage_review` read on move.
- `agents-plugin-tool/internal/mcp/format.go` — `FormatTicketMutate` helper
  exported for CLI (minor scope addition: brief had inline formatting; extracted
  to keep CLI and MCP consistent).
- `agents-plugin-tool/cmd/ws-mcp/main.go` — `tickets close|move` CLI mirrors +
  `runtimeCapabilityCommandNames` entries.
- `agents-plugin/runtime.json` + `agents-plugin-wsflow/runtime.json` — both
  tools in `tools` AND `commands` sections (4 insertion points total).
- `ai-docs/spec/mcp-tools.md` — entries with anchors
  `{#260620-ticket-close-tool}` / `{#260620-ticket-move-tool}`.

**Bug found during CLI spot-check (not caught by unit tests):** `git mv --force`
does not create the destination directory (e.g., `.done/` on a fresh repo). The
mock runner had been masking this via implicit `os.MkdirAll`. Fix: `atomicGitMove`
now calls `os.MkdirAll` on the destination parent before invoking `git mv`; mock
hardened to stop silently mkdir-ing. All 14 tests re-verified after fix.

**Verification results:** `go test ./...` green (all packages). 14/14 new tests
PASS. `TestRuntimeCapabilitiesCommandReportsLauncherContractSurface`,
`...WsflowContractSurface`, and `...NoAgentSurface` all PASS.

### Phase 2: transition-guidance rewiring

Point the scattered transition directives at the new tools through their
canonical home rather than per-playbook copies. Skill/convention edits here run
under `lead-skill-authoring` (read its SKILL.md and apply the invariant checklist
to every changed Invariants/Constraints/Doctrine line).

- `ticket-conventions` (convention doc): make `tickets.close`/`tickets.move` the
  canonical transition mechanism, with `git mv` named as the fallback; the
  dated-field and status-flow rules describe what the tools enforce.
- `lead-workflow-manual`: register `tickets.close`/`tickets.move` in the
  primitive catalog (mutation tools; call form).
- Transition-directing playbooks: run `grep -rn "git mv" agents-plugin/rsrc/` to
  find every playbook with a ticket-transition directive. Replace direct
  `git mv`/frontmatter steps with a reference to the convention rule; keep only
  when-to-transition semantics. (`lead-write-ticket` Move section first; then
  `lead-proceed`, `lead-discuss`, `lead-forge-spec`, `lead-salvage`, and any
  others the grep surfaces.)

Depends on Phase 1 (the tools must exist before guidance points at them).

Verification:
- The convention doc names the tools as canonical with `git mv` as fallback.
- No transition-directing playbook repeats `git mv` usage the convention now owns.
- rsrc manifest + wsflow mirror regenerated; playbook render/freshness guards green.
