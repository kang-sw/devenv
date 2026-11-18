---
title: "playbook 자원을 daemon binary에 넣어 self-contained로 만들기"
completed: 2026-11-19
related:
  261116-feat-ws-daemon-plugin-package-select: "its setting-driven hosted package choice (wsconfig.HostedPluginPackageKey) stays; Phase 2 removes its registry fallback"
  261117-feat-ws-hosted-prompt-assembly: "hosted Claude sessions already launch without the ws/wsflow plugins; Phase 2 extends that to the hosted MCP launcher"
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: e42cc70ba093d506
sage-review-design-reviewed: e42cc70ba093d506
---

# Embed the playbook resource tree in the ws-mcp binary

## Background

Playbooks, manifests and includes are read at call time from a filesystem
root, not from the binary (`agents-plugin-tool/internal/wsrsrc/wsrsrc.go:1-4`,
"NOT go:embed"). Each MCP connection passes its own `WS_RSRC_ROOT`
(`agents-plugin-tool/cmd/ws-mcp/proxy.go#L83-L93` requires it), and the shared daemon uses only that root
(`internal/mcp/process_env.go:228-239`). Hosted launches prefer the starter's
own root when its manifest matches the configured package; otherwise they
try a same-minor root registered from a hello, then fall back to the starter
with a warning (`agents-plugin-tool/internal/wsagent/hosted_plugins.go#L133-L191`,
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
  ships its own byte-identical mirrored rsrc; package config defaults differ,
  and namespace substitution happens at render time
  (`ai-docs/manuals/wsflow-mirroring.md`,
  `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go#L47-L54`). Rejected: embedding `ws` alone,
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
  (new record required: `agents-plugin-tool/internal/wsdashboard/spawn.go#L51-L95`
  currently stages by content hash and retains settled builds, but neither
  selects a newest build nor writes a last-running-executable record;
  `DaemonExecutable` stages only on Windows). Rejected: the plugin runtime
  binary the launcher uses today, since a hosted session has no plugin copy.
- (186), 2026-11-18 (technical, owner ack): stage the launcher together with
  its package's embedded manifests (`runtime.json`, `config-manifest.json`,
  `dashboard-config-manifest.json`, `.claude-plugin/plugin.json`) in one
  hash-named package directory per package (ws, wsflow), so its
  package-relative reads stay unchanged and the same script still serves
  external plugin launches. Rejected: rewriting the launcher to take every
  input through env.

- (187), 2026-11-18 (technical, owner ok): the (175) dev override is one
  daemon-process env var, `WS_RSRC_DEV_CHECKOUT`, holding a repository
  checkout path; the daemon reads `<checkout>/agents-plugin/` or
  `<checkout>/agents-plugin-wsflow/` for every embedded input. It is never
  read from a connection's hello, so one connection cannot redirect another
  session's text. Rejected: per-package root vars.
- (188), 2026-11-18 (technical, owner ok): the generated embed set is
  gitignored inside the Go module (`go:embed` cannot reach `../agents-plugin*`
  from `agents-plugin-tool/go.mod`), written by one generator that every build
  entry runs first. A committed placeholder lets a bare `go build` compile;
  such a daemon fails loud at the first playbook read unless (187)'s override
  is set, and tests set the override to the checkout so a bare `go test`
  still works. Rejected: committing the generated copy, which doubles every
  playbook diff and adds a tracked copy of the roughly 10 MB `native-pi`
  payload.

- Phase 1 implementation verdict, 2026-11-19: **T1** (display/content change
  only for existing same-minor callers); no protocol, tool/argument, resource
  schema or authoritative-store format change. The directional reader/writer
  and older-shim evidence is recorded in Phase 1's Result.

- Phase 2 implementation decisions, 2026-11-19: stages live at
  `<state dir>/hosted-packages/<package>/<sha256>/`; corrupt settled stages
  fail without overwrite. Production warm staging uses memory only; the
  explicit dev checkout rehashes its five inputs at each launch so live env
  and staged manifests cannot disagree.
- Phase 2 restart record, 2026-11-19: owner-only
  `last-running-executable.json`, schema 1, holds the retained absolute
  executable path, version, SHA-256 and build time. Only the singleton owner
  publishes it at startup. The internal `WS_MCP_HOSTED_PACKAGE=1` marker makes
  hosted launchers follow healthy daemons and reread that record for a needed
  restart; no plugin runtime repair/download or mtime-based newest selection.
  External plugin launchers keep their own runtime selection.
- Phase 2 compatibility verdict, 2026-11-19: **T0** for the additive restart
  record, disposable staging and ignored registry; the whole ticket remains
  **T1** due to Phase 1's served-content change. Directional source evidence
  and the older-daemon-after-newer-record scenario are in Phase 2's Result.
- Phase 2 scope check, 2026-11-19: external skill discovery and external
  connection manifest loading remain outside the phase's hosted-launch and
  registry removal set. Hosted manifests now read staged build-owned files;
  external plugin skill/manifest paths are not changed in this run.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/performance.md (declared for `agents-plugin-tool/`)
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`, `ai-docs/manuals/performance.md`,
  `ai-docs/manuals/ws-mcp.md`.
- Project prompt overrides (`ws:lead-tune`) keep working unchanged.
- The local-devenv launcher fingerprint covers embedded assets under the Go
  tool directory already (`f2ea25fd5`), not the canonical sibling plugin trees;
  embedded rsrc must count toward it, including canonical edits before generation
  (`agents-plugin/bin/ws-mcp-launcher.py#L566-L613`).
- The desktop build (`agents-plugin-tool/desktop-electron/scripts/assemble-daemon-input.cjs`)
  stages a full plugin package beside the binary today; settle at promotion
  whether it still stages rsrc.
- Established at promotion (2026-11-18): the "never the daemon's staged
  executable" rule is (181)'s basis; `6873b4800` retired `wsprompt` for drift,
  which (184) answers. The registry removal set is owner-confirmed in (182).
- External Claude Code and Codex sessions keep connecting through their own
  plugin's launcher; this ticket does not change that path.

## Prior Decisions

- 260609-feat-ws-playbook-surface-mvp (2026-06-09, Decisions): "Text-only changes must ship without a binary version bump." — bearing: constrains
- 260611-refactor-ws-tier-taxonomy-delegate-tier-routing (2026-06-12, Decisions): "The pain being solved is drift, not storage" — bearing: supports
- 261116-feat-ws-daemon-plugin-package-select (2026-11-16, Decisions (1211)): "Running and holder-reattached processes keep their package, and no daemon restart is needed." — bearing: constrains
- 261117-feat-ws-hosted-prompt-assembly (2026-11-18, Decisions 12): "Hosted Claude sessions launch without the ws/wsflow plugins" — bearing: supports
- ce21c5bd5 (2026-11-13, commit): "every attached ws MCP launch goes through the Python plugin launcher" — bearing: constrains
- f2ea25fd5 (2026-11-13, commit): "walk everything under tool_dir minus known build-irrelevant paths" — bearing: constrains
- cc32eacb1 (2026-11-18, commit): "running daemon-managed Pi turns intentionally survive replacement" — bearing: constrains
- 4e8add8d3 (2026-11-17, commit): "the stdio launcher survives by rereading the endpoint file" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsrsrc/, internal/mcp/, internal/wsagent/, cmd/ws-mcp/; both plugin packages, build scripts and desktop assembly |
| scope.surface | cross-module | wsrsrc loader APIs feed mcp and wsdoc; hosted launch spans wsagent, cmd/ws-mcp, Python launcher and Pi workflow-owner.ts |
| scope.new_public_symbol | unknown | fs.FS loader and embedded package/staging API symbols are not named yet |
| scope.new_type_contract | yes | Phase 1 changes filesystem-root loader inputs to fs.FS; Phase 2 adds a persistent last-running-executable record, whose shape is not specified |
| scope.test_surface | existing | agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go, internal/wsagent/hosted_plugins_test.go, cmd/ws-mcp/agent_mcp_test.go; launcher tests under agents-plugin/tests/; new scenarios required, no new test filename specified |
| complexity.reuse_points | confirmed | wsrsrc.Load/ReadManifest preserve schema/hash/variant checks; wsholder.StageScript stages by hash; wsdashboard.StageExecutable retains builds; HostedPluginEnvironment reads package MCP env |
| complexity.side_effect_risk | high | shared daemon resource authority changes for all connections; hosted launches and crash restart stop using installed roots |
| risk.correctness | high | two package selections, build generation, hash staging, daemon replacement and older shims must agree |
| risk.fit | moderate | (183)-(184) explicitly supersede text/binary decoupling while preserving canonical source and drift guards; (187)-(188) settle the override and generation |
| risk.test | high | no-plugin startup, killed-daemon restart and replacement require lifecycle integration across Claude/Pi and platform-specific executable handling, beyond existing mirror/unit tests |
| risk.security_or_contract | high | connection-owned resource roots cease to be authoritative; daemon-only dev override and persisted executable restart path define new trust and cross-version boundaries |

## Phases

### Phase 1: Embed and serve the rsrc tree from the build

Goal: every playbook, manifest and include a session receives comes from the
daemon's own build, for both packages; a connection's `WS_RSRC_ROOT` no longer
changes the text it is served.

- Embedded package set, one per package (`ws` from `agents-plugin/`, `wsflow`
  from `agents-plugin-wsflow/`), generated per (184) and (188) from the canonical trees:
  the `rsrc/` tree, `runtime.json`, `config-manifest.json`,
  `dashboard-config-manifest.json`, `.claude-plugin/plugin.json`, the Pi
  workflow skills and `skills/lead-bootstrap/AGENTS.template.md` per (182),
  and `bin/ws-mcp-launcher.py` (staged in Phase 2). The `native-pi` payload
  (`agents-plugin/native-pi/`, about 10 MB, 19 tracked files) and its
  materializer `bin/ws-pi-payload.py` are embedded once (`ws` package).
- Loader: `wsrsrc` reads through an `fs.FS` instead of a filesystem root
  (`internal/wsrsrc/loader.go:62-296`, `manifest.go:18-43`); manifest
  hashing, schema checks and variant resolution are unchanged. The package
  comment and `wsrsrc.go:12-14` are rewritten per (183).
- Selection: a connection's package is its existing identity seam,
  `WS_MCP_NAMESPACE` from the package's `plugin.json` MCP env (`wsflow`; unset
  means `ws`), not the `WS_RSRC_ROOT` path. `Server.rsrcRoot` and
  `Server.skillsRoot` (`internal/mcp/process_env.go:228-250`) and their
  callers (`playbook_tools.go:993`, `workflow_manual.go:464,510`,
  `server_sessions.go:134`) resolve the embedded package; the separate
  `wsdoc.ReadInfra` path calls process-global `wsrsrc.ResolveRoot` directly,
  not `Server.rsrcRoot`, and must also resolve embedded resources
  (`agents-plugin-tool/internal/wsdoc/project_tree.go#L43-L61`); the hello no longer needs `WS_RSRC_ROOT` (`cmd/ws-mcp/proxy.go:90-92`).
- Dev override per (175) and (187): `WS_RSRC_DEV_CHECKOUT` points at a
  repository checkout; while it is set, the daemon reads each package's inputs from `<checkout>/<package dir>/` instead of the
  embedded copy and logs that it does. It is never read from a connection's
  hello.
- Project prompt overrides (`ws:lead-tune`) still layer over the served text
  unchanged.
- `native-pi`: `piPreparedPayload` currently invokes the plugin's Python
  materializer (`agents-plugin-tool/internal/wsagent/pi_payload.go#L20-L44`);
  change it to materialize from the embedded payload (staged
  by content hash under the daemon state directory, as `holder.py` is) rather
  than from a plugin root.
- Build entry points (`scripts/dev.py`, `.github/workflows/ws-mcp-ci.yml`,
  `.github/workflows/ws-mcp-release.yml`,
  `agents-plugin-tool/scripts/build-release-assets.sh#L35-L48`, the launcher's
  source-build path in `agents-plugin/bin/ws-mcp-launcher.py`, and the desktop
  assembly, which currently consumes an already-built binary at
  `agents-plugin-tool/desktop-electron/scripts/assemble-daemon-input.cjs#L47-L80`) produce the
  embedded set before compiling; the local-devenv launcher fingerprint
  (`f2ea25fd5`) counts it.
- Desktop: `assemble-daemon-input.cjs` keeps staging only what a reader outside
  the daemon still needs; the Result cites the reader search for each input it
  drops or keeps.
- Compatibility: the worker states the tier per (177) with cited read and
  write lines, including that an older skill shim's playbook names and
  variables all resolve in the embedded tree.
- Tests: an old-plugin-root connection is served the embedded text; ws and
  wsflow connections get their own package's text; the dev override reads the
  checkout; a drift guard fails when the embedded set differs from the
  canonical trees; `go vet` and the existing wsrsrc mirror tests pass.

### Result (b8b1278f7) - 2026-11-19

Implemented in `b8b1278f7`, with lead-directed verification fixes in
`451f13f9f`. **Phase 1 only**; the ticket remains ready for Phase 2.

- `wspackage` embeds the generated package filesystems; `wsrsrc` serves through
  `fs.FS`, preserving schema/hash/variant/include checks and retaining on-disk
  adapters for authoring. MCP playbooks, command-syntax parts, prompt-override
  discovery, bootstrap-template reads and package-aware `infra.read` use the
  build-owned inputs. Caller resource roots no longer select served text;
  namespace `wsflow` selects wsflow, unset/custom display namespaces select ws.
- The explicit daemon-process `WS_RSRC_DEV_CHECKOUT` override reads live
  canonical package inputs and logs its use. A hello cannot set that authority.
  Both packages' manifests, workflow skills/template and launcher are embedded;
  the native Pi payload/helper are embedded once under ws. Native preparation
  stages shared payload bytes plus selected-package overlays by content hash,
  retaining settled stages, then runs the existing offline Python validator.
- One generator owns the ignored asset set, drift check and directory mapping.
  The small tracked `generated/PLACEHOLDER` is now a generated JSON package
  index, also hash-covered by the inventory: bare builds compile and can use
  the explicit dev checkout without tracking a second resource/payload tree.
  Every named build entry generates before compilation. The local fingerprint
  includes canonical inputs before generation and generated content. Desktop
  assembly checks the prebuilt binary's canonical inventory; it cannot retrofit
  an older binary by generating files after compilation.
- Lead continuation decisions resolved both verification stops: cold-build
  tests accept any nonempty alternating generator/Go sequence with per-call
  timeout checks; helper exceptions propagate through `try/finally` to main's
  pre-existing stderr handler. No retry change, new stderr path or scanner
  allowlist was added. Launcher/workflow/native-first-party mirrors are current.

**Desktop reader search and retained inputs.** No staged input is dropped in
this phase. References below are under `agents-plugin-tool/desktop-electron/`
unless qualified:

- `rsrc/` and `skills/`: `daemon-selection.js:123-126` still validates both;
  Pi's outside-daemon selected-workflow skill resolution remains at
  `agents-plugin-pi/src/skills-dir.ts:20-29`. Removing them now breaks readers.
- `.claude-plugin/plugin.json`, `claude-agents/`, `bin/ws-mcp-launcher.py`,
  `config-manifest.json`, `dashboard-config-manifest.json`:
  `daemon-selection.js:73-100` reads identity and the manifest-declared file
  references; `daemon-controller.js:66-67` still exports config paths.
  `runtime.json`, binary and metadata remain read at
  `daemon-selection.js:108-122`.
- The remainder of the finite package closure (`.mcp.json`, `.codex-plugin/`,
  hooks, other bin/skill files) remains part of the existing producer contract
  at `agents-plugin-pi/scripts/prepare-workflow-bundles.py:10-36` and the
  outside-daemon inventory/copy/byte-equality reader at
  `daemon-stage.js:8-38`. Shrinking that closure while hosted package discovery
  is retained would change another consumer contract, not remove an unused
  directory safely. Phase 2's registry/hosted-launch rewrite was not started.

**Compatibility: T1, no data migration.** Abbreviated `internal/...` Go paths
below are relative to `agents-plugin-tool/`; other paths are repository-relative.
`77a778437:` denotes the pre-implementation daemon.

- Old same-minor plugin callers still send their roots (the new launcher keeps
  setting them at `agents-plugin/bin/ws-mcp-launcher.py:894-917`). The new
  reader selects namespace-owned FS at
  `agents-plugin-tool/internal/mcp/process_env.go:231-236` and
  `internal/wsrsrc/package.go:11-35`; `internal/wspackage/package.go:51-88`
  reads only the process dev override or embedded bytes. This changes guidance,
  not caller data. The hello protocol remains 1 and the existing same-minor
  admission remains unchanged (`internal/wsdashboard/proxy.go:36,251-254`).
  Resource schema remains 1 (`internal/wsrsrc/wsrsrc.go:11`), with schema and
  content checks at `manifest.go:22-36`, `loader.go:286-304`.
- Audited the v0.50.0 ws/wsflow skill shims: their playbook calls pass no context
  variables, only names and optional session keys; wsflow's `lead-proceed`
  targets `lead-run` (`v0.50.0:agents-plugin-wsflow/skills/lead-proceed/SKILL.md:8-10`).
  `internal/mcp/embedded_resources_test.go:10-36` exercises all 13 historical
  targets against both embedded packages through actual `playbook.read`, with
  stale caller roots and no process dev override; every target resolves without
  unresolved variables. Runtime-owned mailbox substitution is preserved.
- New writes are disposable hash-named native input stages
  (`internal/wsagent/pi_payload_stage.go:86-130`), not an authoritative store.
  The old daemon reads its plugin helper and `<cache>/native-pi`, never that new
  stage namespace (`77a778437:agents-plugin-tool/internal/wsagent/pi_payload.go:20-42`).
  Materializer schema 3, complete content keys, verification and publication
  remain unchanged (`agents-plugin/bin/ws-pi-payload.py:17,283-309,329-340`).
  Old/new builds therefore use matching entries or distinct content hashes;
  neither rewrites the other's authoritative history. Reopening either build
  serves its own resources; reopening the new build needs no reconstruction.

**Verification (full output read).** From `agents-plugin-tool/` unless noted:

- `WS_TEST_EMBEDDED_PACKAGES=1 go test -p 1 ./internal/wspackage ./internal/wsrsrc ./internal/wsdoc ./internal/wsagent ./cmd/ws-mcp -count=1 -timeout 45m`:
  pass (0.357s / 0.844s / 7.330s / 116.090s / 176.185s), including mirror,
  isolation, native-payload and no-resource-root hello tests.
- `WS_TEST_EMBEDDED_PACKAGES=1 go test ./internal/mcp -count=1 -timeout 45m`:
  pass, 851.729s, including embedded older-shim and prompt-override coverage.
- `go vet ./...`, generated build to a scratch binary, generator `--check`,
  both materializers' `--check-source`, archive freshness, launcher `cmp`, and
  `git diff --check`: pass. Actual placeholder-only compilation and full
  wspackage/wsrsrc/wsdoc tests also pass; MCP/wsagent/cmd compile with only
  that retained index. Generated inputs were restored and drift-checked.
  Offline desktop assembly with the actual newly built binary succeeds for
  both ws and wsflow (no executable probe or daemon launch).
- Repository-root Python suites: generator 8 tests; dev helpers 24; plugin 201
  tests (198 pass, 3 Windows/remote-only skips); wsflow 22 tests (20 pass,
  2 Windows-only skips). No missing dependency was skipped.
- `npm test --prefix agents-plugin-tool/desktop-electron`: 174 tests,
  170 pass / 4 Windows-only skips. No installed application or shared daemon
  was used. Initial fixture/diagnostic failures are fixed; unchanged tracker
  publication/expiry timing failures passed their subsequent isolated/full
  reruns without editing those implementations.
- One fresh independent lite reviewer inspected `77a778437..b8b1278f7` and
  archive contents: clean, no findings. Subsequent fixes follow the lead's
  explicit decisions; no second review was run under the one-pass allocation.

**Boundaries.** Production resource FS operations are in-memory (zero resource
file opens); warm same-process native-input staging writes nothing. These are
source-level operation counts, not a Windows antivirus/latency measurement.
Native Windows execution and live installed-plugin/desktop lifecycle checks
were not performed. No shared daemon rebuild/restart/replacement, installed
plugin mutation, credential operation, merge or push occurred. Phase 2 remains
unimplemented; no last-running-executable record or registry retirement landed.

#### Edition (b37c0a643) - 2026-11-19

Applied the independent review's three Important findings and one Minor finding
under the lead's explicit decisions; **Phase 1 only**, with no change to the
original T1 compatibility verdict or Phase 2 scope.

- Pi's shared cold-start/child-contract builder now runs package generation
  before Go compilation (`agents-plugin-pi/src/local-devenv.ts:241`), using
  its existing `python3` interpreter choice, the tool-directory cwd and the
  launcher's 120-second timeout. Generation failure prevents compilation and
  preserves its error; the real async seam honors the timeout and identifies
  generation errors (`src/bridge.ts:623`). Unit/integration stubs distinguish
  the two commands. `npm ci --ignore-scripts` avoided prepare-hook rewrites;
  `npm run bundle` deliberately refreshed the distributed extension. Workflow
  archives and both native first-party copies were refreshed with their own
  producers, without changing the dependency closure.
- Failed Pi-stage publication now checks a concurrent winner through the same
  validator used for warm reuse; valid winners are accepted and our temp is
  discarded, while invalid/unverifiable winners fail
  (`agents-plugin-tool/internal/wsagent/pi_payload_stage.go:124,135`).
  Deterministic tests cover valid, corrupt, missing-file, non-directory and
  absent targets. Best-effort cleanup removes only sibling `.stage-*`
  directories older than one hour (`:155`); recent temps, settled hash
  directories and the zero-I/O same-process cache path are retained.
- The generator's stat-only `package_input_paths` owns selection for both
  generation and fingerprinting (`agents-plugin-tool/scripts/generate-package-assets.py:40`).
  Launcher fingerprints use canonical relative path, size and mtime only;
  they neither open asset contents nor inspect generated output. Canonical
  additions/deletions and metadata edits still count, while generation cannot
  churn the stamp. Both product launchers and the Pi mirror remain identical;
  tests exercise both product copies with asset opens forbidden.

**File-open measurement.** Python's audit hook around one controlled
inspection-mode launcher `main` call, after module import, with the marker
contract and healthy daemon probe supplied in memory: **355 asset content
opens / 22,327,088 bytes before; 0 opens / 0 bytes after**. All controlled-main
open attempts fell from 361 to 5, including imports and missing-file attempts.
The measurement never connected to or started a real daemon. This is macOS
supporting evidence, not a Windows antivirus latency measurement; canonical
stat/directory operations remain.

**Final verification.** Go commands below ran from `agents-plugin-tool/`;
Go test subprocesses removed inherited `WS_*` settings and set
`WS_TEST_EMBEDDED_PACKAGES=1`.

- `gofmt -d internal/wsagent/pi_payload_stage{,_test}.go`: empty;
  `go vet ./...`: pass.
- `go test -p 1 ./internal/wspackage ./internal/wsrsrc ./internal/wsdoc ./internal/wsagent ./cmd/ws-mcp -count=1 -timeout 45m`:
  pass (0.590s / 1.251s / 7.474s / 118.973s / 182.835s).
- `go test ./internal/mcp -count=1 -timeout 45m`: pass, 898.814s.
  `go test -race ./internal/wsagent -run 'TestPiInputStage' -count=1 -timeout 5m`:
  pass. The initial broad run inherited tool/profile/dashboard restrictions
  and stale intermediate assets; final isolated runs supersede it.
- Repository root: `python3 -m unittest discover agents-plugin/tests`:
  201 tests, 198 pass / 3 platform-only skips;
  `python3 -m unittest discover agents-plugin-wsflow/tests`:
  22 tests, 20 pass / 2 Windows-only skips;
  `python3 -m unittest discover -s agents-plugin-tool/scripts -p test_generate_package_assets.py -v`:
  9 pass. No missing dependency was skipped.
- `npm test --prefix agents-plugin-pi`: 2,689 tests, 2,668 pass / **17
  pre-existing failures** / 4 skips. A separately installed, standalone
  `origin/develop` (`ee1e4de51971751370af6617fea319c2cbca54ff`) snapshot ran
  2,688 tests, 2,667 pass / 17 fail / 4 skips. The final failing test-name
  sets are exactly equal. Targeted local-devenv, child-wiring and mailbox-daemon
  suites pass 46/46 using npm's environment sanitization.
- Generator `--check`, workflow-bundle `--check`, both materializers'
  `--check-source`, launcher `cmp`, and `git diff --check`: pass.

**Verification incident and boundaries.** An initial baseline invocation
incorrectly passed the real repository's `GIT_DIR`/scratch `GIT_WORK_TREE` into
npm's Git-fixture subprocesses. They injected a shared `core.worktree`, test
Git identity and 16 empty `init` commits on this implementation branch. Reflog
and tree comparison proved the commits had exactly the original `1ebd8b02e`
tree; the injected settings were removed and the branch pointer restored by
compare-and-swap, preserving the implementation files and index. Authoritative
npm comparison and affected-package verification were repeated after recovery,
with no inherited Git redirection. The final branch/config checks are clean.
No credential store, shared daemon lifecycle, push or Phase 2 operation occurred.
The 17 baseline Pi failures and native Windows latency/live checks remain outside
this bounded correction.

### Phase 2: Move hosted sessions off the plugin copy and retire the registry

Goal: a hosted lead or worker starts with no installed plugin copy, and the
registry with its same-minor fallback is gone.

- Staged package directory per (186): the daemon stages each embedded
  package (launcher plus its four manifests) into one hash-named directory
  under its state directory, the way `internal/wsholder/holder.go:26` stages
  `holder.py`, and keeps the launcher's package-relative reads unchanged.
- Hosted stdio MCP per (181): `agentMCPCommand`
  (`cmd/ws-mcp/agent_mcp.go:18-48`) runs the staged launcher with the holder's
  Python (`WS_MCP_HOLDER_PYTHON`, else `python3`), `serve --stdio`, and the
  package env from the embedded `plugin.json` (`HostedPluginEnvironment`,
  `hosted_plugins.go:96-125`, rewritten to read the embedded manifest). The
  hosted package is still chosen by `wsconfig.HostedPluginPackageKey`
  (default `wsflow`, `hosted_plugins.go:133-140`). HTTP transports for Codex,
  agy and Claude are unchanged.
- Daemon restart per (185): the daemon records the staged executable it runs
  from in its state directory, and the staged launcher restarts a dead daemon
  from that record when it has no plugin runtime binary. An external plugin's
  launcher keeps its own runtime binary path.
- Removal per (182): the registry writer (`cmd/ws-mcp/proxy.go:83`) and reader
  (`hosted_plugins.go:78`), `ResolveHostedPlugin` (`hosted_plugins.go:133`)
  with its same-minor admission (`:145-164`), `bundledPluginRoot` and
  `daemonPluginRoot` (`lead_plugins.go:15-38`), and their callers
  (`cmd/ws-mcp/agent_mcp.go:21,79`, `pi_lead.go:19,40,166`). A leftover
  `hosted-plugin-roots.json` is ignored, not migrated.
- Compatibility: the worker states the tier per (177) for the new state-dir
  record and the removed registry, including an older daemon started after a
  newer one wrote the record.
- Tests: a hosted Claude, Pi lead and Pi worker start with no plugin copy on
  disk; a daemon killed under a hosted session is restarted by the staged
  launcher from the recorded executable; a daemon replace leaves the hosted
  session's launcher reading the new endpoint; ws and wsflow hosted packages
  each get their own env.

### Result (11f975fb7) - 2026-11-19

Phase 2 landed in `a085da0a8` and `11f975fb7`; Phase 1 and its Edition were
left unchanged. Both phases are complete.

- Hosted package selection reads the global setting (default wsflow), not
  caller roots, installed copies or a registry. `wspackage.StageHosted`
  retains exactly the launcher and four manifests under
  `hosted-packages/<package>/<sha256>/`. Package env comes from embedded
  `plugin.json`, with staged paths substituted. Hosted stdio still runs
  holder Python plus the launcher; HTTP transports are unchanged.
- Pi leads stage their selected workflow skills, rsrc, config and owner
  manifest from embedded inputs, alongside the already embedded native
  payload. Workers resolve the selected package name without an installed
  root. Native Pi and attached MCP launchers carry the hosted marker.
- Removed the registry writer/reader, `ResolveHostedPlugin`, both installed
  root helpers and their callers. A malformed leftover registry is ignored
  byte-for-byte. Invalid global selection warns and uses wsflow.
- The singleton-owning daemon retains its build on every platform and
  publishes the private last-running-executable record once at startup.
  Hosted launchers follow healthy daemons; after proven absence and starter
  coordination they reread/validate that record and restart its retained
  build. Missing/corrupt records fail without plugin repair/download. The
  hosted marker is removed from the daemon starter's environment. External
  launchers retain their own runtime path.
- Settled stages are immutable and retained; corrupt cold-reuse/publication
  winners fail without overwrite. Production warm hosted staging is memory
  only. Unlike that immutable production cache, the explicit dev checkout
  rehashes five inputs at each launch, preventing stale staged manifests from
  disagreeing with live package env. Launcher/workflow/native mirrors and the
  operational runbook are current. Desktop's outside-daemon package closure
  remains unchanged; its retained readers are listed in Phase 1's Result.

**Compatibility: Phase 2 T0; whole ticket T1.** Paths abbreviated below are
under `agents-plugin-tool/` unless otherwise qualified.

- New writes are additive/disposable: the startup writer
  `internal/wsdashboard/running_executable.go:25-42` stages the executable
  and atomically writes schema 1 evidence through the private 0600 writer
  (`endpoint.go:99-122`), under the singleton (`daemon.go:404-410`). The
  Python reader validates shape, retained-bin path and content SHA-256
  (`agents-plugin/bin/ws-mcp-launcher.py:1036-1060`), and reads afresh for
  every necessary restart (`:1717-1730`). No tool, hello, manifest schema or
  authoritative-history format changes.
- The old `489a3ff4e` daemon knows only endpoint/last-port lifecycle files
  (`internal/wsdashboard/endpoint.go:31-59,72-95,125-141`) and its separate
  registry (`internal/wsagent/hosted_plugins.go:17,28-91`). It does not read,
  reject or rewrite the new record or hosted-package stages; settled binary
  pruning was already a no-op (`internal/wsdashboard/spawn.go:87-95`). An
  older daemon started after the new record therefore runs normally and
  leaves it/retained bytes intact. The new reopen publishes its own build
  again; no reconstruction or migration is needed. The lock-owner test
  (`running_executable_test.go:11`) covers unchanged evidence across a
  simulated pre-record startup and successor publication. This is additive
  coexistence, not a claim that old daemons gain plugin-free launch support.
- The old version may continue writing its registry (`489a3ff4e:
  internal/wsagent/hosted_plugins.go:28-73`). The new reader selects only
  config plus build-owned package inputs (`internal/wsagent/hosted_plugins.go:
  20-45`); no production registry reader/writer remains. New reopen ignores
  any old registry writes, and neither version loses authoritative data.
  Existing minor/protocol admission is unchanged; Phase 1's content-only T1
  assessment remains the ticket's highest tier.

**Verification (full output read by each executing agent).** Go commands ran
from `agents-plugin-tool/`; test environments removed inherited `GIT_DIR`,
`GIT_WORK_TREE` and `WS_*`, then set `WS_TEST_EMBEDDED_PACKAGES=1` for Go.

- `go test -p 1 ./internal/wspackage ./internal/wsrsrc ./internal/wsdoc ./internal/wsagent ./internal/wsdashboard ./cmd/ws-mcp -count=1 -timeout 45m`:
  pass (0.690 / 1.341 / 9.085 / 114.730 / 305.872 / 200.674 seconds).
  After the live-dev cache refinement, full wspackage, native-Pi package
  matrix and `go vet ./...` passed again. `go build ./...` and
  `go test -race ./internal/wspackage -count=1 -timeout 5m`: pass.
- Separately, `go test ./internal/mcp -count=1 -timeout 45m`: pass,
  **846.576 seconds**. No dependency absence was skipped.
- The real lifecycle test runs ws and wsflow with isolated cache/config and
  five-file stages: one persistent stdio launcher follows a newer-core
  successor, then restarts that successor after a kill with both original
  build paths deleted. Subsequent tool reads succeed on a distinct healthy
  PID. Package/runner fixtures cover Claude launch construction, native Pi
  lead materialization and worker payload/MCP wiring without installed roots;
  these are not live model turns.
- Python plugin suite: **205 tests, 202 pass / 3 platform-or-remote skips**;
  wsflow: **22 tests, 20 pass / 2 Windows skips**. Generator `--check`,
  workflow-bundle `--check`, both materializers' `--check-source .`, launcher
  byte comparisons and `git diff --check`: pass. Two initial materializer
  check invocations had incorrect CLI arguments; corrected repository-root
  invocations passed without any implementation/test change.
- Pi `npm test`: **2,689 tests, 2,668 pass / 17 pre-existing failures /
  4 skips**. Independently installed `origin/develop`
  (`ee1e4de51971751370af6617fea319c2cbca54ff`) ran 2,688 tests,
  2,667 pass / 17 fail / 4 skips; sorted failing names match exactly.
  Desktop: **174 tests, 170 pass / 4 Windows skips**. Missing dependencies
  were installed with `npm ci --ignore-scripts`. Full logs and comparison:
  `node_modules/phase2-verification/report.md`. Non-authoritative baseline
  setup attempts (Node's node_modules TypeScript restriction, enclosing Git
  discovery and producer identity) are documented there; final baseline Git
  metadata is isolated and no real-repository Git redirection was inherited.
- Fresh independent one-pass Lite review of `489a3ff4e..11f975fb7`: **clean**.
  Reviewer independently reran both real lifecycle cases, hosted/native-Pi
  tests, launcher start/proxy suites and mirror guards. Detailed ignored
  report: `node_modules/phase2-verification/independent-review.md`.

**File-open evidence and boundaries.** A Python audit hook around controlled
healthy-launcher `main` after imports measured **4 -> 2 open attempts** versus
`489a3ff4e`: hosted startup no longer attempts two plugin runtime metadata
reads; its two runtime-contract reads remain. Network/client handling was
supplied in memory and no daemon started. Production warm staging writes zero
files; cold staging publishes five package files once. The restart record is
written once per daemon startup, never per session/tool/heartbeat. These are
macOS supporting evidence/source counts, not native Windows antivirus latency
measurements.

**Unresolved/outside this phase.** External harness skill discovery still reads
its installed package (`agents-plugin-pi/src/skills-dir.ts:20-29`, and Claude/
Codex's own plugin discovery); external connection config manifests still read
caller paths (`internal/mcp/process_env.go:148`, `config_manifest.go:172-174`).
Phase 2's hosted-launch/removal text does not cover those external paths, so
neither was expanded. The 17 baseline Pi failures, native Windows execution/
latency and live Claude/Pi model-session acceptance remain unverified here.
No shared daemon build/restart/replacement, installed plugin or credential
mutation, develop merge or push occurred.

**Workflow tooling.** `workflow_manual`, `route.resolve_implement`,
`tickets.acquire`, `playbook.render` and `tickets.close` were unavailable in
this session profile. Used the bundled manual/reviewer fallback and the
already-assigned implementation branch; chose the same one-pass Lite review
as Phase 1. Closure uses the documented native move fallback. The ticket's
Sage freshness warning is accepted: additions record implementation decisions
and evidence, not a changed approved phase plan. The lead retains merge/push
and merge-confirm authority.
