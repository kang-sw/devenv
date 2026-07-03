---
title: Branch-pinned Windows acceptance for the playbook-factory epic
sage-review: completed
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260622-chore-windows-shipping-hardening: predecessor; closed done — Phase A (static code hardening) and Phase B (launcher cold-load robustness) landed and were verified on Linux/WSL2 (build/test/vet + cross-compile, reviewed); this ticket is the split-out Phase C (branch-pinned Windows acceptance), which requires a real Windows host this session did not have
---

# Branch-pinned Windows acceptance for the playbook-factory epic

## Background

The epic `260605-epic-ws-playbook-factory-pivot` branch is a fast-forwardable
descendant of `main`. Linux is well dogfooded and mercenary is functionally
verified on Linux. The only remaining shipping surface is Windows.

Predecessor `260622-chore-windows-shipping-hardening` implemented and verified
(on Linux/WSL2, via `go build/test/vet` and cross-compile, plus the launcher's
Python unit-test suite) all the static-code and cold-load-robustness fixes the
Windows surface needs. Both phases (A: static code hardening; B: launcher
cold-load robustness) are done and reviewed. What remains — and could not be
executed in that session — is empirical proof on a real Windows host: this
ticket.

This ticket makes the epic mergeable by proving the Windows build and a live
mercenary round-trip on a real Windows host via a branch-pinned install.

## Decisions

- **mercenary-on-Windows is v1 shipping scope** (user decision, carried over
  from `260622-chore-windows-shipping-hardening`). The mercenary
  quoting/path/process fixes landed there are not deferrable.
- **Epic merge is deferred** until this ticket's branch-pinned acceptance
  passes. Static verification (Phase A/B in `260622-chore-windows-shipping-hardening`)
  already passed; this ticket is the remaining gate.
- **Branch-pinned verification strategy:** test the epic build on real Windows
  without exposing it to other consumers via
  `claude plugin marketplace add kang-sw/devenv@260605-epic-ws-playbook-factory-pivot`
  then `claude plugin install ws@kang-sw-devenv` (cold install → real Go build).
  The marketplace is this repo itself (`source: "./agents-plugin"`), so pinning the
  marketplace ref pins the plugin; marketplace.json needs no change.
- **Cache invalidation:** the AGENTS.md "version bump on dev-merge" rule
  (`bump-ws-version.sh`) is what makes the branch-pin loop pick up fresh builds —
  Claude Code keys cache on the `version` string. Without a bump, branch-pin
  reinstall serves stale cache.

## Constraints

- **Live-host safety (hard):** every Windows process-tree termination MUST stay
  PID/job-scoped (Toolhelp32 PPID walk or job object). Never terminate by image
  name (`taskkill /IM`) or any broad sweep — the dogfooding host runs a live
  `claude.exe`. This was already verified honored across the cancel paths in
  `260622-chore-windows-shipping-hardening` (Phase A item 6, PID-scoped
  `cancelAsyncProcessTree`); this ticket's live round-trip + cancel must confirm
  the same holds true in practice, not just in code.
- **Contract unchanged.** These are behavior-preserving conformance fixes to the
  existing `named-agent-runtime` behavior on Windows; no new caller-visible
  interface is introduced by this ticket. Spec text should need no change
  (confirm at closeout).

## Spec Impact

- **Target spec areas:** `named-agent-runtime` (mercenary process spawn, command
  quoting, file replacement, and process-liveness behavior) and `plugin-runtime`
  `## Windows Plugin-Managed Startup` (`#260505-windows-plugin-managed-startup`,
  launcher cold-load). Both specs were already confirmed unaffected by the
  `260622-chore-windows-shipping-hardening` Phase A/B implementation.
- **Expected caller-visible change:** none. This ticket only empirically
  confirms, on a real Windows host, that the Phase A/B fixes already landed in
  `260622-chore-windows-shipping-hardening` behave as specified. No new tool,
  schema, or semantic surface is introduced.
- **Contract-first spec: no.** These are behavior-preserving conformance fixes;
  the existing spec text describes the intended behavior already. Confirm at
  closeout that no spec wording drifted. If this ticket's live run surfaces a
  real contract nuance (e.g. an observable mercenary-on-Windows behavior not
  already covered), capture and address it as a spec update then, not before.

## Phases

### Phase 1: Branch-pinned Windows acceptance

Goal: prove the epic build on a real Windows host without exposing it to other
consumers, then clear the merge.

Steps:
1. Ensure a version bump landed on the branch (per the AGENTS.md rule) so the
   cache invalidates.
2. On Windows: `claude plugin marketplace add kang-sw/devenv@<epic-branch>` →
   `claude plugin install ws@kang-sw-devenv`. Confirm the MCP server boots (Python
   prerequisite assumed) and the Go binary builds cold.
3. Run a real mercenary call round-trip (claude and/or codex backend): register →
   call → result, plus one cancel, confirming PID-scoped termination does not reach
   the live `claude.exe`.
4. Fix anything that breaks; bump + reinstall; iterate until clean.
5. On green: the epic is cleared for merge to `main` + ship.

Verification: real Windows cold install + at least one mercenary round-trip and one
cancel, observed clean.

Depends on: `260622-chore-windows-shipping-hardening` Phase A and B landed and
version-bumped on the branch (already true — both phases have `### Result`
sections and are merged into the epic branch).
