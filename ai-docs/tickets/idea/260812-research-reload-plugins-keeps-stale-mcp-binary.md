---
title: /reload-plugins reconnects to the stale ws MCP process; a freshly reinstalled version stays live-invisible until /mcp reconnect
related:
  260703-bug-claude-plugin-cache-stuck-below-source-version-mcp-refuses-start: adjacent install/cache-divergence class (known limitation); that case hard-fails MCP start, this one silently serves a stale-but-running binary
  260627-bug-playbook-render-uses-stale-plugin-cache-during-source-dogfood: adjacent stale-cache dogfood surprise on the render path
---

# /reload-plugins reconnects to the stale ws MCP process; a freshly reinstalled version stays live-invisible until /mcp reconnect

## Background

Dogfood surprise on 2026-08-12. After merging the note repo-layer + visibility
work to `main` and running `install.sh update` (which re-synced the installed
snapshot to `0.40.2` and reinstalled the plugin), the running session's ws MCP
tool surface still exposed the **old** contract: no `note.mute` / `note.unmute`,
and `note.write`'s `layer` enum lacked `repo`. `/reload-plugins` reported
reloading the plugin and its MCP server, yet `ws/runtime_info` still returned
`version: 0.40.0-dev` — the binary the session started with.

Only a subsequent `/mcp` reconnect brought the freshly-built binary live:
`ws/runtime_info` then returned `version: 0.40.2-dev`, `note.mute` / `note.unmute`
appeared, and the `repo` layer enum was present. A write→mute→unmute→erase smoke
test then passed end-to-end.

So the observed behavior: `/reload-plugins` reloads plugin manifests/agents but
**reconnects to the already-running on-demand MCP subprocess rather than
restarting it**, so a version reinstalled mid-session stays invisible until the
MCP transport itself is torn down and reconnected. The devenv launcher builds the
ws binary on demand from source (`.local-devenv-runtime`), which sharpens this:
the on-disk snapshot and source were already correct, only the *live process* was
stale.

## Severity / framing

Low. This is dogfood friction, not a correctness defect: the reinstall itself
worked, and `/mcp` reconnect is a cheap, discoverable recovery. It differs from
`260703` (its adjacent class) — there the version skew hard-fails MCP startup;
here the server runs fine, just one build behind. No sharp fix is obviously worth
building; captured as a data point plus one research angle, not as pressure to
prioritize.

## Research direction — does a minor-version change get detected where a patch does not?

Open question raised in discussion (2026-08-12): the case that slipped through was
a **patch/dev-level** advance within one minor (`0.40.0-dev` → `0.40.2-dev`).
Claude Code keys plugin-cache invalidation on the plugin `version` string, and the
runtime compatibility gate (`version_compatible`, see
`spec/plugin-runtime.md {#260506-launcher-hot-path-compatibility-cache}`) tolerates
patch drift within a minor. It is plausible the harness would *notice* and force a
real MCP restart when the **minor** version changes (e.g. `0.40.x` → `0.41.0`),
because that crosses the `required_mcp` window and the cache key more decisively,
while a same-minor rebuild is silently reconnected as "compatible."

To investigate:

- Confirm whether `/reload-plugins` vs `/mcp` differ by design in whether they
  restart the launcher subprocess, or whether reload is expected to restart it and
  did not here.
- Test the minor-bump hypothesis: reinstall across a minor boundary and observe
  whether `/reload-plugins` (or session-start staleness detection) then forces a
  fresh binary where a patch bump did not.
- If minor bumps *are* reliably detected, weigh whether the devenv dogfood loop
  should bump a minor (not just a patch) when a live-surface change must be picked
  up mid-session — versus simply documenting "`/mcp` reconnect after
  `install.sh update`" as the one-line recovery in the MCP runbook.

## Suggested minimal outcome

If not investigated further, the cheapest capture-of-value is a one-line note in
`ai-docs/manuals/ws-mcp.md`: after `install.sh update` re-syncs a new version
mid-session, run `/mcp` (reconnect), not just `/reload-plugins`, to pick up the
rebuilt binary. Deprioritized in `idea/`; not requesting promotion.
