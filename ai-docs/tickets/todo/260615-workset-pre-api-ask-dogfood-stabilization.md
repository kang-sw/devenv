---
title: pre-api.ask dogfood stabilization workset
related:
  260605-epic-ws-playbook-factory-pivot: parent epic whose M4 api.ask redesign should wait for this dogfood pass
  260609-refactor-ws-api-ask-corpus-routing: next planned M4 implementation target after this workset is cleared or explicitly deferred
related-mental-model:
  - mcp-runtime
  - plugin-runtime
  - prompt-bundle
  - workflow-skills
---

# pre-api.ask dogfood stabilization workset

## Context

The playbook-factory pivot core is mostly landed, and the next epic milestone is
M4 api.ask corpus routing. Before entering that feature slice, dogfood findings
from the Codex plugin reload and installed-MCP loop should be triaged into a
short stabilization pass so M4 does not mix feature work with toolchain
reliability noise.

This workset is a non-hierarchical operating context. It does not make the
listed tickets children, does not own their implementation phases, and is not
itself an implementation target.

## Tickets

- `ai-docs/tickets/idea/260524-bug-codex-plugin-cache-refresh-mcp-startup-race.md`
  - idea; primary cache/materialization dogfood blocker. Covers plugin reload
    states where the MCP process and installed plugin cache can temporarily
    disagree.
- `ai-docs/tickets/idea/260525-bug-codex-local-marketplace-worktree-cache-regression.md`
  - idea; adjacent cache fidelity risk. Keep separate from the startup race
    because it concerns local marketplace refresh selecting or regressing to the
    wrong worktree source.
- `ai-docs/tickets/idea/260612-bug-root-aware-mcp-tool-schemas-missing-session-key.md`
  - idea; root-aware MCP schemas omit the mandatory `session_key`, which makes
    dogfood calls fail unless the caller already knows to pass an unadvertised
    field.
- `ai-docs/tickets/idea/260612-bug-ws-rsrc-dev-server-new-file-staleness.md`
  - idea; rsrc dev-server staleness concern. Recheck against recent reload
    observations before deciding whether it is the same failure class as plugin
    cache materialization or a separate dev-override bug.
- `ai-docs/tickets/idea/260610-bug-wsflow-runtime-contract-playbook-tools-drift.md`
  - idea; wsflow runtime contract drift around playbook tools. Relevant because
    wsflow parity tests are part of the release-confidence gate.
- `ai-docs/tickets/idea/260612-bug-wsflow-skill-ws-dotted-namespace-ref.md`
  - idea; currently reproduced by `python3 -m unittest discover
    agents-plugin-wsflow/tests` as the lone known wsflow skill-bundle failure.
- `ai-docs/tickets/idea/260612-bug-ws-mcp-smoke-script-stale-mercenary-surface.md`
  - idea; smoke verification still points at retired mercenary/agent names and
    should not remain stale when dogfood confidence depends on smoke output.
- `ai-docs/tickets/idea/260611-bug-rsrc-load-unknown-playbook-misleading-error.md`
  - idea; rsrc load diagnostics can mislead callers during prompt/playbook
    iteration.
- `ai-docs/tickets/idea/260611-bug-launcher-repair-failure-opaque-mcp-error.md`
  - idea; launcher repair failures surface opaquely, which raises the cost of
    diagnosing plugin reload and installed-runtime failures.

## Planned References

- `installed plugin reload acceptance checklist`
  - intended role: small checklist or ticket if repeated reload verification
    keeps relying on manual probes; create only if the existing cache and smoke
    tickets do not give enough acceptance coverage.
- `api.ask entry readiness note`
  - intended role: handoff note for `260609-refactor-ws-api-ask-corpus-routing`
    once the dogfood blockers are fixed, dropped, or explicitly deferred.

## Focus

Current focus is a pre-M4 dogfood pass:

1. Classify plugin cache reload failures first, because stale rsrc/runtime
   materialization can invalidate every later installed-plugin probe.
2. Fix root-aware schema discoverability next, because routine ws calls should
   not depend on hidden `session_key` knowledge.
3. Clear wsflow and smoke-test drift enough that the standard verification
   surface reports only known, explicitly deferred failures.
4. Re-enter `260609-refactor-ws-api-ask-corpus-routing` only after remaining
   dogfood items are either closed or documented as non-blocking for M4.

## Exit Criteria

- Done: plugin reload/cache behavior has an accepted fix or a documented
  non-blocking boundary; root-aware `session_key` discoverability is fixed or
  explicitly deferred; wsflow/smoke verification no longer hides unrelated drift
  behind stale tests; M4 `api.ask` can start without mixing feature work with
  current dogfood failures.
- Deferred: dashboard-specific polish, named-agent retained-path bugs unrelated
  to the M4 api-doc path, and broader release-process improvements remain in
  their existing tickets unless they block the pre-M4 probes.
