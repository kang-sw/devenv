---
title: two MCP JSON/response fields were removed with no deprecation path for out-of-repo callers
related:
  260726-refactor-retire-spec-planned-marker-mechanism: the retirement whose Phase 2 made both removals below
---

# two MCP JSON/response fields were removed with no deprecation path for out-of-repo callers

## Background

Two separate field removals landed on this branch. Both were verified by live
A/B: running `main` and goal-branch binaries against the same fixture, driving
the real MCP call path, and diffing the response.

**1. `specs.list(format=json)` dropped `marker_contexts` / `marker_context`.**
`agents-plugin-tool/internal/wsdoc/spec_discovery.go:31,54`:
`SpecInfo.MarkerContexts` and `SpecAnchorInfo.MarkerContext` changed from
`json:"marker_contexts,omitempty"` / `json:"marker_context,omitempty"` to
`json:"-"`. For an identical fixture spec, `main` returns
`"marker_contexts": [...]` in the JSON response; the goal branch omits the key
entirely. `toolJSONResponse` is a bare `json.Marshal` with no field-filtering
layer, so a struct tag change is a direct wire-contract change — there is no
intermediate schema or versioning layer that would have caught or gated it.

**2. `tickets.sage_gate` dropped its commit line.**
`agents-plugin-tool/internal/mcp/server.go` around lines 1394-1420 and
2816-2822: the tool lost its automatic ask-decline commit and the
`commit: <hash> (<title>)` response line it used to emit, replaced by an
`advisory: <text>` line instead.

Neither removal is unmotivated — both are consequences of
`260726-refactor-retire-spec-planned-marker-mechanism` (`marker_contexts`
existed to surface the retired planned-marker mechanism; the sage_gate commit
line was tied to a flow the retirement restructured). The one in-repo caller of
`tickets.sage_gate` (`lead-write-ticket`) was co-updated to match. No in-repo
caller reads `marker_contexts`/`marker_context`, and neither field is
documented in `ai-docs/spec/mcp-tools.md`.

## Why this is worth deciding rather than just fixing

Both removals are individually justified by the retirement's internal logic.
What is not settled is a standing policy question that this removal is only
the latest instance of: **does this project consider the MCP JSON surface a
public contract with out-of-repo consumers?**

- If yes: both removals needed a deprecation path (a transition window, a
  documented breaking-change note, or at minimum an entry in
  `ai-docs/spec/mcp-tools.md` marking the fields removed), and neither got one.
- If no: the honest fix is not to retrofit a deprecation process onto these two
  removals after the fact, but to say the "no external contract" position
  somewhere durable — likely `ai-docs/spec/mcp-tools.md` itself — so that the
  next removal doesn't re-litigate the same question from scratch.

Fixing either field in isolation (restoring it, or writing a changelog entry
just for these two) would answer the wrong question. The question is the
contract policy, not these two fields specifically.

## Non-Scope

- Does not restore either removed field. If the answer is "yes, it's a
  contract," restoration or a deprecation shim is a separate follow-up ticket.
- Does not audit the full MCP tool surface for other undocumented breaking
  changes on this branch; these two were the ones caught by the regression
  review's A/B pass.
