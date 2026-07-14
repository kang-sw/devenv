---
title: Windows dogfood gateway serves a stale frontend build; repoint its static-dir at the WSL build
related:
  260525-feat-ws-dashboard-endpoint-linked-server-add: the linked-server/gateway feature this dogfood setup exercises
  260707-chore-dashboard-linked-server-tunnel-dogfood-plan: sibling linked-server dogfooding plan
  260714-idea-dashboard-linked-server-localhost-ipv6-hang: the IPv4-literal link caveat below was surfaced by that finding
sage-review-design: required
---

# Windows dogfood gateway serves a stale frontend build; repoint its static-dir at the WSL build

## Background

For cross-OS dogfooding we run a Windows-native ws-dashboard daemon as a
frontend/gateway: a Windows browser loads its static frontend, and API calls
are forwarded through to the WSL-side daemon (linked server `wsl-daemon`,
upstream `http://127.0.0.1:8787`).

The problem: the Windows gateway serves its OWN `frontend/dist`, which is a
stale build (observed: a Jul-10 build from branch
`git-discovery-combined-rev-parse-hotfix`), so its UI drifts from the latest
WSL frontend build. Rebuilding the Windows frontend by hand on every UI change
is easy to forget, and the drift is confusing during dogfooding because the
two environments look different.

Deferred at filing time: the owner had live Windows-side work open and did not
want the gateway process restarted; this ticket parks the fix for a convenient
moment.

## Decisions

- **Approach to try (owner-chosen): a single frontend build.** Point the
  Windows gateway's `--static-dir` at the WSL `frontend/dist` over the
  `\\wsl.localhost\<distro>\...` UNC path instead of maintaining a separate
  Windows-side build. Then the WSL build is the single source of the served
  frontend, and no Windows-side frontend rebuild is needed for ordinary UI
  changes.
- **Known residual risk (owner-flagged): the binary is a separate axis.** The
  Windows gateway *binary* is independent of the served frontend. When the
  HTTP API contract changes, the old binary can no longer satisfy the latest
  frontend (for its own `server-local` routes and, importantly, for the
  forwarding routes it must understand to proxy to `wsl-daemon`), so the binary
  still needs an occasional rebuild. The static-dir approach only reduces the
  rebuild cadence from "every UI change" to "only on API-contract breaks" — it
  does not eliminate binary rebuilds. Worth tracking how often the
  frontend↔daemon API contract actually breaks.
- **Fallback:** if Windows cannot reliably serve static files over
  `\\wsl.localhost` (cross-mount serving unreliable/slow), keep a Windows-side
  build but add a durable refresh path (a small `refresh-frontend` script in
  the Windows checkout) plus a note in `_index.local.md`.

## Constraints

- Exactly ONE Windows gateway instance; never restart or disturb the WSL
  daemon on `:8787`.
- Linking must use the IPv4 literal `http://127.0.0.1:8787`. A bare `localhost`
  endpoint resolves to IPv6 `::1`, which WSL2 does not forward from Windows,
  and the daemon's outbound client hangs with no fast-fail
  (`260714-idea-dashboard-linked-server-localhost-ipv6-hang`).
- Machine-specific details — exact `D:\...` checkout paths, the resolved
  `\\wsl.localhost\<distro>` path, PIDs, ports, the WSL daemon's link
  passphrase source, and the exact relaunch command — are local machine
  context and belong in `ai-docs/_index.local.md` (gitignored), NOT in this
  committed ticket. The paths/values named above are examples for orientation
  only.

## Phases

### Phase 1: Repoint the gateway static-dir at the WSL build and probe API compatibility

- Rebuild the WSL `frontend/dist` if stale, resolve its Windows-visible
  `\\wsl.localhost\<distro>\...\frontend\dist` path, and confirm `index.html`
  is visible from Windows.
- Relaunch the single Windows gateway (same binary, same `--no-auth`, same
  port) detached, with `--static-dir` pointing at the WSL build; re-establish
  the `wsl-daemon` link using the IPv4 literal endpoint.
- **Make the relaunch double as an API-compatibility probe:** verify the
  latest frontend actually works against the current (old) Windows binary for
  BOTH `server-local` and the forwarded `wsl-daemon` — exercise
  `/api/dashboard/.../resources` and at least one git-status route, and record
  any 404/400/500 that indicates the frontend expects an API the old binary
  lacks. If the probe shows the contract already drifted, rebuild the Windows
  binary to the current lineage as part of this phase.
- Record the durable runbook (relaunch command, static-dir path, link
  passphrase source, the API-compat probe result) in `_index.local.md`.

Verification: the Windows browser at the gateway URL loads the latest UI
(matching the WSL build), and operating through the `wsl-daemon` server still
returns the real WSL workspace/worktree tree.
