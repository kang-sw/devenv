---
title: "playbook 자원을 daemon binary에 넣어 self-contained로 만들기"
---

# Embed the playbook resource tree in the ws-mcp binary

## Background

Playbooks, manifests and includes are read at call time from a filesystem
root, not from the binary (`agents-plugin-tool/internal/wsrsrc/wsrsrc.go:1-4`,
"NOT go:embed"). Each MCP connection passes its own `WS_RSRC_ROOT`
(`cmd/ws-mcp/proxy.go:158`), and the shared daemon uses only that root
(`internal/mcp/process_env.go:228-239`). Hosted leads use the plugin root the
daemon last registered from a hello (`internal/wsagent/hosted_plugins.go`,
`hosted-plugin-roots.json`).

So the guidance a session receives follows whichever plugin copy connected,
not the daemon build. On 2026-11-18 the owner's Mac ran a track/agent-v2
daemon while the Claude plugin copy (`install.sh` snapshot, plugin 0.50.0
from 2026-11-15) still served the pre-diet workflow manual and whiteboard
playbook (38 KB vs 9 KB in the repo). A daemon swap never changed that.

Owner, 2026-11-18 22:49: embed the playbook assets in the Go build, so the
daemon is fully self-contained.

## Decisions

- Owner direction 2026-11-18: the ws-mcp build embeds the playbook resource
  tree, and the daemon serves playbooks from its own build.
- (173), 2026-11-18 (announced, owner ack): skills (`SKILL.md` trees) stay in
  the plugin package; only the rsrc tree (playbooks, manifests, includes) is
  embedded. Claude Code and Codex load skills from the installed plugin
  directory, so the daemon cannot serve them.
- (174), 2026-11-18 (technical, owner ack): embed both the `ws` and `wsflow`
  rsrc trees and pick one by the connecting package's manifest name. wsflow
  ships its own mirrored rsrc with different defaults
  (`ai-docs/manuals/wsflow-mirroring.md`). Rejected: embedding `ws` alone,
  which would hand wsflow sessions ws text.
- (175)=a, 2026-11-18: the embedded tree always wins over a caller's
  `WS_RSRC_ROOT`. A separate, explicit dev-only override env (named at
  promotion) points at a source tree for playbook iteration without a
  rebuild. Rejected: `WS_RSRC_ROOT` winning when set (every plugin launcher
  sets it, so the embedded tree would go unused); the newer of the caller's
  plugin and the daemon winning.
- (176)=b, 2026-11-18: the work lands on `develop`, through the main session,
  which owns develop and the shipped rsrc. Rejected: carrying it on
  track/agent-v2.
- (177), 2026-11-18 (technical, owner ack): compatibility target T1 or
  better. The worker gives the verdict with cited read and write lines and
  stops above T2. The visible change is that an older plugin session
  connected to a newer daemon receives the daemon's playbook text; the worker
  checks that older skill shims still resolve every playbook name and
  variable they pass. Rejected: settling a tier before the source read.
- Related deferral: owner (171), 2026-11-18, holds refreshing the installed
  plugin copy until this ticket lands.

## Constraints

- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`, `ai-docs/manuals/performance.md`,
  `ai-docs/manuals/ws-mcp.md`.
- Project prompt overrides (`ws:lead-tune`) keep working unchanged.
- The local-devenv launcher fingerprint covers embedded assets already
  (`f2ea25fd5`); embedded rsrc must count toward it.
- The desktop build (`agents-plugin-tool/desktop-electron/scripts/assemble-daemon-input.cjs`)
  stages a full plugin package beside the binary today; settle at promotion
  whether it still stages rsrc.

## Phases

### Phase 1: Embed and serve the rsrc tree from the build

- Phase text (file layout, the dev override env name, the launcher and desktop staging changes, tests) is settled at promotion to `ready/` on develop.
