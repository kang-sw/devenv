---
title: "P0: Shared daemon Claude plugin root depends on the launcher that starts it"
related:
  261113-feat-ws-hosted-lead-bundled-plugin-only: "preserve bundled-only isolation while correcting package resolution"
  261115-bug-ws-claude-binary-provenance-and-resume-model: "neighboring launcher-environment issue; not the same binary/model-selection defect"
---

# P0: Shared daemon Claude plugin root depends on the launcher that starts it

## Background

Dogfood finding #12, 2026-11-16, priority P0. The machine-wide daemon's ability
to launch Claude leads depends on which harness launcher won daemon startup.
This also affects downstream mixed-harness installations, not just this checkout.

At approximately 09:10 KST, a Pi launcher using the freshly installed v0.50.1
release runtime in `agents-plugin-pi/.runtime` replaced the older 0.50.0-dev
daemon under the newer-core-wins rule. Every subsequent hosted CLAUDE lead
relaunch failed. The owner's messages to an idle hosted Claude session remained
pending; Pi and Codex sessions were unaffected. Restarting through
`scripts/dev.py` restored Claude launches by pinning the wsflow resource root.
These runtime observations are owner-supplied evidence, not a new reproduction
performed during ticket capture.

Source evidence:

- `agents-plugin-pi/bin/ws-mcp-launcher.py:840-850`, `apply_rsrc_root_env`, sets
  `WS_RSRC_ROOT=<agents-plugin-pi>/rsrc` when unset. Startup calls it at
  `:1851` and `:1924`.
- `agents-plugin-pi/bin/ws-mcp-launcher.py:1378-1395`,
  `DaemonLink.start_daemon`, passes the inherited environment to `daemon-start`.
- `agents-plugin-tool/internal/wsagent/lead_plugins.go:16-31`,
  `bundledPluginRoot`, derives the plugin package from the parent of the resource
  root; `:36-46`, `validateBundledPlugin`, requires
  `<root>/.claude-plugin/plugin.json`. `agents-plugin-pi` has no such manifest.
- The incident's launch failures were logged at
  `agents-plugin-tool/internal/wsagent/runner.go:2004` (incident coordinates).
- `scripts/dev.py:51,199-201` chooses `agents-plugin-wsflow` and explicitly sets
  its resource/skills roots, explaining the observed recovery.

## Reproduction

1. In an isolated mixed-harness installation, start an older compatible shared
   daemon, with an idle hosted Claude lead and working Pi/Codex leads.
2. Start a Pi launcher with `WS_RSRC_ROOT` initially unset and a newer release
   runtime, allowing the normal replacement rule to select it.
3. Send a dashboard message to the idle Claude lead or relaunch it. Observe the
   missing `agents-plugin-pi/.claude-plugin/plugin.json` error and pending mail.
4. Compare Pi/Codex operation, then start the daemon with a valid ws/wsflow
   package root and confirm Claude recovery. Do not replace the owner's daemon
   merely to collect this reproduction.

## Candidate fix directions

- Resolve the bundled Claude plugin package independently of the resource root.
- Validate a Claude-capable daemon's package manifest at daemon-start/launcher
  admission instead of accepting a root that will fail every Claude launch.
- Keep the Pi launcher's resource selection from defining the shared daemon's
  Claude plugin root.

These are alternatives, not a selected startup or distribution policy. Preserve
bundled-only isolation; do not fall back to an arbitrary marketplace/project
plugin or require a repository sibling in downstream installs.

## Decisions

Compatibility target: **T0**, provisional for a correction that preserves
existing resource/config fields, protocol and installed layouts. The current
launcher writes an environment path and `bundledPluginRoot` reads it as a
package identity (source above); no shared-data recovery is implicated. If the
chosen correction introduces a manifest field, changed hello, or an old-client
startup refusal, classify that concrete writer/reader path before readiness;
an old-version boundary is T3 and needs owner approval and a minor bump.
No compatibility boundary is authorized by this capture.

## Open questions

- Which component owns the Claude package identity independently of Pi resources?
- Should invalid package roots be rejected at daemon admission or only when a
  Claude backend is requested, and how should pending mail report the failure?
- How is a valid bundled package located in each supported installed layout
  without silently substituting ws for wsflow?

## Phases

### Phase 1: Make cross-harness daemon replacement safe for Claude package resolution

Trace environment/package ownership through all launcher and installed-package
lanes, settle the open startup-policy questions, and implement the chosen
bounded correction. Preserve existing newer-core arbitration and unrelated
Pi/Codex behavior. Follow the applicable shipped-surface, launcher/MCP,
compatibility and mirroring manuals before implementation.

Verification boundary: isolated fixtures for Pi-started, ws-started and
wsflow-started daemons, unset versus explicit resource roots, source and
package-only layouts, missing manifests, cross-harness relaunch and pending
message recovery. Record the old/new reader and launcher matrix; account-backed
acceptance or live daemon replacement requires separate authorization. This
ticket capture runs no tests, builds or daemon operations.

