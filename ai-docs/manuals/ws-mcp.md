---
summary: MCP operational runbook, launcher environment, release and verification steps
---

# ws MCP Operations Runbook

Operational runbook for the host-neutral `ws-mcp` runtime used by the `ws`
plugin package.

This file is not the MCP tool contract or current tool inventory. That contract
is the test suite under `agents-plugin-tool/internal/`, and the implementation
coupling is the code itself:

- MCP dispatch, schemas, and tool allowlist: `agents-plugin-tool/internal/mcp/`
- plugin launcher and release packaging: `agents-plugin-tool/internal/wsrsrc/`
  and each package's `runtime.json`

For live tool schemas and current inventory, ask the runtime: use MCP
`tools/list`, `ws/runtime.read`, `ws-mcp runtime capabilities`, or the source
registry in `agents-plugin-tool/internal/mcp/server.go`.

## Tool Output Convention

An MCP tool's output is read by an LLM before it is read by anything else, so a
new tool's default response is compact, readable text. Return JSON only when a
caller needs stable machine parsing, protocol metadata, or a
compatibility-preserving shape, and put it behind an explicit opt-in (a
`format: "json"` argument) rather than making it the default. Nothing enforces
this - `toolTextResponse` and `toolJSONResponse` are equally available at every
dispatch site in `agents-plugin-tool/internal/mcp/server.go` - so it has to be
applied when the tool is written.

## Process Model

The baseline server is stdio MCP, not a background OS daemon.

```bash
ws-mcp serve --stdio --root <repo-root>
```

The host launches the command and communicates over stdin/stdout JSON-RPC. The
server advertises MCP protocol version `2025-03-26` and tool capability.

Local command examples:

```bash
ws-mcp version
ws-mcp doctor --root <repo-root>
ws-mcp runtime info
ws-mcp runtime capabilities
ws-mcp serve --stdio --root <repo-root>
```

`doctor` is a host-independent smoke check. It verifies the repository root,
`ai-docs/`, and `AGENTS.md`; downstream projects should rely on MCP tools for
bundled conventions rather than repository-local source paths.

### Dashboard daemon

`ws-mcp dashboard` is an optional machine-wide daemon (one per OS
environment) that `serve` starts detached when `dashboard.enabled` is on. MCP
tools never depend on it. Its state lives under `<cache root>/dashboard/`:

- `dashboard.lock` - the single-instance OS file lock; `endpoint.json` - port,
  pid, version, and the shutdown token; `dashboard.log` - the running daemon's
  log, including one line per session added to or removed from its index;
  `lan-token` - the LAN access token (LAN mode only), kept across daemon
  restarts; deleting it issues a new one at once. `last-port` - the port the
  last daemon bound: when `dashboard.port` is taken by another program, the
  daemon binds this port before an OS-assigned one, so a restart keeps its
  origin (page localStorage, LAN bookmark); a free `dashboard.port` always
  wins.
- `sessions/<reply-id>/registration.json` - one entry per top-level lead
  session whose `dashboard.enabled` resolves on (session scope included),
  written by the serving ws-mcp at its first keyed call and heartbeated every
  2 minutes; a session counts as live while its heartbeat is under 5 minutes
  old and it has not ended. The daemon only reads this registry; ws-mcp prunes
  entries unseen for 7 days.
- `sessions/<reply-id>/snapshot.json` and `sessions/<reply-id>/ring/` - the
  session's live snapshot (current lead and subagent activity, rewritten
  debounced and on a 30-second heartbeat, stale after 3 missed heartbeats by
  its `updatedAt`) and its conversation ring (the last 50 finished messages,
  one file each). ws-mcp writes them from host adapter events
  (`dashboard.intake`) and from its mailbox send and recv path, only for a
  registered session; `internal/wslive` holds the schema and readers.
- `sessions/<reply-id>/inbox/` - the board-answer inbox (`internal/wsinbox`):
  held messages the user sent from the page, one file each; `drained/` keeps
  the newest 20 as sent records. The adapter-only `dashboard.inbox_drain` tool
  drains it, and `mailbox wait --board-inbox` also wakes on it (peek only). A
  message promoted to steering is rewritten with `steer: true` and is no
  longer a merge target; `dashboard.inbox_drain steer_only: true` claims only
  those, and `mailbox wait --board-steer` wakes only on one (the wait an
  adapter arms mid-turn; exclusive with `--board-inbox`).

After a store write the serving ws-mcp sends `POST /nudge {reply_id,
project}`; the daemon also re-reads every session every 30 seconds.

The daemon serves its pages with no CORS: `/` lists sessions by project
(ended ones for a day), `/s/<reply-id>` shows one session, and `/events` and
`/s/<reply-id>/events` are their SSE streams, which send only the page regions
whose hash changed. A session with a fresh adapter snapshot gets the live
layout (lead, agent tree, conversation, board); one without gets the
board-first page. On the served board and in the conversation's lead
replies, a file link renders as an action link (`data-wl="file"`, titled with
its repo-relative path) and an http(s) link is marked `data-wl="url"`: a
click opens a menu (open in new tab, open in system browser, copy relative or
absolute path; copy URL for an http(s) link), and Cmd/Ctrl+click opens a new
tab. A file inside the session's project root links `GET
/s/<reply-id>/file?path=<root-relative path>`, the file view, which serves a
regular, non-symlink file that still resolves inside the root after every
symlink (404 otherwise), to a loopback viewer and a LAN token holder alike:
an image inline, markdown rendered in the board's escaped mode with relative
links resolved against the file's own directory, other UTF-8 text as
`text/plain`, and a binary or over-2-MiB file as a page offering the path
copies. A file outside the root offers the copies only. A board image inside
the root loads through `GET /s/<reply-id>/image?path=<root-relative path>`,
which serves only an image-extension file under the same containment, and
any other image renders as its path. A finished lead reply draws its mermaid
fences; the reply still streaming keeps them code blocks. A finished lead
reply also keeps its inline HTML and SVG through an allow-list sanitizer: no
script, form, embed, or fetching element or attribute survives, and a raw
link or image follows the same link rule, so it renders as the same action
link or path text as its markdown form; the streaming reply, user messages,
and mail bodies show HTML as text. `POST
/s/<reply-id>/open {target}` opens an http(s) URL or this session's own file
view path in the host's default browser with the OS open command, run with
no shell (`rundll32 url.dll,FileProtocolHandler` on Windows); it requires a
viewer on this machine (404 to any other, token or not) and an `Origin`
equal to the daemon's own, and the page offers the item only to such a
viewer. The live layout's
composer sends: `POST /s/<reply-id>/message {text, chips, images, refs,
refNums}` holds the message in the session's inbox (merged into a still-held
one that is not steered). `images` are new uploads `{name, data, n}` (base64,
sniffed as PNG/JPEG/GIF/WebP, at most 8 per held message), `refs` name files
a cancel returned, and `refNums` numbers them; the daemon writes the uploads
into the inbox's `attachments/` directory. The text names image `n` with an
`[Image #n]` marker, and the delivered message is the text, one `Image #n:
<absolute path>` line per image, then the board answers; a send that merges renumbers its
images and markers to continue the held message's, under the daemon's inbox
lock. The page still reads the older `[image: <path>]` lines. `POST
/s/<reply-id>/cancel {id}` takes a held one back, answering `{text, chips,
images, nums}`, and `POST
/s/<reply-id>/steer {id}` promotes a held one to steering, answering the
rewritten entry's new `{id}` (409 once the session has received it). The
adapter claims a steered message at the session's next tool boundary instead
of at the turn end. All three require an `Origin` equal to the daemon's own,
and a session without a fresh adapter snapshot gets 409. `whiteboard.update`'s `full:` line names
`http://127.0.0.1:<port>/s/<reply-id>` once the session's last nudge was
answered, and the page path otherwise.

By default the daemon binds loopback and every route requires a loopback
Host. LAN mode (`dashboard.lan_enabled`, read from the global config layer
only and passed as `dashboard --lan`) binds every interface: a request whose
peer address and Host are both loopback still passes with no token, and every
other request, on every route, needs the LAN access token, which is separate
from the endpoint file's shutdown token. The token arrives as `?token=` on the
address the index page shows to a viewer on this machine only; that answer
sets an HttpOnly cookie, which the page's links, SSE stream, images, and posts
then carry, so no rendered URL holds it. `/health` reports the mode as `lan`,
and a `serve` whose global value differs from a same-version daemon's mode
replaces it through the normal handoff.

At an equal version the newer build wins: `/health` reports `build_time` (the
ws-mcp executable's mtime in Unix milliseconds, read from the original binary
and passed to the daemon as `dashboard --build-time`), and a `serve` whose
build time is newer than the daemon's (a daemon that omits it counts as older)
replaces it; an older or unknown build time never does, so builds of one
version side by side leave each other's newer daemon alone. A rebuilt dev
binary therefore replaces the running daemon at the next `serve`. To force a
restart anyway, `POST /shutdown` with `Authorization: Bearer <token from
endpoint.json>`; the next `serve` starts a new one.

## Plugin-Managed Startup

Codex plugin bundles can reference plugin-local MCP configuration:

```json
{
  "skills": "./skills/",
  "mcpServers": "./.mcp.json"
}
```

The plugin-local `.mcp.json` starts the runtime through the Python launcher:

```json
{
  "mcpServers": {
    "ws": {
      "command": "python3",
      "cwd": ".",
      "args": ["./bin/ws-mcp-launcher.py", "serve", "--stdio"],
      "startup_timeout_sec": 30,
      "tool_timeout_sec": 600
    }
  }
}
```

`cwd: "."` is intentional. Codex normalizes it to the installed plugin cache so
`./bin/ws-mcp-launcher.py` resolves inside the plugin package. Because that also
makes the MCP process cwd the plugin cache, tools must not infer the project
root from process cwd.

The launcher exports `WS_MCP_PROJECT_ROOT` when it can infer the caller project
root from the parent Codex process. Skills may still pass explicit absolute
roots for compatibility, and lead actor setup should pass the repository's
absolute filesystem path.

Native Windows plugin-managed startup uses the same Python launcher. Windows
users need a working `python3` command on `PATH`; if the Windows Store alias is
present without Python installed, install Python 3 and refresh the plugin.

## Launcher Environment

Current launcher inputs:

| Variable | Purpose |
|----------|---------|
| `WS_MCP_RUNTIME_DIR` | Override the runtime binary directory; defaults to plugin-local `.runtime/<os>-<arch>`. |
| `WS_MCP_RUNTIME_BINARY` | Exact repaired runtime binary path exported by the launcher for async worker recovery. |
| `WS_MCP_BOOTSTRAP_BINARY` | Copy a prebuilt local binary into the runtime directory. |
| `WS_MCP_BOOTSTRAP_URL` | Download a prebuilt binary when no runtime binary exists. |
| `WS_MCP_BOOTSTRAP_SHA256` | Optional SHA-256 checksum for `WS_MCP_BOOTSTRAP_URL`. |
| `WS_MCP_RELEASE_REPOSITORY` | Override the GitHub release repository from `runtime.json`. |
| `WS_MCP_RELEASE_TAG` | Override the release tag from `runtime.json`, for example `v0.48.10`. |
| `WS_MCP_RELEASE_BASE_URL` | Override the full release asset base URL; useful for local file or HTTP smoke tests. |
| `WS_MCP_LAUNCHER_DEBUG` | Print launcher diagnostics to stderr when set to `1`. |
| `WS_MCP_PROJECT_ROOT` | Project root default for root-aware tools and CLI commands when no higher-priority root exists. |
| `WS_MCP_NO_AGENT` | Product-mode gate for agentless distributions such as wsflow. |
| `WS_MCP_NAMESPACE` | User-facing MCP namespace text override; empty or unset defaults to `ws`. |
| `WS_MCP_SETUP_TOOL` | Advertised setup tool name override; empty or unset defaults to `ws.setup`. |
| `WS_MCP_CONFIG_MANIFESTS` | Adapter config key manifests, separated by the platform path-list separator (`:` POSIX, `;` Windows); `;` is accepted on every platform, which the static launcher configs use. Declared keys become `config.tune`-writable, cataloged by `config.list`, and typed in `config.get`; a rejected manifest is logged to stderr and listed by `config.list`. The ws and wsflow packages each ship their own `config-manifest.json` (namespace `claude.`; the same keys, differing only in `default`, pinned by `TestShippedClaudeManifestsDifferOnlyInDefaults`) and set it in both launch configs: `.mcp.json` (Codex) as `./config-manifest.json`, resolved against `cwd: "."` (the plugin root), and `.claude-plugin/plugin.json` as `${CLAUDE_PLUGIN_ROOT}/config-manifest.json`. Each package (ws, wsflow, Pi) also lists its `dashboard-config-manifest.json` (namespace `dashboard.`: `dashboard.enabled`, default on in ws and Pi, off in wsflow; `dashboard.port`, default 29417; `dashboard.lan_enabled`, global-only and default off in every package) beside its own manifest; the Pi bridge joins the two with the platform separator. A path that does not resolve fails nothing: the keys stay undeclared, so check `config.list` after changing either. |
| `WS_MCP_CLI_NAME` | Command name that printed agent-runnable CLI commands (the rendered `mailbox wait` command and its re-arm line) start with; empty or unset uses the invoked binary path (`os.Args[0]`). Set by the `ws-cli`/`wsflow-cli` shims and by the Claude manifests' `mcpServers.<name>.env`, where plugin `bin/` is on the agent's Bash PATH; left unset for Codex, which does not put plugin `bin/` on PATH. |

Launcher diagnostics must go to stderr. Stdout belongs to the MCP JSON-RPC
stream until the launcher execs the repaired runtime.

## Runtime Repair And Drift

The launcher reads plugin-local `runtime.json`, repairs a missing or
incompatible runtime binary, then execs `ws-mcp`.

Runtime binaries live in the installed plugin cache by default:

```text
<installed-plugin-cache>/.runtime/<os>-<arch>/ws-mcp-<plugin-version>-<runtime-json-sha12>[.exe]
```

Compatibility checks use `ws-mcp runtime capabilities` when available. That
single probe reports runtime version, source commit, MCP protocol version,
prompt bundle metadata, MCP tool names, and CLI command names needed by the
launcher. Older runtimes fall back to bounded legacy validation when the
contract allows it.

`runtime.json` is active compatibility data, not release notes. Version, tool,
command, and prompt-bundle drift can make a working binary stale. Use the
version bump helper instead of editing release/version references by hand:

```bash
agents-plugin-tool/scripts/bump-ws-version.sh <X.Y.Z>
```

The helper updates plugin manifests, runtime contracts, Go runtime development
defaults, release workflow references, build script defaults, selected
documentation references, and the `agents-plugin-pi/` mirrors (`runtime.json`,
`bin/ws-mcp-launcher.py`, `rsrc/`, resynced byte-for-byte from `agents-plugin/`,
plus both `ws-pi-bridge` `package.json` versions). Development binaries such as `0.48.10-dev` satisfy the
plugin version the helper just set; older or newer minor releases are stale. The
concrete range lives in `runtime.json.required_mcp` — read it there rather than
restating a version here, which froze at `0.29.2` for seven minor releases
because no bump-script regex matches a bare `X.Y.Z`.

## Release Distribution

`ws-mcp` is distributed as prebuilt native binaries produced from
`agents-plugin-tool/`. End users should not need Go, Node, Cargo, or Visual
Studio Build Tools to run the MCP server.

Release asset names follow this pattern:

```text
ws-mcp-<os>-<arch>[.exe]
```

Initial assets:

```text
ws-mcp-darwin-arm64
ws-mcp-darwin-amd64
ws-mcp-linux-amd64
ws-mcp-linux-arm64
ws-mcp-windows-amd64.exe
ws-mcp-windows-arm64.exe
SHA256SUMS
```

`SHA256SUMS` is a plain `shasum -a 256` manifest covering every release binary.
The launcher verifies the matching checksum line before chmod/exec. Offline or
proxy failures should keep stdout clean and report the URL or runtime directory
that needs manual repair.

The local build script is:

```bash
agents-plugin-tool/scripts/build-release-assets.sh [version]
```

GitHub Actions workflow `.github/workflows/ws-mcp-release.yml` runs tests,
cross-compiles assets, uploads workflow artifacts, and publishes GitHub release
assets for pushed `v*` tags. Pull requests touching workflow or plugin/runtime
paths run checks without publishing.

## Local Devenv Repair

Local development has one repository-specific repair exception. When the
installed plugin path is under `~/.codex/plugins/cache/kang-sw-devenv/<ws|wsflow>/`
or `~/.claude/plugins/cache/kang-sw-devenv/<ws|wsflow>/` and the installed cache
contains a valid `.local-devenv-runtime` contract, the launcher forces local
runtime repair before accepting an already compatible cache-local binary. Both
the Codex and Claude plugin caches are recognized so the same source-build
dogfood loop works on either host.

The contract format is:

```json
{
  "schema_version": 1,
  "source_root": "/Users/kang-sw/devenv",
  "tool_dir": "/Users/kang-sw/devenv/agents-plugin-tool",
  "go": "/opt/homebrew/bin/go"
}
```

All paths must be absolute. `source_root` and `tool_dir` must exist, `tool_dir`
must contain `cmd/ws-mcp`, and `go` must be an existing file (an executable bit is
required on POSIX; on Windows the `go.exe` file is trusted because
`os.access(X_OK)` is not meaningful there). If the marker is missing, invalid
JSON, missing required fields, or references missing paths, local repair is
inactive and the launcher uses the normal cache/release path. Local devenv repair
is honored on all platforms including Windows; the build inherits a launch
environment with `HOME` recovered on POSIX and `USERPROFILE`/`LOCALAPPDATA`
recovered on Windows so `go build` resolves its module/build caches under a
sanitized launch env.

When the contract is valid, the forced local path builds
`<tool_dir>/cmd/ws-mcp` with the declared Go executable first so pre-release
dogfood exercises the current checkout. Non-forced repair may copy a local
runtime binary from `<tool_dir>/dist/` or the contract-addressed
`<source_root>/agents-plugin/.runtime/<os>-<arch>/ws-mcp-<plugin-version>-<runtime-json-sha12>`
path before building. Legacy fixed-name source-cache binaries such as
`agents-plugin/.runtime/darwin-arm64/ws-mcp` are not local repair candidates.

If no compatible local runtime can be installed while a valid marker is active,
startup fails instead of falling back to the published release asset.

While a valid marker is active, the runtime version gate is relaxed from an exact
patch match to a **same-minor** match: a locally built runtime is accepted when
its `(major, minor)` equals the installed snapshot's `plugin_version`, regardless
of patch (a `-dev` suffix is ignored). The live source version routinely drifts a
few patches ahead of the installed snapshot's `runtime.json` between
`install.sh update` runs, and the exact-match gate would otherwise reject the
freshly built runtime after every local patch bump, breaking the MCP server until
a re-snapshot. A different minor or major is still rejected, because a minor bump
is a real contract change that must go through a re-snapshot. Released/downloaded
runtimes (no marker) keep the strict exact-match gate unchanged.

### Enabling the local dogfood loop (manual setup)

The marker is per-machine and gitignored, so place it once in the source
checkout for each package you dogfood, then let the plugin install or refresh
copy it into the plugin cache:

1. Write `agents-plugin/.local-devenv-runtime` with this machine's absolute
   paths (use `command -v go` for the Go path). If dogfooding the separate
   wsflow package, write the same marker to
   `agents-plugin-wsflow/.local-devenv-runtime`:

   ```json
   {
     "schema_version": 1,
     "source_root": "/home/you/devenv",
     "tool_dir": "/home/you/devenv/agents-plugin-tool",
     "go": "/home/linuxbrew/.linuxbrew/bin/go"
   }
   ```

2. Run `./install.sh update`. The `rsync` step copies the marker into the
   plugin snapshot and `claude plugin install` carries it into the versioned
   cache, so the next MCP launch source-builds the current checkout.

After setup, editing the Go source and reconnecting the MCP server is enough to
pick up changes — no rebuild-and-stage step. To disable, delete the marker and
re-run `install.sh update` (the launcher reverts to the cache/release path).

This path exists only for the repository-local Codex or Claude plugin
development loop. The marker file is gitignored and should not exist in normal
GitHub release installs, downstream repositories, or Windows installs.

## Development Verification

Use three verification levels while developing `ws-mcp` and plugin-managed
runtime behavior.

Level 1 validates the Go runtime and host-independent MCP server:

```bash
cd agents-plugin-tool
go test ./...
scripts/smoke-ws-mcp.sh ..
```

Level 2 validates local release assets:

```bash
cd agents-plugin-tool
scripts/build-release-assets.sh 0.48.10-dev
dist/ws-mcp-darwin-arm64 version
cd dist
shasum -a 256 -c SHA256SUMS
```

Level 3 validates Codex plugin-managed MCP startup from the installed plugin
cache. This level requires a human-in-the-loop plugin cache refresh when plugin
files change: uninstall/install `ws` in the Codex UI or start a fresh session
after cache refresh is available. Then run:

```bash
codex mcp get ws
codex exec --dangerously-bypass-approvals-and-sandbox --json \
  'There is an enabled MCP server named ws. Use its tool named project_tree with arguments {"root":"/Users/kang-sw/devenv"}. Do not use shell commands. Reply with the exact server name, exact tool name, and the first non-empty line of the tool result.' \
  < /dev/null
```

Success means the JSONL output contains an MCP tool call with server `ws`, tool
`project_tree`, and a result whose first non-empty line is `ai-docs/`.

Use Level 1 for ordinary Go/MCP changes, Level 2 for release/build changes, and
Level 3 whenever `.codex-plugin/plugin.json`, `.mcp.json`, launcher behavior, or
installed plugin packaging changes.

## Troubleshooting

- MCP startup fails before JSON-RPC: inspect launcher stderr; stdout must remain
  clean.
- Plugin-managed root resolution points at the plugin cache: pass an explicit
  absolute `root`, call setup with the repository's absolute filesystem path, or
  check `WS_MCP_PROJECT_ROOT` propagation.
- Installed plugin files changed but Codex still starts old behavior: refresh
  the plugin cache through UI uninstall/install or a fresh Codex session.
- Runtime surface mismatch: run `ws-mcp runtime capabilities` against the
  cache-local binary and compare it with `runtime.json`.
- Windows startup reports Python alias problems: install Python 3 and refresh
  the plugin so Codex rematerializes the MCP entry.
- Routing or implementation context lost after context compaction: expected — the
  transcript is summarized, but session state persists in
  `<cache-root>/keys/<session-key>.json`. Recover by calling
  `ws.workflow_manual(session_key: <key preserved in the compaction summary>)`,
  which reloads the manual primitives and restores the agenda/todo Session State.
  Do not call `ws.ferrule` to "re-enter" — it is non-idempotent and mints a new key,
  orphaning the prior state.
