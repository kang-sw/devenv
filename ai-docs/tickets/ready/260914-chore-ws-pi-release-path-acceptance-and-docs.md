---
title: Verify agents-plugin-pi release-path acceptance and document git-install
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260914-chore-ws-pi-join-release-train: prerequisite
  260903-research-ws-pi-adapter-npm-distribution: source of the transferred release-path acceptance and the git-install decision
  260907-feat-ws-pi-local-devenv-ws-mcp-build-bootstrap: originated the marker-removal acceptance transferred here
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: cd7c575097c7ae96
sage-review-completeness-reviewed: cd7c575097c7ae96
---

# Verify agents-plugin-pi release-path acceptance and document git-install

## Background

`260903` transferred an **unverified** acceptance from `260907`: the adapter's
ordinary launcher release path — downloading the ws-mcp release binary rather
than a developer build — has never been confirmed in a real Pi session after
removing the developer-only `.local-devenv-runtime` marker. Historical no-env
launcher reuse is not the same check.

Separately, there is no consumer-facing documentation for installing the adapter
through Pi's git package manager, even though the consumption path is decided.

## Decisions

- **Consumption path = `pi install git:github.com/kang-sw/devenv@<tag>`.** The
  repo-root manifest loads the subdir extension; the launcher downloads the
  tag's ws-mcp release binary. npm distribution stays secondary/deferred
  (`260903`). The `<host>` segment is REQUIRED: Pi's git spec is
  `git:<host>/<user>/<repo>` and a host-less `git:kang-sw/devenv` mis-parses
  `kang-sw` as the host (`Could not resolve host: kang-sw`), observed during
  the v0.46.4 owner-gate acceptance on a fresh Windows clone.
- **Acceptance procedure.** Remove `agents-plugin-pi/.local-devenv-runtime`,
  start a fresh Pi session, and confirm the ordinary launcher path downloads and
  SHA256-verifies the ws-mcp release binary for the current release tag and the
  adapter registers its `ws/*` tools (i.e. `assertVersionPin` passes against the
  release binary's reported `serverInfo.version`). Record the observed
  runtime/version and launch behavior as evidence.

## Constraints

- No package-metadata change, no npm publish, and no runtime reimplementation is
  authorized by the transferred acceptance (`260903` transfer bounds).
- The `.local-devenv-runtime` marker is a gitignored developer artifact; never
  commit it, and preserve the default release path.
- Full end-to-end verification needs a real published tag whose pi mirror is
  non-drifted — hence the `260914-chore-ws-pi-join-release-train` prerequisite;
  cutting that tag is a separate ship action (`ws:lead-ship`).
- `ai-docs/manuals/shipped-surface-boundary.md` applies: consumer-facing install
  text is downstream-facing.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | single-file | Phase 1 makes no code or doc changes (live verification only); Phase 2 edits one consumer doc file, README.md's install section |
| scope.surface | internal | no exported code symbol changes; Constraints forbid runtime reimplementation or package-metadata change |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | agents-plugin-pi/test/version-check.test.ts, agents-plugin-pi/test/local-devenv.test.ts already cover assertVersionPin and the local-devenv marker path; Phase 1's own verification is a live, human-run Pi session, not an added automated test |
| complexity.reuse_points | not-applicable | no code authored or reused; ticket is live verification plus consumer documentation |
| complexity.side_effect_risk | low | no code path is touched; Constraints explicitly disallow runtime or package-metadata changes |
| risk.correctness | low | no code changes; Phase 1 is self-gating (a failed release path halts before Phase 2's docs are written) |
| risk.fit | low | consumption path and doc-placement precedent already confirmed: root package.json declares pi.extensions ["agents-plugin-pi/src/index.ts"], origin remote is kang-sw/devenv, runtime.json release_repository/release_tag match, and consumer-facing install guidance already exists in README.md and ai-docs/manuals/codex-integration.md |
| risk.test | moderate | acceptance is a manual, human-observed live Pi session with no automated regression guard for the release path |
| risk.security_or_contract | low | no change to SHA256 verification, the version-pin contract, or package metadata; Constraints forbid all three |

## Phases

### Phase 1: Release-path acceptance verification

Perform the acceptance procedure above against the current release tag —
resolved from `agents-plugin-pi/runtime.json` `release_tag` (currently
`v0.46.4`) — and record the evidence (observed runtime/version, download +
checksum behavior, tool registration). The implementing agent runs the live Pi
session and observes it directly; where a real Pi TUI session cannot be driven
non-interactively, hand the run to the user and record their observed evidence.
If the release path fails, capture the failure as the result and stop before
writing docs.

Verification: a real Pi session with the marker absent registers `ws/*` tools
from a downloaded release binary; evidence recorded in the Result.

### Phase 2: Consumer git-install documentation

Document the `pi install git:github.com/kang-sw/devenv@<tag>` install and enablement flow
for downstream consumers, reflecting the behavior observed in Phase 1. Add it to
`README.md`'s install section, where general ws-plugin install text already
lives, so a consumer finds it beside the existing install guidance; do not bury
it in project-internal memory.

Verification: the documented command and flow match the Phase 1 evidence; a
reader can install and enable the adapter from the doc alone.

## Blocked (2026-09-16)

No agent-advanceable work remains; this ticket is gated on owner action.

- **Phase 1 is owner-run live verification.** The acceptance is a real Pi TUI
  session on a **clean machine** ("hand the run to the user and record their
  observed evidence"). This dev machine is not clean — `agents-plugin-pi/node_modules`
  is already populated by dogfooding, the exact false-positive condition
  `260914-chore-ws-pi-root-manifest-runtime-deps` documented — so running the
  acceptance here would be an invalid test.
- **A re-release carrying the deps fix is required first.** The git-install
  consumption fix (root-manifest runtime deps + pi-web-access resolution) is
  merged to `develop` but not yet re-released; the current `runtime.json`
  `release_tag` is `v0.46.4`, which predates the fix. Per this ticket's
  Constraints, full e2e verification needs a real published tag whose pi mirror
  is non-drifted — a separate `ws:lead-ship` action, owner-gated.
- **Phase 2 (docs) is gated on Phase 1 evidence** and cannot be written before
  the acceptance run produces observed behavior to document.

Unblocks when the owner cuts the re-release and records the clean-machine Pi
acceptance evidence. `ws:lead-run` cannot advance this ticket further.
