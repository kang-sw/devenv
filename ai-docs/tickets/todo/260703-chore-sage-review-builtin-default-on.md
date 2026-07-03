---
title: "Flip sage_review plugin builtin default from unset to auto"
related:
  260626-bug-sage-review-config-setter-missing: adjacent gap in the sage_review config surface (missing lead-facing setter/tuning knob); this ticket only changes the shipped builtin default value, not the setter surface
sage-review: required
---

# Flip sage_review plugin builtin default from unset to auto

## Background

`wsconfig.ItemSageReview` (`agents-plugin-tool/internal/wsconfig/scope.go:34-38`)
accepts raw config values `off|ask|auto`, resolved by
`wsdoc.ResolvedSageReviewPosture` (`agents-plugin-tool/internal/wsdoc/tickets_mutate.go:220-229`)
into the ticket-frontmatter posture vocabulary: `off`/empty/unset → `skipped`,
`ask` → `recommended`, `auto` → `required`. `ItemSageReview` defaults to
`ScopeProject` (`scope.go:76`), so an explicit project-scope value always wins
over the builtin floor.

`builtinConfigDefaults()` (`agents-plugin-tool/internal/mcp/server.go:319-324`)
currently has entries only for `ItemWorkflowPreferSubagent` (`"off"`) and
`ItemWorkflowPreferMercenary` (`"hide"`) — there is no entry for
`ItemSageReview`. Any project with no explicit `sage_review` override
therefore resolves to an empty value at the `tickets.create`/`tickets.move`
call sites (`server.go:1063-1064`, `1086-1087`, both pass `nil`
builtinDefaults today), which maps to `skipped`: the sage-review gate never
runs by default.

During this session's dogfooding, the sage-review reviewer pair
(`ticket-reviewer-design`, `ticket-reviewer-completeness`) caught a genuine
critical design gap in a sibling ticket
(`260703-chore-prefer-subagent-verify-discussion-inline-mirror`) that the
human+lead discussion had missed — a real runtime consumer
(`lead-workflow-manual`'s keyless prefer-subagent embedding) that a planned
rsrc-playbook removal would have silently broken. Given this observed hit
rate, the project should default new tickets to running the gate rather than
requiring each project to opt in.

## Decisions

- Add `wsconfig.ItemSageReview: "auto"` to `builtinConfigDefaults()` in
  `agents-plugin-tool/internal/mcp/server.go`. This is the raw config value
  that resolves to the `required` ticket posture (always run the gate without
  asking) — not the string `"required"` itself, which is a
  `sage-review:`-frontmatter-only posture value, never a raw `sage_review`
  config value.
- Also update the `ItemSageReview` doc comment in `scope.go:34-38` ("Builtin
  default: off (absent = disabled)") to reflect the new default.
- Projects with an existing explicit `sage_review` project-scope (or
  session/global) value are unaffected: `ScopeProject`/session/global always
  outrank the builtin floor. This change only affects fresh installs or
  projects that never set `sage_review` explicitly.
- No change to the `sage_review` value vocabulary, the setter surface, or the
  resolution/posture-mapping logic itself — those are unchanged and (setter
  surface aside) already match the intended design; the apparent vocabulary
  mismatch noticed during scoping (`ticket_create.go`'s doc comment listing
  `off|auto|ask` vs. `tickets_mutate.go`'s `recommended|required|skipped`
  branching) is not a bug — they are two different vocabularies for two
  different fields (raw `sage_review` config input vs. derived
  `sage-review:` ticket-frontmatter posture) and need no fix here.
- Out of scope: `260626-bug-sage-review-config-setter-missing` (no
  `config.tuning`/`ws:lead-tune` writer for `sage_review` exists yet) is a
  separate, pre-existing gap. This ticket does not depend on it and does not
  resolve it — a project that wants to opt back out of the new default can
  still only do so via the manual config JSON edit that ticket describes,
  until that setter ships.

## Spec Impact

`ai-docs/spec/mcp-tools.md` (`{#260620-ticket-move-tool}`,
`{#260622-create-ticket-tool}`) already documents the resolved-value-to-posture
mapping (`skipped` for `off`/empty/unset, `recommended` for `ask`, `required`
for `auto`) as the caller-visible contract; that mapping is unchanged by this
ticket. The spec does not state what the shipped builtin default value is, so
changing which value ships as the floor is not a contract change and needs no
spec edit. Contract-first spec: no.

## Phases

### Phase 1: Add the builtin default

- Add `wsconfig.ItemSageReview: "auto"` to `builtinConfigDefaults()`
  (`agents-plugin-tool/internal/mcp/server.go:319-324`).
- Update the doc comment on `ItemSageReview` in
  `agents-plugin-tool/internal/wsconfig/scope.go:34-38` to state the new
  builtin default (`auto`).
- Verification: with no project-scope `sage_review` override present, run
  `ws/tickets.create`/`ws/tickets.move` and confirm the resolved posture is
  `required` (not `skipped`). Confirm existing tests that assert the current
  `skipped`-by-default behavior are updated to the new expectation, and that
  a test asserts an explicit project-scope `sage_review` override still wins
  over the new builtin default.

