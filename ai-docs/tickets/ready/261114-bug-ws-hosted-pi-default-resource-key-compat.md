---
title: "Hosted Pi default mode fails when the host lacks getResourcePathKey"
related:
  261113-bug-ws-release-gate-sweep-fixes: "completed sweep that identified this separate Important compatibility finding"
  261113-feat-ws-hosted-lead-bundled-plugin-only: "preserve native resource loading and default-mode extension restoration"
  261114-bug-ws-pi-version-probe-never-refreshes: "distinct daemon executable/version-probe incident; not the cause here"
  261114-bug-ws-installed-plugin-missing-native-pi-payload: "distinct installed-package omission; not the cause here"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: e201fd039fe83548
sage-review-completeness-reviewed: e201fd039fe83548
---

# Hosted Pi default mode fails when the host lacks getResourcePathKey

## Background

The original Important finding is a default-mode hosted Pi startup failure on
Pi 1.0.4, not the later stale daemon version probe or missing installed native
payload. The release-gate sweep's 2026-11-14 Result recorded that the installed
Pi 1.0.4 host did not export `getResourcePathKey`
(`ai-docs/tickets/.done/261113-bug-ws-release-gate-sweep-fixes.md#L313-L320`). The current default-mode bootstrap imports that helper
from the selected host's `core/source-info.js` and passes it to
`installResolver`, which rejects a missing function before the real CLI starts
(`agents-plugin-tool/internal/wsagent/pi_lead.mjs`). An isolated extraction of
the npm-cached `@earendil-works/pi-coding-agent@1.0.4` confirms that
`dist/core/source-info.js` lacks the helper, while
`dist/core/package-manager.js` deduplicates extensions with `canonicalizePath`
from `dist/utils/paths.js`. The package's published CLI is
`dist/bundle/cli.js`; its adjacent SDK must remain the selected module graph.

The owner assigned EXEC 3 to fix this Important failure on 2026-11-14. Existing
default-mode semantics remain the contract: Pi's own loader owns trust,
settings, extension filters, metadata, ordering, initialization and reload;
the adapter replaces only installed ws/wsflow extensions with the bundled
extension and restores enabled non-ws ambient extensions. Starting isolated
instead would not satisfy the requested default-mode behavior.

## Decisions

- Repair the existing Pi 1.0.4 default-mode compatibility failure without
  changing the requested isolation mode, launcher identity or native loader
  ownership. Preserve the newer-host path and fail closed for an actually
  incompatible resolver.
- Prefer the selected host's `getResourcePathKey` when available; otherwise
  use that same host's native `canonicalizePath`, matching the confirmed
  Pi 1.0.4 package-manager deduplication semantics. Reject an unavailable or
  incompatible native resolver instead of substituting a global SDK, changing
  isolation, or swallowing the failure.
- Compatibility tier T0: this repairs an internal host compatibility adapter;
  no MCP argument, API/protocol semantics, shared-store shape, tracker event,
  manifest field or version bump changes.

## Constraints

- Do not absorb the stale version probe, missing installed native payload,
  hosted Bad Request, or prompt-admission incidents into this ticket.
- Do not replace the shared daemon, modify live installation/configuration,
  start a model request or push main. Verification uses temporary host/resource
  fixtures and process-local SDK/CLI invocations.
- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before tooling edits. Consult the
  selected Pi SDK/resource-loader documentation and implementation before
  changing its integration.
- Keep the native SDK and CLI in the same host module graph. Preserve
  synthetic resource identities and native filesystem deduplication.

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Prior Decisions

- 261113-bug-ws-release-gate-sweep-fixes (2026-11-14, Result): "The installed Pi 1.0.4 has no `getResourcePathKey` in its SDK, and the review saw the same failure with the 0.84.4 dependency." — bearing: supports
- 261113-feat-ws-hosted-lead-bundled-plugin-only (2026-11-13, Result): "Default mode re-adds enabled non-ws resources through Pi's native in-process resolver rather than a preflight enumeration" — bearing: constrains
- 52dc582b (2026-11-13, commit): "Pi restoration wraps its own in-process SDK resolver instead of pre-enumerating extensions, preserving project trust hooks, ordering, package filters and reload without initializing extensions twice." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsagent/pi_lead.mjs and existing testdata/pi-lead.test.mjs; targeted Go launch/isolation tests |
| scope.surface | internal | Embedded host adapter; existing installResolver export is internal to the adapter/tests, not an MCP API |
| scope.new_public_symbol | no | No new public symbol requested; repair existing installResolver/bootstrap |
| scope.new_type_contract | no | No new signature or serialized contract requested; retain native resolver interfaces |
| scope.test_surface | existing | agents-plugin-tool/internal/wsagent/testdata/pi-lead.test.mjs; pi_lead_test.go#L296-L317; lead_isolation_test.go |
| complexity.reuse_points | confirmed | Read installResolver and bootstrap in agents-plugin-tool/internal/wsagent/pi_lead.mjs#L42-L87; native enabled-resource restoration and host-relative imports already exist |
| complexity.side_effect_risk | moderate | Resolver prototype wrapping changes extension restoration at every trust pass and reload (pi_lead.mjs#L49-L67) |
| risk.correctness | high | Fallback identity must preserve synthetic resources and filesystem alias deduplication without restoring disabled/ws extensions |
| risk.fit | moderate | Must preserve selected CLI/SDK graph and the missing-SDK-only isolated fallback (pi_lead.go#L63-L99) |
| risk.test | high | Native tests require WS_PI_TEST_SDK; regression fixtures reproduce both helper layouts but need exact-host acceptance. The npm-cached Pi 1.0.4 package is available in an isolated temporary extraction; its package-manager.js uses canonicalizePath and source-info.js lacks getResourcePathKey. |
| risk.security_or_contract | high | Default-mode restoration participates in native project trust and enabled-extension filtering; an incorrect fallback could load excluded code |

## Phases

### Phase 1: Restore default-mode startup across supported host helper layouts

Remove the bootstrap's assumption that every otherwise-compatible host
publishes the newer resource-path-key helper. Use the selected host's resource
identity behavior while retaining native package-manager restoration. Keep
existing newer-host behavior and reject genuinely incompatible resolver
interfaces rather than hiding their errors or silently selecting isolation.

Verification:

- Add regression fixtures for a host that exports the newer helper and one
  that lacks it but provides its native older path canonicalization. Require
  the original CLI to start in-process with unchanged argv/cwd and enabled
  ambient extensions restored.
- Verify symlink aliases deduplicate, disabled/ws resources stay excluded,
  metadata/order survive, and incompatible resolver interfaces still fail.
- Exercise the exact Pi 1.0.4 package when locally available, plus the current
  host. Run existing adapter and targeted Go launch/isolation tests. Report
  any exact-host/platform gap rather than treating synthetic fixtures as full
  native acceptance.
- Obtain independent review before integration. Record exact commands,
  outcomes, source commits and any remaining gaps in the Result.
