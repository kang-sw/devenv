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

## Earlier blocked verification (2026-11-14)

Exact Pi 1.0.4 native acceptance is **blocked on dependency closure**, not
established by the synthetic fixtures or current-host results. The isolated
cached tarball confirms the missing helper and native canonicalization source,
but both native test cases fail with `ERR_MODULE_NOT_FOUND` for `cross-spawn`
imported by `dist/utils/child-process.js`. The authorized offline preparation
attempt then failed with `ETARGET` because `@earendil-works/pi-client@^1.0.4`
is absent from the local cache. No further dependency preparation or exact-host
run is authorized until the root grants the shared heavy-work token and the
required preparation permission.

Evidence already obtained:

- Before the source fix, both added legacy-layout bootstrap fixtures failed
  with the recorded incompatible-extension-resolver error.
- `node --test internal/wsagent/testdata/pi-lead.test.mjs`: 9 passed, 0 failed,
  2 native cases skipped without a host override.
- With `WS_PI_TEST_SDK` selecting the installed Pi 1.5.2 SDK,
  `go test ./internal/wsagent -run 'TestPiLead|TestLeadIsolation|TestPi.*Isolation'
  -count=1 -timeout 4m -v` passed. Native checks include actual npm-bin startup,
  same-graph ambient loading, the wrong-SDK negative control, and native
  trust/filter/builtin/reload cases. This is not Pi 1.0.4 acceptance.
- No shared daemon, installed runtime, user configuration or model request was
  changed or invoked by this implementation. Independent source review and
  exact-host acceptance remain open; the ticket is not complete.

## Exact-host acceptance checkpoint (2026-11-14)

MAIN granted EXEC3 the shared heavy slot after EXEC1's quiescent seed handoff.
An independent owned temporary copy was verified with internal-only symlinks,
23,548 independently copied file hashes, lock/inventory agreement and SDK tar,
manifest and bundle identities. This supersedes the earlier missing-dependency
block, but **complete acceptance remains unestablished**.

- Source under test: `88ad299198d927103aa13f1e55086ae7da7ccc30`, unchanged at
  clean checkpoint HEAD `58f491a69bfde26dd9a761f2cb62f1e7e80715fc` before and
  after the run.
- Actual graph: coding-agent SDK **1.0.4**, AI/TUI and codemode **1.5.2** as
  resolved by published ranges. This is not an all-1.0.4 dependency graph.
- Exact SDK identities (SHA-256): tar
  `04910bdae661a6529e9d6869b04006f6aad01186398b04a16c6d9793667b96c5`;
  manifest `248decde30021109984def7ddc8f7bf4caf21ef04910a1303ab19604206c1bd5`;
  bundle `84b4506ad77165b118b3f2ec4ffaaf8309af560de967ff05d8af6a75c8457052`.
- From `agents-plugin-tool`, ran `node --test
  internal/wsagent/testdata/pi-lead.test.mjs` with `WS_PI_TEST_SDK` selecting
  the owned copy's `node_modules/@earendil-works/pi-coding-agent/dist/bundle/index.js`.
  Outcome: **14 passed, 1 failed, 0 skipped** (15 tests), exit 1, no timeout;
  supervisor elapsed 1.655 seconds. The process group was absent in repeated
  post-run checks.
- Positive bundled npm-bin startup restored `bundled`, `explicit`, `user`,
  `package` and `project`. Native saved/never/ask/extension trust, filtering,
  builtins and reload cases passed.
- The existing npm-bin test then failed in its **unbundled-SDK negative
  control**, `pi-lead.test.mjs:227`: Pi 1.0.4 `dist/index.js` imports
  `MCP_TYPESCRIPT_PREAMBLE` from `pi-codemode/declarations`, but resolved
  codemode 1.5.2 lacks that export. This is a concrete remaining acceptance
  gap, not a passing negative control or proof that the entire supported
  graph works.
- A duplicate Go adapter rerun was skipped because it invokes the same Node
  suite; no new Go acceptance is claimed. No dependencies or tests were
  weakened or changed. Independent implementation review remains open.

Full command, identity, stdout/stderr and quiescence artifacts are under
`/private/tmp/ws-exec3-pi104-ag2bcjay/`: `identity.json`,
`node.stdout.log`, `node.stderr.log`, `node-supervisor.json` and
`settlement.json`. The root read the complete Node output and the failing
source boundary. No shared daemon, installed runtime, live account or model
request was invoked; the seed was read/copied, never run or mutated.

