---
title: ws ticket status-transition MCP tools (close/drop/promote)
---

# ws ticket status-transition MCP tools (close/drop/promote)

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

- **Scope = close + drop + promote.** Two tools:
  - `tickets.close(stem, status, resolution?)` — `status ∈ {done, dropped}`.
    Writes the dated frontmatter field (`completed:` for done, `dropped:` for
    dropped, both = today), moves the file to `.done/`/`.dropped/`, and when
    `resolution` is supplied appends a `## Resolution (<today>)` body section.
  - `tickets.move(stem, to)` — promotes along `idea → todo → ready`.
  - Out of scope: `tickets.set` (general frontmatter such as `parent:` tagging),
    `tickets.create`, and any full-CRUD surface. Those stay manual `Edit`.

- **git boundary = mutation only.** The tools mutate files (frontmatter + move +
  optional Resolution append) and stage that change set; they do **not** commit.
  The lead commits with `ws/git.commit`, because the commit `## AI Context` /
  `## Ticket Updates` message is a lead judgment the tool must not author. The
  footgun is fully removed by atomic mutation alone.

- **Rules become guards.** The tools enforce ticket conventions rather than
  trusting the caller: reject an unknown stem or invalid target status; preserve
  the immutable `YYMMDD` date prefix; reject re-closing an already-closed ticket;
  apply the status-specific dated field (done→`completed`, dropped→`dropped`).

## Phases

### Phase 1: tickets.close + tickets.move

Implement both tools in the native MCP tooling tree (`agents-plugin-tool`,
`internal/mcp` over the existing `tickets.*` handlers and the `wsdoc` ticket
layer), sharing one frontmatter read-modify-write + atomic-move helper so the
close and promote paths cannot diverge on staging behavior.

Surface both as MCP tools, and as CLI commands if the namespace ships CLI
parity. Update `ai-docs/spec/mcp-tools.md` and both `runtime.json` contracts
(full + wsflow). The wsflow agentless contract cross-check test added by
`260619-bug-wsflow-runtime-contract-uncrosschecked` will catch a missed wsflow
manifest update; `TestRuntimeCapabilitiesCommandReportsLauncherContractSurface`
covers the full surface.

Open design points to settle at implementation start (deliberately not decided
yet):
- Whether `tickets.move(stem, to=ready)` is permitted at all, or whether ready
  promotion stays exclusively in `lead-write-ticket` so the spec-address gate is
  never bypassed — and if permitted, whether the tool refuses a ready move that
  lacks `spec:`/`spec-remove:`/`## Spec Impact`.
- Whether close/move are exposed in the agentless (wsflow) surface or full-only.

Verification:
- An atomic close/move leaves frontmatter + directory move + (on close) the
  Resolution section all in one staged change set, with no unstaged remainder —
  the staging footgun this fixes.
- Convention guards reject: unknown stem, invalid target status, re-close of an
  already-closed ticket, and any date-prefix mutation.
- `go test ./...` green, including the runtime-contract cross-checks.
