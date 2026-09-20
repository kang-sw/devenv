---
title: "ws MCP input-shape coercion traps: create_empty stem + git ai_context"
parent: 260909-epic-ws-worker-interpreter-refoundation
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 975e826f0e6baf69
sage-review-completeness-reviewed: 975e826f0e6baf69
---

# ws MCP input-shape coercion traps

## Background

Two recurring caller mistakes surfaced during dogfooding, both with the same
root cause: an MCP tool demands an input shape that diverges from the shape the
same concept carries everywhere else in the workflow, and then *silently
coerces* a mismatched input instead of rejecting it clearly.

- **`tickets.create_empty` doubles the date prefix.** `stem` is contractually a
  *dateless semantic* stem; `TicketCreate` unconditionally prepends today's
  date (`fullStem := today + "-" + stem`,
  `agents-plugin-tool/internal/wsdoc/ticket_create.go:59`). A caller passing a
  dated stem (`260920-feat-foo`) — the form "stem" takes in filenames,
  `git log --grep`, and every ticket reference, including AGENTS.md's own
  "Reference tickets by stem" example — yields `260920-260920-feat-foo.md`. No
  guard; the wrong artifact is created silently.
- **`git.commit` / `git.merge` `ai_context` string-vs-array.** The arg is
  `array<string>`, one bullet per element
  (`agents-plugin-tool/internal/mcp/server.go` schema). A caller passing a
  single dash-bulleted prose string (the shape a commit message's
  `## AI Context` block takes everywhere) hits `stringList` /
  `stringListKeepBlank` (`server.go:4534,4556`), which return `nil` for any
  non-`[]any` value. The exact reported message differs by tool but both name
  *emptiness*, not *wrong type*: `git.merge` reports literally "requires
  nonempty ai_context" (`agents-plugin-tool/internal/mcp/git_merge.go:93`);
  `git.commit` reports "ai_context is required: no ai_context field was
  received" (`agents-plugin-tool/internal/wsgit/git.go:628`), indistinguishable
  from a caller that omitted the field entirely.

Both are host-neutral MCP tooling defects (Arch Rule 4 clean); the fix ships in
`ws` proper. The behavioral contract is the test suite under
`agents-plugin-tool/internal/`.

## Decisions

- **Date dedup is lenient only when unambiguous.** A leading `\d{6}-` prefix
  equal to today is a harmless duplicate → strip and proceed. A leading
  `\d{6}-` prefix that differs from today is ambiguous between an intentional
  backdate and a mistake → reject with an explicit message, never silently
  strip-and-re-date. This honors AGENTS.md "Creation-date prefixes are stable;
  never rename to change the date." A leading `\d{6}-` is never a valid semantic
  stem (categories are feat/bug/refactor/chore/research/epic, never numeric), so
  detection is unambiguous. The differ-reject message names the collision
  concretely, e.g. `stem carries date prefix "<embedded>-" but today is
  <today>; pass a dateless semantic stem (the date is prepended
  automatically)` — exact wording is the implementer's, but it must name both
  dates and the dateless-stem contract. Rejected: silently honoring the
  embedded date as a backdate (riskier, hides mistakes); silently re-dating to
  today (mutates the author's declared date).
- **`ai_context` type mismatch is rejected, not coerced.** When `ai_context`
  (or another array-typed arg) is present but not an array, reject with a
  type-accurate message ("ai_context must be an array of strings, one bullet per
  element; got <type>") instead of coercing to `nil` and reporting emptiness.
  How the runtime `<type>` token is rendered, and whether the guard sits in the
  shared helper or at each of the two downstream paths (`git.merge`'s own
  emptiness check vs `git.commit`'s wsgit `normalizeCommitOptions`), is a wiring
  detail the implementer settles; the committed contract is only "reject a
  present non-array with a type-accurate message." Rejected: leniently splitting
  a prose string on `- ` bullets — a clear rejection is the safer default and
  avoids mangling multi-line bullets.

## Constraints

- `agents-plugin-tool/internal/mcp/` — read `ai-docs/manuals/ws-mcp.md` before
  editing (MCP dispatch, schemas, tool allowlist; the manual is the operational
  runbook, the test suite is the contract).
- Caller-visible behavior changes (new rejections, the dedup) require tests;
  tests are the behavioral contract, there is no separate behavior document.
- `stringList` / `stringListKeepBlank` are shared by several array-typed args
  (`updated_tickets`, `stems`, note/todo arrays). A helper-level typed-reject
  change has broad blast radius; a per-call validation is surgical. The
  implementer chooses and records the choice under `decisions:` — this is an
  implementation-strategy call, not a workflow-policy decision.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 260725-idea-ws-git-commit-rename-and-payload-rejections (2026-07-25, Result): "Phase 2 (`ai_context` constraint) remains open and independent." — bearing: constrains
- 260720-wsdoc-commit-boundary (2026-07-25, commit 4dee23d7): "git.commit's ai_context rejection names absent/empty/all-blank distinctly; the MCP case must use stringListKeepBlank (not stringList) so [\"\"] does not collapse into empty-array" — bearing: constrains
- 260622-feat-sage-review-ticket-gate (2026-06-22, commit 8ced5351): "Auto-prefixes today's date to semantic `stem`." — bearing: constrains
- 260620-feat-ws-ticket-status-transition-tools (2026-06-20, ticket Decisions): "Rules become guards: reject an unknown stem or invalid target status; preserve the immutable `YYMMDD` date prefix; reject re-closing an already-closed ticket" — bearing: supports
- 260911-feat-ws-git-merge-lead-owned-merge-authority (2026-09-11, commit f6255a1f): "Phase 1 adds a lead-only git.merge tool that derives the target from the local impl ref, validates optional assertions, and preserves merge records through the shared commit-message builder." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdoc/ticket_create.go, agents-plugin-tool/internal/mcp/server.go, agents-plugin-tool/internal/mcp/git_merge.go |
| scope.surface | public-interface | existing MCP tools tickets.create_empty, git.commit, git.merge change caller-visible input-validation/rejection behavior (server.go:3727, 3743, 4001) |
| scope.new_public_symbol | no | no new tool or exported symbol; the fix changes existing tools' validation behavior |
| scope.new_type_contract | unknown | Constraints explicitly defers helper-level vs per-call validation shape to the implementer's choice |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdoc/ticket_create_test.go, agents-plugin-tool/internal/mcp/git_merge_test.go, agents-plugin-tool/internal/wsgit/git_test.go |
| complexity.reuse_points | confirmed | stringList/stringListKeepBlank (server.go:4534,4556) and TicketCreateOptions.Today already exist for the implementer to build on |
| complexity.side_effect_risk | moderate | stringList/stringListKeepBlank are shared by updated_tickets, stems, and note/todo arrays (ticket Constraints); a helper-level change has broad blast radius |
| risk.correctness | moderate | date-prefix regex must not misclassify a valid semantic stem or mishandle a date-rollover edge case |
| risk.fit | low | extends the absent/empty/all-blank diagnostic convention already landed for git.commit ai_context (260725-idea-ws-git-commit-rename-and-payload-rejections) |
| risk.test | low | ticket enumerates the exact branches to cover and existing test files already establish the pattern to follow |
| risk.security_or_contract | moderate | changes caller-visible accept/reject behavior of existing public MCP tools (tickets.create_empty, git.commit, git.merge) |

## Phases

### Phase 1: Reject or dedup mismatched tool inputs

Fix both coercion traps; they share the root cause but are independent edits.

- **create_empty date dedup** (`internal/wsdoc/ticket_create.go`,
  `internal/mcp` schema/tests): detect a leading `\d{6}-` on `stem`; strip when
  it equals today, reject when it differs, per the Decisions above. Cover both
  branches (equal-today dedup, differ-reject, no-prefix passthrough) with tests.
- **ai_context typed rejection** (`internal/mcp/server.go` commit/merge arg
  validation, or the shared `stringList`/`stringListKeepBlank` helpers): when an
  array-typed arg is present but not a JSON array, reject with a type-accurate
  message rather than coercing to `nil`. Decide helper-level vs per-call per the
  Constraints and record it. Cover the wrong-type-present case (distinct from
  genuinely-empty) with a test; keep the existing "present but all blank"
  classification intact.
