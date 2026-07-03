---
title: "Flip sage_review plugin builtin default from unset to auto"
related:
  260626-bug-sage-review-config-setter-missing: adjacent gap in the sage_review config surface (missing lead-facing setter/tuning knob); this ticket only changes the shipped builtin default value, not the setter surface
sage-review: completed
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
- **Required, not optional**: the `tickets.move` (`server.go:1063`) and
  `tickets.create` (`server.go:1086`) call sites — the exact two consumers
  this ticket targets — build their resolver with
  `wsconfig.NewResolver(wsconfig.Options{}, nil, adapter, adapter)`, passing a
  literal `nil` for `builtinDefaults`. `NewResolver` substitutes `nil` with an
  empty map (`wsconfig/resolver.go:65-67`), so anything added to
  `builtinConfigDefaults()` is structurally unreachable from these two call
  sites as written. Every other `NewResolver` call site in `server.go` that
  wants the builtin floor calls `builtinConfigDefaults()` explicitly (lines
  523, 553, 596, 733, 752, 1184, 3925) — 1063 and 1086 are outliers. Swap
  `nil` → `builtinConfigDefaults()` at both call sites as part of this phase.
  (Two other `nil`-passing sites exist, `config.prompt.set`/`unset` at
  `server.go:651` and `702` — these resolve `prompt.*` override keys via
  `resolver.Set`/`Unset` only, never read `ItemSageReview` or any other
  builtin-defaulted item through `Get`, so they are not part of this bug and
  are left unchanged.)
- Verification: with no project-scope `sage_review` override present, run
  `ws/tickets.create`/`ws/tickets.move` and confirm the resolved posture is
  `required` (not `skipped`) — this assertion only holds once the `nil` →
  `builtinConfigDefaults()` swap above is also made, since the default alone
  is unreachable without it. Confirm existing tests that assert the current
  `skipped`-by-default behavior are updated to the new expectation, and that
  a test asserts an explicit project-scope `sage_review` override still wins
  over the new builtin default.

### Result

Implemented on `implement/sage-review-builtin-default-auto`
(`58dd7a66..d62474c5`, 3 commits). Added `wsconfig.ItemSageReview: "auto"` to
`builtinConfigDefaults()`, updated its doc comment, and swapped `nil` →
`builtinConfigDefaults()` at both `tickets.move`/`tickets.create`
`NewResolver` call sites (`server.go:1063`, `1086`) — survey confirmed no
other `NewResolver` nil-passing site was in scope. Added tests confirming
no-override resolves to `required` for both tools and that an explicit
project-scope override still wins.

Partitioned review: fit clean, test non-clean (1 critical) — the two
"defaults-to-required" tests set `WS_CACHE_HOME` but not `WS_CONFIG_HOME`,
so they leaked the real `~/.ws/config.json` global scope on the review
machine and passed for the wrong reason rather than exercising the nil-swap
fix. Fixed in `d62474c5` by isolating `WS_CONFIG_HOME` per the repo's
existing test pattern; verified with an explicit revert experiment (tests
correctly FAIL without the production fix, PASS with it restored).

Plugin version bumped 0.32.1 → 0.32.2 (dev-merge rule) in `aebc4d68`.

