---
title: "git.commit writes `## Updated Tickets` while every convention surface says `## Ticket Updates`"
---

# git.commit writes `## Updated Tickets` while every convention surface says `## Ticket Updates`

## Problem

`agents-plugin-tool/internal/wsgit/git.go:779` composes the ticket section as
`## Updated Tickets`. Every surface that documents or parses the section uses
`## Ticket Updates`:

- `agents-plugin/skills/lead-bootstrap/AGENTS.template.md` (the commit-message
  template, plus migration items v0009 and v0023 that add and order it)
- `agents-plugin/skills/lead-bootstrap/WORKFLOW.md`
- `agents-plugin/rsrc/doc-gap-discovery/doc-gap-discovery.md`, whose grouping
  rule keys on `## Ticket Updates`
- `agents-plugin-tool/internal/wsdoc/legacy_marker.go`, whose comment names
  `## AI Context` / `## Ticket Updates` / `## Spec` as the sections
- this repository's own `AGENTS.md` `### Commit Rules`

So a commit made through `ws/git.commit` carries a heading that the documented
convention does not define and that at least one shipped consumer
(doc-gap-discovery grouping) does not match, while a commit written by hand
against the template carries the other. Both forms are present in this
repository's history.

## Surprise

Found while dogfooding: two commits authored in the same session for the same
ticket ended up with different section headings depending on whether they went
through the tool or through `git commit -F`. Nothing warned.

## Open questions

- Which spelling is canonical? `## Ticket Updates` has the larger surface and
  is the one shipped text teaches, which argues for changing the emitter.
- Changing the emitter changes observable commit-message output, so it is an
  "ask first" change under the Approval Protocol; it may also want a migration
  item for projects whose `AGENTS.md` already documents the other form.
- Should any consumer accept both spellings during the transition, given that
  history contains both?

## Result

The emitter changed, not the convention: `CommitMessage` in
`agents-plugin-tool/internal/wsgit/git.go` now writes `## Ticket Updates`.
`## Ticket Updates` has the larger surface (the bootstrap `AGENTS.template.md`
commit-message template and its migration items, the bootstrap `WORKFLOW.md`,
and this repository's `AGENTS.md` `### Commit Rules`), so the single emitter
was the outlier. The lead authorized the observable-output change.

Answers to the open questions:

- Canonical spelling: `## Ticket Updates`.
- No migration item is needed. The bootstrap template already teaches
  `## Ticket Updates` and its v0009 migration item already adds that heading,
  so a downstream `AGENTS.md` produced by any template version already matches
  the new emitter output.
- No consumer needs to accept both spellings: nothing under
  `agents-plugin-tool/` parses either heading, and the `doc-gap-discovery`
  playbook the ticket named as a keyed consumer no longer exists. History
  keeps both forms; nothing reads it by heading.

Three tests pinned the old string (`internal/wsgit/git_test.go` x2 plus a
comment, `internal/mcp/server_test.go` x1) and were updated with the emitter.
Documentation of history in closed tickets and plans keeps the old spelling.
