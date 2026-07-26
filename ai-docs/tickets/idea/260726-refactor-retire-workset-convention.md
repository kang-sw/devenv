---
title: "Retire the workset ticket convention"
related:
  260726-feat-verify-ticket-graph-advisories: the epic/workset boundary analysis that surfaced this; its Out of Scope carries the supporting measurements
  260624-epic-pre-release-cleanup: currently categorized `epic` but shaped as a workset (`## Items` + `### 1.`-`### 7.`, zero `parent:` children); its category must be resolved by this retirement, not before it
  260713-workset-workflow-dogfood-bugs: the only open workset ticket; decides whether retirement needs a migration step or just a category rename
---

# Retire the workset ticket convention

## Background

`workset` was introduced for one job: **group tickets that must be handled in one
pass even when their categories and parents differ.** That job is now done by a
runtime mechanism instead - `lead-goal-step` and the goal loop pile the selected
work into `ready/` and drain it, which is the same "explicit batch, mixed
parents" outcome without a document to hand-maintain.

The user confirmed the convention is retirable on this basis (2026-07-26).

This does **not** transfer workset's role to `epic`. Workset's role went to a
runtime loop, not to another document category. An epic that absorbed
"project-wide common agenda" would re-create workset immediately after deleting
it, so `epic`'s identifier stays **single-outcome decomposition**.

## Decision

Remove `workset` as a ticket category. The grouping need is served by the goal
loop over `ready/`; the hierarchy need is served by `epic` + `parent:`; the
non-hierarchical annotation need is served by frontmatter `related:`.

## Measured Footprint

Measured 2026-07-26 (excludes `.plans/`, `CHANGELOG.md`, and worktrees).

**Usage is nearly zero - 5 workset tickets ever:**

- `.done/`: `260615-workset-pre-api-ask-dogfood-stabilization`,
  `260629-workset-policy-sweep-implementation`,
  `260629-workset-pre-merge-idea-backlog-sweep`,
  `260702-workset-wsflow-dogfood-followups`
- open: `260713-workset-workflow-dogfood-bugs` (todo/)

(`260524-epic-ws-dashboard-editor-workroot-workset` is unrelated - "workset"
there is a dashboard workroot concept, not the ticket category.)

**But the implementation surface is wide:**

| Surface | Files |
| --- | --- |
| Go source | `internal/mcp/proceed_resolver.go`, `internal/mcp/server.go`, `internal/wsdoc/tickets_{checklist,mutate,sage,template}.go` (6) |
| Go tests | `internal/mcp/{session_state,tickets_checklist,tickets_template}_test.go`, `internal/wsdoc/{ticket_create,tickets_mutate,tickets_sage,tickets_verify}_test.go` (7) |
| Conventions (`go:embed`) | `ticket-conventions.md` (`## Workset Tickets` 3 lines + 2 `## Status Flow` lines), `spec-conventions.md` |
| Playbooks (x2 mirrors) | `lead-{forge-spec,proceed,workflow-manual,write-spec,write-ticket}` |
| Bootstrap template | `agents-plugin/skills/lead-bootstrap/AGENTS.template.md` |
| Specs | `spec/{mcp-tools,workflow-skills,documentation-system}.md` |
| Mental models | `mental-model/{documentation-system,workflow-skills}.md` |

**This asymmetry is the whole shape of the ticket.** Ceasing to *use* workset is
free and can happen today. *Removing* it is a real refactor touching a category
enum, a template, a sage-gate exemption, a spec-address exemption, and two
`go:embed`'d convention docs - and it lands a version bump because installed
plugin caches key on the version string.

## Open Questions

- **Do `.done/` worksets have to keep parsing?** Four archived tickets carry
  `workset` in their stem and category position. If the category enum drops the
  value, `tickets.list`/`project_tree` behavior over `.done/` must be checked -
  archived tickets must not start erroring. Likely cheapest answer: drop workset
  from *creation* and from all exemption logic, but keep the enum value
  parseable.
- **What happens to `260713-workset-workflow-dogfood-bugs`?** It is open. Either
  drain it before retirement, or re-categorize it. Its stem is immutable, so
  re-categorizing means a new ticket that absorbs it plus `.dropped/` for the old
  one.
- **Does `260624-epic-pre-release-cleanup` get fixed here?** It is a workset
  wearing an `epic` label. Left alone deliberately so far, precisely because
  redirecting it toward a category that is going away is wasted motion. This
  ticket unblocks that decision.
- **One commit or staged?** Stopping new workset creation is independent of the
  code removal and could land first as a cheap prose-only change.

## Phases

Outline only - this is an `idea/` ticket and the phase plan is not frozen.

### Phase 1: Stop the bleeding (prose-only)

Remove workset from the *authoring* surface: conventions `## Workset Tickets`,
the two `## Status Flow` workset lines, playbook mentions, the bootstrap
template, and the mental models. No Go change. Leaves existing workset tickets
readable and existing tooling working. Ships with a version bump so the embedded
conventions actually change for installed plugins.

### Phase 2: Remove the implementation surface

Drop workset from the category enum's *creation* path, `tickets.template`, the
sage-gate exemption, and the ready spec-address exemption. Resolve the `.done/`
parsing question first. Update the three specs. Update the Go tests that assert
workset behavior rather than deleting the assertions blindly - several exist to
pin the *exemption*, and the exemption disappearing is the behavior change under
test.

## Out of Scope

- Changing `epic` semantics. Epic stays single-outcome decomposition; see
  Background.
- Changing the goal loop / `lead-goal-step`. This ticket consumes that
  mechanism's existence as a premise; it does not modify it.
- The workflow manual's **Ticket System Concepts** epic-vs-workset rationale
  paragraph is in scope for Phase 1, but the broader manual restructure is not.
