---
title: Dev-machine-only ws-mcp source build for the Pi adapter, injected through the launcher's existing bootstrap seam
related:
  260802-research-ws-pi-native-framework: direction anchor; AGENTS.md clause (3) pins the Pi bundle's ws-mcp to the develop root worktree, which this marker builds from
  260903-research-ws-pi-adapter-npm-distribution: packaging; the marker and build output are gitignored and outside the `files` whitelist so a published tarball never carries or honors them
  260906-bug-ws-pi-rsrc-mirror-drift: the launcher stays a byte-identical mirror; this ticket deliberately adds nothing to it
spec:
  - 260903-pi-bridge-version-pin
  - 260903-pi-bridge-subprocess-lifecycle
  - 260903-pi-adapter-package-topology
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 35758d31cc5525b6
sage-review-completeness-reviewed: 35758d31cc5525b6
---

# Dev-machine-only ws-mcp source build for the Pi adapter, injected through the launcher's existing bootstrap seam

## Background

The Pi adapter is loaded straight from this worktree's source tree (the
owner's Pi `settings.json` lists `agents-plugin-pi` as a package path), but
the ws-mcp it runs comes from `agents-plugin-pi/.runtime/<platform>/`, which
the byte-identical launcher fills from the GitHub release named by
`runtime.json`. On the owner's development machine that is the wrong
binary: the Pi track needs develop-side ws-mcp changes that are merged but
not yet released. The manual's current workaround (`ws-mcp.md`, "Pi adapter
dogfood") is a hand-run `go build` plus a one-shot
`WS_MCP_BOOTSTRAP_BINARY` launch; on 2026-09-07 that produced a
`ws-mcp-0.45.2-<hash>` cache entry whose content was a stale 0.44.4
Pi-track build, every launch failed with "incompatible ws-mcp runtime after
repair", and the failure was only diagnosed by hashing the cache files.

The launcher's own local-devenv path (`.local-devenv-runtime` marker with
fingerprint-keyed rebuilds) is gated to plugin-cache install paths under
`.claude`/`.codex`, so it never fires for `agents-plugin-pi`, even though
`.gitignore` already lists `agents-plugin-pi/.local-devenv-runtime`.

## Decisions

- **Adapter-side, launcher untouched.** The bridge, not the launcher, owns
  the dev bypass. The launcher already honors `WS_MCP_BOOTSTRAP_BINARY`
  (clear the compatibility stamp, copy the binary in, verify the exact
  version and the capabilities superset, stamp, exec); the bridge builds the
  binary and passes that one variable to the launcher child. The launcher
  and `version-check.ts` are not edited, so the byte-identity test and the
  exact `plugin_version` pin stay as they are. Rejected: widening the
  launcher's `local_devenv_cache_package` gate to recognize
  `agents-plugin-pi` plus relaxing the bridge pin to accept a `-dev` suffix.
  That is two files, one of them develop-authored shared code reaching the
  Pi track only by cherry-pick, for a dev-only convenience.
- **Marker file and schema.** Opt-in is the presence of
  `agents-plugin-pi/.local-devenv-runtime` (already gitignored) with the
  launcher's marker schema, so a developer who knows one marker knows both:
  `{"schema_version": 1, "source_root": <abs>, "tool_dir": <abs>, "go": <abs>}`.
  The launcher ignores this file in this package because of its path gate;
  no double handling. Missing file: the bypass is inert and no other
  behavior changes. Present but invalid (bad JSON, wrong schema, relative
  path, missing `cmd/ws-mcp` under `tool_dir`, non-executable `go`): fail
  loud at session start with the reason, the same posture as the version
  pin. A marker is an explicit developer opt-in; silently ignoring a broken
  one is how the stale-binary confusion above happens.
- **Build stamp.** `go build -ldflags "-X main.version=<runtime.json
  plugin_version> -X main.sourceCommit=<short HEAD of source_root>" -o
  <pluginDir>/.runtime/local-devenv/ws-mcp ./cmd/ws-mcp`, run with
  `cwd = tool_dir`. Stamping the exact `plugin_version` is what lets the
  unchanged bridge pin pass; stamping the source commit is what makes
  `runtime info` answer "which develop commit is this" next time. The
  policy intent (AGENTS.md clause 3: the Pi bundle's ws-mcp is the develop
  root worktree's) is preserved by convention, not enforced: the marker's
  `source_root` is whatever the developer writes.
- **Build every lead session start, no fingerprint cache.** Go's build
  cache makes an unchanged rebuild sub-second (0.36 s measured on the
  owner's machine); a fingerprint scheme would duplicate the launcher's for
  no gain. Build failure fails loud (session load stops with the go
  output), never falls back to whatever the cache holds.
  The launcher's own fingerprint cache exists because a cold-cache build
  "blew past the MCP startup timeout" of the hosts that spawn it; that
  budget does not apply here. The build runs synchronously inside
  `session_start` before the launcher is spawned, Pi's extension runner
  awaits each handler with no time bound (`runner.js` `emit` is a plain
  `await handler(event, ctx)` loop), and the bridge's `initialize` has no
  timeout either. A cold build therefore only delays session start, once
  per machine or toolchain change. So the developer can tell a cold build
  from a hang, the bridge notifies "building ws-mcp from `<source_root>`
  @`<commit>`" before invoking go and reports the elapsed time after.
- **Atomic output.** The build writes to a per-process temp path in
  `.runtime/local-devenv/` (`ws-mcp.<pid>.tmp`) and is renamed to
  `.runtime/local-devenv/ws-mcp` on success, so two lead sessions starting
  at once never hand the launcher a torn file; each renames its own complete
  build and the launcher copies whichever complete file is current. The
  launcher's local-devenv path uses the same temp-and-replace shape.
- **Marker context on any launch failure.** While the marker is active,
  a launcher or `initialize` failure is re-thrown with the marker context
  prefixed: source root, short commit, and the built path. The one failure
  the stamp makes reachable is a clean build whose tool set no longer
  satisfies the pinned `runtime.json` contract (a renamed or dropped tool
  on the source branch); the launcher reports that only as its generic
  "incompatible ws-mcp runtime after repair", and the adapter is the only
  place that can say the marker put that binary there.
- **Lead-only.** Only a process whose spawn role is lead (or fork, which
  shares the lead's bridge path) builds and injects. Worker/explore
  children skip the module entirely; their launcher finds the compatibility
  stamp the lead's launch wrote and reuses the installed binary. This keeps
  child spawn latency unchanged and avoids N concurrent `go build`s.
- **Environment scoping.** The variable is passed only on the launcher
  child's `spawn` env (`{...process.env, WS_MCP_BOOTSTRAP_BINARY}`), never
  written to `process.env`, so it cannot leak into Pi child processes the
  spawner creates. When there is nothing to inject, the `env` option is
  omitted so the spawn call is byte-for-byte what it is today.
- **Packaging guardrail (owner, 2026-09-07).** The feature must be invisible
  to a published package: the marker is looked up relative to the adapter's
  own package directory, so an npm-installed copy has no marker and takes
  no new code path; neither `.local-devenv-runtime` nor
  `.runtime/local-devenv/` is in `package.json` `files`; nothing is added to
  the pack scripts; and the `ws-mcp.md` dogfood recipe is rewritten to point
  at the marker instead of the hand-run bootstrap.

## Constraints

- Pi-extension code only (`agents-plugin-pi/src`, `test`, and the manual
  section). No `bin/ws-mcp-launcher.py`, `runtime.json`, `rsrc/`,
  `version-check.ts`, or ws-mcp Go change.
- Build execution is an injected function so the suite runs without Go.
- The marker must not be created by any code path; the developer writes it.

## Spec Impact

- `{#260903-pi-bridge-version-pin}`: add that on a developer machine the
  marker path satisfies the same pin by stamping the source build with the
  bundled `plugin_version`; the pin itself, its exact match, and its
  fail-loud posture are unchanged.
- `{#260903-pi-adapter-package-topology}`: add the two package-local,
  gitignored, never-packed dev-only artifacts (`.local-devenv-runtime`
  marker, `.runtime/local-devenv/`) and that the adapter, not the launcher,
  owns the dev bypass because the launcher's local-devenv gate excludes this
  package.
- `{#260903-pi-bridge-subprocess-lifecycle}`: no contract change; a build
  or marker failure is one more spawn-time fail-loud case, and the marker
  context prefix is a message-shape addition.

## Phases

### Phase 1: Local-devenv build-and-inject module on the bridge startup path

Add `src/local-devenv.ts`: read and validate the marker relative to
`pluginDir`; compose the `go build` argv and ldflags from `runtime.json`'s
`plugin_version` and the `source_root` short HEAD; run the injected build
function; return the env fragment or `undefined` when no marker exists. Add
an optional `env` to `McpStdioClient`'s options and `spawnWsMcpClient`,
applied only when provided. In `startBridge`, call the module for lead/fork
roles before spawning the launcher and pass the result through. Rewrite the
"Pi adapter dogfood" subsection of `ai-docs/manuals/ws-mcp.md` to describe
the marker and keep the release-download path as the default.

Tests (node --test, no Go): no marker returns `undefined` and the spawn
options carry no `env` key; each invalid-marker case throws naming the
field; a valid marker yields the exact ldflags string, a temp output path
under `.runtime/local-devenv/`, and calls the injected build once with
`cwd = tool_dir`, then the final path is the renamed `ws-mcp`; a build
failure propagates; a launch failure while the marker is active carries
the source root, commit, and built path in its message; worker and explore
roles never consult the marker; `spawnWsMcpClient` forwards `env` when
given. The existing
byte-identity and version-pin tests must pass unchanged.

Verification (owner, live): write the marker for `/Users/kang-sw/devenv`,
delete the stale `.runtime/darwin-arm64/ws-mcp-0.45.2-*` entry, start a Pi
lead session, and confirm from the launcher's stderr that it installed the
bootstrap binary and from `runtime info` on the installed file that
`version` is `0.45.2` and `source_commit` is the develop root HEAD; spawn
one worker and confirm no second build ran; remove the marker and confirm
the next session takes the ordinary path. `npm pack --dry-run` must list
neither the marker nor `.runtime/`.
