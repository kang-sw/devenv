---
title: Windows launcher bootstrap metadata destination still reaches MAX_PATH after the short-temp repair
---

# Windows bootstrap metadata destination reaches MAX_PATH

## Background

The full native `cmd/ws-mcp` suite at exact source
`9854ff350fa1b0eccfb5c71649c5190c405925b9` failed
`TestLauncherRunsMailboxWaitThroughTheDaemonLikeTheDirectCLI` after the inherited
BOM/short-metadata-temp repair `df96a866e` was already present. No active matching
IDEA was found by metadata-temp or MAX_PATH discovery.

Classification: confirmed native launcher-integration acceptance failure at a
deep fixture path. The measured destination length supports a remaining path-size
risk; it does not independently establish all causation or an installed-host
failure. No source repair, OS long-path setting change, or retry was performed.

## Reproduction and evidence

Actual native command from the Go module:

```text
go test -count=1 -timeout 10m ./cmd/ws-mcp
```

Expected: the launcher-hosted mailbox command and direct command both return exit
3 for the fixture's tested condition; bootstrap metadata installs and the owned
fixture daemon is reachable.

Actual at `cmd/ws-mcp/launcher_proxy_test.go:354`: hosted exit 1, direct exit 3.
The launcher reported `runtime repair failed: [WinError 3]` while renaming
`.metadata.<pid>.<uuid>.tmp` to the bootstrap candidate's metadata sidecar,
then `dashboard daemon not reachable` after its 75-second budget. The same
rename failure appeared twice inside the single test's normal launcher recovery
path; no external test retry was added. Test duration 82.97s; package 268.263s;
command exit 1 after 270.679s, starting 2026-11-14T13:49:31Z.

The two logged rename attempts each have a 210-character source path and a
260-character destination path after decoding the diagnostic's escaped Windows
backslashes. The destination retains
`.ws-mcp-0.50.0-dev-<contract>.exe.bootstrap.<pid>.<uuid>.tmp.metadata.json`.
The shortened `.metadata...tmp` source therefore does not bound the bootstrap
sidecar destination itself. Exact paths remain only in untracked evidence.

Temporary artifact index: `96d19897-01-windows-696-repass-evidence.md`;
complete raw output: `windows-696-9854ff350/native/cmd.log`, corresponding to
`evidence/cmd.log` in the owned native Temp fixture. Native runner and full argv
are preserved beside the artifact. No machine address, user path, or token is
needed in this ticket.

Environment: native ThinkPad P14s Gen 2a, Windows 11 Pro 10.0.26200, Go 1.26.2
windows/amd64, Python 3.12.10, Git 2.38.1.windows.1. Exact source archive SHA256
verified before extraction:
`542a67db53840af5f0c3d7bfafefc08d4576dcd5372564485c7bba80c8f35a87`.
All test homes, caches/configuration, runtime directories, processes and ports
were fixture-owned. Native source build passed first. The existing installed
plugin, user checkout, shared daemon and host long-path settings were untouched.

Separately, an isolated actual-source launcher cold build and raw MCP round-trip
passed in this run (`native/cold-source.log` and
`native/cold-launcher.stderr.log`). That shorter-path success does not erase this
integration failure or justify describing the whole cmd suite as green.

## Decisions

- Diagnose the bootstrap destination as well as the temporary source. Do not
  assume the inherited short-temp repair closes every MAX_PATH boundary.
- Do not require an owner registry/security/PATH change to make the test pass.
  Settle whether a product path bound or a fixture-specific precondition is the
  correct contract before implementation.
- This capture authorizes no source fix, API/protocol/store change, compatibility
  tier change, promotion, merge or deployment.

## Phases

### Phase 1: Diagnose and bound native bootstrap metadata paths

Establish the precise Windows filesystem failure and the supported path contract,
then repair the appropriate implementation or fixture without masking a product
failure. Verify metadata identity and cleanup across cold bootstrap, compatible
runtime reuse and mailbox CLI startup, including native deep-path coverage.
