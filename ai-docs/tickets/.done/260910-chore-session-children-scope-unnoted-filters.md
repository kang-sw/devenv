---
title: "Add scope and unnoted filters to session.children so the lead-run worker-key lookup reads one row, not the whole subtree"
related:
  260910-bug-lead-run-worker-key-lookup-ignores-delegate-scope: sibling; that ticket fixed the correctness of the control-scope worker-key rule, this one adds the matching volume fix so the lookup returns a single row
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 90110a3a7cd7b32e
sage-review-completeness-reviewed: 90110a3a7cd7b32e
completed: 2026-09-10
---

# Add scope and unnoted filters to session.children so the lead-run worker-key lookup reads one row, not the whole subtree

## Background

`lead-run` step 2 finds the freshly rendered worker's key by calling
`session.children` on the lead key and picking the one un-noted `scope:
control` child. `session.children` returns the lead key's entire child
subtree unconditionally, so that lookup pays for the whole history to find
one row. After one epic the lead key had 26 children — every finished worker
plus every disposable delegate (populator, sage reviewers, code reviewers),
each with its note body, roughly 5k tokens, and the run path reads it twice
per cycle.

This is a lead-context cost with no cheap-model substitute. `session.children`
is a lead-only call and its result lands in the lead's context by
construction, so filtering the result is the only lever — the work cannot be
pushed to a cheaper delegate the way file reads can. The data the filter needs
already exists: `session.children` computes each child's scope
(`sessionChildScopeLabel` maps a lead-capability child to `control` and a
delegate child to `delegate`) and already knows whether a child carries a
note. It simply returns everything, so the caller that wants the single fresh
worker key must scan the full subtree to find it.

## Decisions

- **Selection stays on the Explore delegate; it is not moved into the lead.**
  An earlier draft of this ticket also proposed replacing `lead-run`'s
  Explore-based ticket selection with a direct `tickets.query` in the lead.
  Rejected on pricing, not on tokens: the Explore selection's ~15k tokens run
  on a cheap model, while moving selection into the lead spends fewer tokens
  but on a model roughly 30-40x more expensive, so the dollar cost rises even
  as the token count falls. The Explore spawn is cheap-model dollars; the lead
  is not. Two further facts make the move a clear loss: `tickets.query` does
  not expose the `## Blocked (...)` run-skip marker and nothing in the tooling
  parses it (only the file-reading Explore honors it), so a lead-side
  selection would need a new parser and a new query field; and the current
  playbook deliberately says the lead must not list `ready/` itself. If
  selection is ever revisited, the direction is to make the selection delegate
  call `tickets.query` instead of reading files — keeping the cheap model —
  not to move the work into the lead.
- **Do not auto-prune resolved children.** The notes on finished workers are
  the carry-over record a restarted or compacted lead rebuilds from through
  `session.children`, so they must stay queryable. The fix is a
  default-narrowing filter the caller opts into, not deletion.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/server.go (session.children handler and tool schema, L1516-1563 and L2884-2897), agents-plugin/rsrc/lead-run/lead-run.md (step 2, L46-54), agents-plugin-wsflow/rsrc/lead-run/lead-run.md (byte-identical mirror), agents-plugin/rsrc/manifest.json |
| scope.surface | public-interface | session.children's MCP tool input schema gains new caller-facing filter fields (server.go#L2885-2896) |
| scope.new_public_symbol | no | can extend the existing unexported handleSessionChildren/sessionChildScopeLabel (server.go#L1516, #L1599); no new exported Go identifier is required |
| scope.new_type_contract | yes | adds new optional scope and unnoted-only fields to session.children's inputSchema (server.go#L2887-2896), additive and default-preserving |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/session_auth_test.go already exercises session.children (L932-1150) |
| complexity.reuse_points | confirmed | the existing live/include_dead filter loop (server.go#L1535-1540) and sessionChildScopeLabel (server.go#L1599-1610) are the direct precedent for the new filters |
| complexity.side_effect_risk | low | pure read-path filter addition; new params default to today's unfiltered return so no existing caller's behavior changes |
| risk.correctness | low | follows the existing live/include_dead filter precedent in the same function; the only new branching is the three-way scope match (control/delegate/any) |
| risk.fit | low | matches the ticket's own decision to keep this a default-narrowing, opt-in filter rather than deletion, consistent with the existing include_dead pattern |
| risk.test | low | session_auth_test.go already has helpers (callToolOnce, JSON response parsing) to extend with scope/unnoted cases |
| risk.security_or_contract | low | additive change to a tool already gated lead-only (roleAllowsTool blocks the session.* prefix for delegate/leaf, server.go#L3679); no new exposure |

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Phases

### Phase 1: Add the filters and wire the lookup

Add filter parameters to `session.children`: a scope selector (control /
delegate / any) and an unnoted-only toggle, both defaulting to today's
behavior — return the whole subtree — so every existing caller is unaffected.
Then wire `lead-run` step 2 to call `session.children` with scope control and
unnoted-only, so the worker-key answer is one row instead of the full
subtree; this completes the control-scope correctness fix
(`260910-bug-lead-run-worker-key-lookup-ignores-delegate-scope`) with its
matching volume fix. Regenerate the rsrc manifest and the wsflow mirror, then
extend the existing `session.children` coverage in
`agents-plugin-tool/internal/mcp/session_auth_test.go`: a case per scope value
(control / delegate / any), the unnoted-only toggle on and off, and one
asserting the no-argument default still returns the whole subtree unchanged.
Verify on one installed-build cycle that the lookup returns a single row.

Rejected: pruning resolved keys automatically (see Decisions) — the fix is a
narrowing filter, not deletion.

### Result (00f8f976) - 2026-09-10

Added optional `scope` (`control`, `delegate`, `any`) and `unnoted_only`
filters to the existing handler and MCP schema. Invalid filter values return
an error. Filtering happens after traversal, preserving matching descendants
under excluded ancestors. `any` includes leaf scope. No records or notes are
pruned. `lead-run` now requests the unnoted control child explicitly; the
manifest and byte-identical wsflow mirror were regenerated.

The ticket's whole-subtree shorthand does not describe the existing default:
no optional arguments return immediate live children, while `depth: 0`
requests the full subtree. Both existing behaviors remain unchanged, including
response fields and ordering.

Verification:

- New filter tests initially failed because the implementation did not yet
  honor the new fields; after implementation, `go test ./internal/mcp -run
  TestSessionChildren -count=1` passed. Coverage includes each scope, toggle
  on/off, omission defaults, nested matches, dead keys, invalid values, and
  text/JSON output.
- `TMPDIR=/private/tmp go test ./...` passed. The initial plain command failed
  in an unrelated absolute-ticket-path test under macOS's `/var` symlink
  alias; no source or test change was made for that failure. Follow-up:
  `260910-bug-route-ticket-absolute-path-symlink-alias`.
- `scripts/smoke-ws-mcp.sh ..` passed.
- Both documented rsrc manifest/mirror regeneration commands passed with
  `-count=1`; `python3 -m unittest discover agents-plugin-wsflow/tests`
  passed all 10 tests; `git diff --check` passed.
- Installed-cache launcher source-build cycle passed using the installed
  `ws/0.45.2/bin/ws-mcp-launcher.py`, its existing local-runtime marker,
  a temporary isolated session cache, and current source rsrc via
  `WS_RSRC_ROOT`. The schema advertised both fields. Rendering a noted old
  worker, an unnoted reviewer, and a fresh worker produced three children;
  the filtered lookup returned exactly one unnoted control child in JSON
  and text. This validates the installed launcher and current runtime;
  the existing host connection was not reconnected and installed prompt
  files were not refreshed.

Independent full-scope review of `epic/refound..00f8f976` was clean, including
fresh-reader audit, committed mirror/digest verification, and an independent
focused test run. No findings remain. Source inputs stayed unchanged after
that verified commit. Implementation is retained on
`impl/epic/refound/sweat-thaw-thumb`; merge into `epic/refound` belongs to the
lead.
