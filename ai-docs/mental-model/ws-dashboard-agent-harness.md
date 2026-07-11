---
domain: ws-dashboard-agent-harness
description: "Dashboard-owned interactive agent-harness/provider integration: Codex app-server, OpenCode ACP, and Claude CLI headless stream-json duplex adapters, the cross-harness common interactive subset, session lifecycle/resume, and the Passthrough/Overlay/Hack/Unavailable capability tiering used to scope what the dashboard may build per (harness, capability) cell."
sources:
  - ws-dashboard/
related:
  ws-web-dashboard: "Shares the Server Route/workRoot/Activity Feed browser-facing identity model; this file owns the interactive-provider-specific rules layered on top of that read model."
  mcp-runtime: "Provider adapters must not become ws MCP session/root authority; the same non-authority boundary from ws-web-dashboard applies here."
  named-agent-runtime: "The legacy wsstate named-agent projection is a compatibility source, not the future authority for provider-adapter sessions."
---

# ws Dashboard Agent Harness Integration

## Domain Rules

- Any future interactive agent-harness provider/session work (Codex app-server,
  OpenCode ACP, Claude CLI headless stream-json duplex) must classify each
  capability per **(harness, capability) cell**, never per capability as a
  whole, into one of four tiers: **Passthrough** (the harness itself
  officially documents/exposes the capability for third-party programmatic
  use), **Overlay** (the dashboard composes only officially-exposed
  primitives into new behavior; no vendor-private state touched), **Hack**
  (the only reachable path mutates a harness's private/undocumented on-disk
  state or relies on reverse-engineered/unofficial protocol messages), or
  **Unavailable** (no known path yet; stays not-implemented rather than
  silently becoming a Hack). A capability native to only one harness stays
  Passthrough for that harness — cross-harness commonality (common subset vs.
  per-harness-gated) is a separate, orthogonal axis, never conflated with
  tier. Tiering as of 2026-07-11: session resume/create/send, permission
  interception (Claude `PreToolUse` hooks), and read-only context-usage
  display are Passthrough across all three. **Codex's column is
  fixture-verified** (2026-07-11, via `codex app-server generate-json-schema
  --out <dir> --experimental` against the installed `codex-cli 0.144.1`, not
  WebSearch-only): Codex additionally offers Passthrough manual compaction
  (`thread/compact/start`, result arrives async via `thread/compacted`), fork
  (`thread/fork`), skill listing (`skills/list`), mid-turn steering
  (`turn/steer`), and a native goal-state-tracking family
  (`thread/goal/set`/`get`/`clear` — bookkeeping only, not an auto-looping
  primitive) with no confirmed equivalents elsewhere. `thread/rollback`
  (rewind) is Passthrough but **confirmed deprecated for removal** and
  coarser than originally assumed (drops N turns from the end, not
  point-based, does not revert file changes) — do not design new
  functionality around it. OpenCode's column remains WebSearch-only/
  unverified (OpenCode not installed in this environment as of 2026-07-11).
  Claude's only reachable rewind/fork path is a Hack (transcript-file
  truncation, not officially supported) while Claude's compaction control
  stays Unavailable (auto-only; no workaround attempted). A dedicated 2026-07-11 research pass
  on whether headless `/compact` input text reproduces interactive-mode
  compaction found no direct confirmation either way, but official headless
  docs enumerate every other supported built-in slash command and stay
  silent on `/compact` — that enumeration gap is inference, not proof, so
  the cell stays Unavailable until a real fixture spike (send literal
  `/compact` via stream-json to an installed `claude` binary and check for a
  compaction/system event) settles it; subagent listing has no clean
  harness-native surface anywhere and should default to modeling a subagent
  as an ordinary nested Activity/session row rather than inventing a bespoke
  API; goal/loop (repeat-until-condition) has no harness-native primitive on
  any of the three and must stay dashboard-built Overlay, not assumed
  harness-native. New capabilities should default to stricter Hack/Unavailable
  scrutiny on the Claude column specifically, since Codex app-server was
  purpose-built as a third-party integration surface and OpenCode ACP is a
  purpose-built standard, while Claude's headless surface is documented but
  not purpose-fit for this exact use case. Full per-capability matrix and
  research trail: `260620-feat-ws-dashboard-agent-client-activity-sources`.
- Provider adapters are opinionated subsets, not full feature parity: each
  adapter (Codex app-server, OpenCode ACP, Claude CLI stream-json) implements
  only the slice of that harness's capability the dashboard's shared
  interaction contract actually needs.
- Session discovery and session control are separate mechanisms and must not
  be conflated: cross-harness history/session discovery comes from uniform
  vendor-history-file scraping (works without a live provider process; see
  `260624-feat-ws-dashboard-managed-cli-recent-sessions`), while resuming a
  selected entry dispatches through each vendor's own native live mechanism
  (Claude `--resume <session_id>`, Codex app-server thread resume, OpenCode
  ACP session resume/load).
- Claude CLI provider processes are killed and respawned rather than kept
  alive indefinitely: idle processes (no running turn, no active child shell
  subprocess) are killed after a timeout and transparently respawned via
  `claude --resume <session_id>` on next input. The browser must render
  "running" and "killed-but-resumable" as visually opaque/indistinguishable
  so resuming an old session reads as continuing a live chat, not restarting
  one.
- Execution-approval interception for the Claude provider uses CLI-level
  `PreToolUse` hooks (`.claude/settings.json`), not
  `--dangerously-skip-permissions`/`--permission-mode bypassPermissions`; the
  bypass flag is confirmed to hang with no human-response surface in
  TTY-less stream-json contexts and to refuse to start under root/sudo, so it
  may only back a deliberately-risky, human-toggled "dangerously bypass" opt-in
  mode, never the default per-prompt approval path.

## Coupling

- The frontend interaction-API surface (`activity.history.list`,
  `activity.session.start/create/send`, `activity.session.usage`, and the
  per-harness-gated `activity.session.compact/rewind/fork/skills`) couples
  directly to the tiering above: only Passthrough/Overlay cells may back a
  shipped API method in the normal adapter phases; Hack cells require a
  separate ticket with explicit experimental UI labeling and owner risk
  sign-off before they can back any method at all.
- This file's tiering is the shared classification surface for `ws-web-dashboard`'s
  WorkRoot Activity read model once provider adapters start feeding
  `items`/transcripts: adapter work must not treat SQLite/wsstate named-agent
  records as provider authority for these newer sources.

## Extension Points & Change Recipes

- **Add a new interactive-provider capability**: classify it per (harness,
  capability) cell using the four-tier system above before writing any
  adapter code; default new Claude-column capabilities to Unavailable absent
  direct evidence, and record the classification and its research trail
  (or "pending fixture verification" if unverified) in this file's Domain
  Rules bullet rather than only in the originating ticket.
- **Promote a Hack-tier capability toward shipping**: do not fold it into the
  normal adapter phases; open a dedicated ticket with explicit
  experimental/unsupported UI labeling and owner risk sign-off, mirroring the
  "dangerously bypass" opt-in pattern.
