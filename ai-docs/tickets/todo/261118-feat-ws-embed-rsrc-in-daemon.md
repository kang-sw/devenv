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
- (176)=b, 2026-11-18, superseded by the owner at 23:14 the same day: the
  ticket now goes to track/agent-v2 and starts after that track's current
  work wraps up, because Phase 2 rewrites the hosted launch path that
  agent-v2 owns ("이건 지금 거기서 하는 일이 마무리된 후에 넣기 좋은 작업으로
  보여서요"). Rejected: landing it on develop through the main session.
- Scope widened, owner 2026-11-18 23:14: hosted sessions stop depending on an
  installed plugin copy. After the agent-v2 work, skills, hooks and
  `claude-agents` are expected to be deprecated, so the only plugin-copy
  inputs left are the launcher (`bin/ws-mcp-launcher.py`), the `native-pi`
  payload (`internal/wsagent/pi_payload.go:42`) and the package manifests
  (`config-manifest.json`, `dashboard-config-manifest.json`,
  `runtime.json`). The owner called the registry fallback
  (`hosted-plugin-roots.json` plus the same-minor check in
  `ResolveHostedPlugin`, `internal/wsagent/hosted_plugins.go`) "duct tape".
  The work runs in two phases. Phase 1 embeds the rsrc tree, the per-package
  manifests (both packages, as in (174)) and the `native-pi` payload.
  Phase 2 moves hosted sessions off the plugin copy and retires the registry
  and its same-minor fallback; how hosted MCP launches is settled by (181)
  below, which replaced the first wording ("start hosted sessions on the
  daemon's own executable"). Rejected: rsrc only; one combined phase.
- (177), 2026-11-18 (technical, owner ack): compatibility target T1 or
  better. The worker gives the verdict with cited read and write lines and
  stops above T2. The visible change is that an older plugin session
  connected to a newer daemon receives the daemon's playbook text; the worker
  checks that older skill shims still resolve every playbook name and
  variable they pass. Rejected: settling a tier before the source read.
- Owner (171), 2026-11-18, had deferred refreshing the installed plugin copy
  until this ticket lands. Superseded at 23:14 the same day: the owner ran
  `./install.sh` so the new playbooks reach sessions now.
- (182)=a, 2026-11-18 23:35: Phase 1 also embeds the remaining plugin-copy
  inputs a hosted lead reads today: package identity and MCP env from
  `.claude-plugin/plugin.json` (`internal/wsagent/lead_plugins.go:55-69`,
  `hosted_plugins.go:96-125`; Pi validates it at
  `agents-plugin-pi/src/workflow-owner.ts:31-36`), the Pi workflow skills
  (`agents-plugin-pi/src/skills-dir.ts:20-29`, roots at `pi_lead.go:58-60`)
  and the bootstrap template `skills/lead-bootstrap/AGENTS.template.md`
  (`internal/mcp/bootstrap_alarm.go:75-76`). Phase 2 then removes the
  owner-confirmed set: the registry writer (`cmd/ws-mcp/proxy.go:83`) and
  reader (`hosted_plugins.go:78`), `ResolveHostedPlugin`
  (`hosted_plugins.go:133`) with its same-minor admission (`:145-164`) and
  its callers (`cmd/ws-mcp/agent_mcp.go:21,79`, `lead_plugins.go:15`,
  `pi_lead.go:19,40,166`), and the plugin-copy launcher path in the hosted
  MCP command. Rejected: embedding only the scoped set and keeping the
  registry for the extra inputs until skills are deprecated.
- (183), 2026-11-18 (announced, owner ack): a text-only playbook edit reaches
  sessions only through a rebuilt daemon; `wsrsrc.go:12-14` "text-only edits
  must ship without a binary bump" no longer holds for production. The rsrc
  schema version stays separate from content.
- (184), 2026-11-18 (announced, owner ack): the embed is generated at build
  time from the canonical `agents-plugin/rsrc` and `agents-plugin-wsflow/rsrc`
  trees with a drift guard; no hand-maintained second copy. Basis: 260611
  retired `wsprompt` because its embedded copies drifted from rsrc ("the pain
  being solved is drift, not storage"; "rsrc is the single source").
- (181)=b, 2026-11-18 23:35: hosted stdio MCP keeps the Python launcher, but
  the binary embeds `ws-mcp-launcher.py` the way it embeds `holder.py`
  (`internal/wsholder/holder.go:26`, staged by hash) and runs it on the
  holder's Python, so no plugin copy is needed. The owner recalled the
  launcher was kept because it restarts a dead daemon ("데몬 죽었을 때 다시
  띄우는 것 때문에 python launcher를 유지하기로 결정했던 거 같은데요"); the
  2026-11-13 rule (`ce21c5bd5`: never `os.Executable()` as the attached ws
  MCP) stands. Rejected: (a) HTTP-only hosted MCP (port Pi leads and workers,
  drop Claude's stdio fallback); (c) the daemon's staged executable as the
  stdio server, because the session holder outlives a daemon replace and
  keeps every process it started (`cc32eacb1`), so that server would keep
  the old build's code running and pin its file, while the launcher is a
  thin proxy that rereads the endpoint file (`4e8add8d3`).
- (185), 2026-11-18 (technical, owner ack): the staged launcher restarts a
  dead daemon from the daemon executable the last running daemon recorded in
  its state directory, the newest staged and retained build
  (`internal/wsdashboard/spawn.go:63-95`). Rejected: the plugin runtime
  binary the launcher uses today, since a hosted session has no plugin copy.
- (186), 2026-11-18 (technical, owner ack): stage the launcher together with
  its package's embedded manifests (`runtime.json`, `config-manifest.json`,
  `dashboard-config-manifest.json`, `.claude-plugin/plugin.json`) in one
  hash-named package directory per package (ws, wsflow), so its
  package-relative reads stay unchanged and the same script still serves
  external plugin launches. Rejected: rewriting the launcher to take every
  input through env.

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
- Established at promotion (2026-11-18): the "never the daemon's staged
  executable" rule is (181)'s basis; `6873b4800` retired `wsprompt` for drift,
  which (184) answers. The registry removal set is owner-confirmed in (182).
- External Claude Code and Codex sessions keep connecting through their own
  plugin's launcher; this ticket does not change that path.

## Phases

### Phase 1: Embed and serve the rsrc tree from the build

- Phase text (file layout, the dev override env name, manifest and
  `native-pi` embedding, desktop staging changes, tests) is settled at
  promotion to `ready/` on track/agent-v2.

### Phase 2: Move hosted sessions off the plugin copy and retire the registry

- Phase text (the staged launcher and package directory per (181), (185) and
  (186), registry and same-minor fallback removal per (182), tests) is
  settled at promotion.
