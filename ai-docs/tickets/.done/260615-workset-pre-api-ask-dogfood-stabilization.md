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
completed: 2026-06-15
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

- `ai-docs/tickets/.done/260524-bug-codex-plugin-cache-refresh-mcp-startup-race.md`
  - done; primary cache/materialization dogfood blocker. Closed by
    `3c1518d9`: the launcher now waits briefly for `runtime.json` when the
    plugin cache is partially materialized and reports an explicit
    package-materialization diagnostic if the wait expires.
- `ai-docs/tickets/idea/260525-bug-codex-local-marketplace-worktree-cache-regression.md`
  - deferred idea; adjacent cache fidelity risk. It remains separate from the
    startup race because it concerns local marketplace refresh selecting or
    regressing to the wrong worktree source. Current installed-cache probes did
    not reproduce a sibling downgrade in this pass, so it is not an M4 blocker.
- `ai-docs/tickets/.done/260612-bug-root-aware-mcp-tool-schemas-missing-session-key.md`
  - done; closed by `601c4e25`. Root-aware MCP schemas now advertise
    `session_key` and continue omitting `root`, matching the mandatory
    session-auth runtime behavior.
- `ai-docs/tickets/idea/260612-bug-ws-rsrc-dev-server-new-file-staleness.md`
  - deferred idea; rsrc dev-server new-file staleness remains a dev-override
    investigation. The M4 path can rely on source tests and server/plugin
    reloads after adding new rsrc files.
- `ai-docs/tickets/.done/260610-bug-wsflow-runtime-contract-playbook-tools-drift.md`
  - done; current audit verified `agents-plugin-wsflow/tests` passes and the
    runtime-contract drift no longer reproduces.
- `ai-docs/tickets/.done/260612-bug-wsflow-skill-ws-dotted-namespace-ref.md`
  - done; closed by `7fd40ce8` through the skill-dispatch test realignment
    slice. The wsflow workflow manual no longer carries the forbidden dotted
    `ws.` reference.
- `ai-docs/tickets/.done/260612-bug-ws-mcp-smoke-script-stale-mercenary-surface.md`
  - done; closed by `3a03e599`. The smoke script now logs in for a
    `session_key`, calls root-aware tools with that key, and uses the current
    `mercenary register` CLI surface.
- `ai-docs/tickets/.done/260611-bug-skill-dispatch-contract-tests-stale-after-entry-shim-migration.md`
  - done; closed by `7fd40ce8`. `agents-plugin/tests` now checks playbook-backed
    procedure bodies and `agents-plugin-wsflow/tests` passes its skill-bundle
    forbidden-reference check.
- `ai-docs/tickets/.done/260611-bug-rsrc-load-unknown-playbook-misleading-error.md`
  - done; closed by `2da50a22`. Unknown playbook stems now return a distinct
    no-such-playbook diagnostic while manifest integrity failures keep
    `ErrFileMissing`.
- `ai-docs/tickets/idea/260611-bug-launcher-repair-failure-opaque-mcp-error.md`
  - deferred idea; launcher repair failures can still surface opaquely through
    host MCP `-32000`, but the package-materialization wait and refreshed smoke
    script cover the current pre-M4 dogfood failure modes.

## Planned References

- `installed plugin reload acceptance checklist`
  - not created; the package-materialization fix, root-aware schema fix, and
    refreshed smoke script give enough acceptance coverage for this workset.
- `api.ask entry readiness note`
  - satisfied by the Result section below instead of a separate ticket.

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

## Result - 2026-06-15

This workset is closed for M4 entry. The active dogfood blockers were fixed or
reclassified:

- Plugin cache/materialization race fixed by `3c1518d9`.
- Root-aware `session_key` schema discoverability fixed by `601c4e25`.
- Skill-dispatch and wsflow verification drift fixed by `7fd40ce8`.
- Source-tree ws-mcp smoke drift fixed by `3a03e599`.
- wsflow runtime-contract drift closed by current passing verification.
- Unknown playbook diagnostics fixed by `2da50a22`.

Remaining idea tickets stay open but are not M4 blockers:

- `260525-bug-codex-local-marketplace-worktree-cache-regression`
- `260612-bug-ws-rsrc-dev-server-new-file-staleness`
- `260611-bug-launcher-repair-failure-opaque-mcp-error`

Verification used for the closeout:

- `python3 -m unittest discover agents-plugin/tests`
- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `agents-plugin-tool/scripts/smoke-ws-mcp.sh ..`
- `go test -count=1 ./internal/wsrsrc ./internal/mcp`
- `go test -count=1 ./...` from `agents-plugin-tool`
