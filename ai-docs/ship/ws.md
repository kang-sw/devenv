# Ship: ws

The `ws` workflow plugin and native `ws-mcp` runtime, published from
`agents-plugin/` and `agents-plugin-tool/`.

## Version Strategy

Semantic versioning for the Codex-first plugin.

Source of truth:
- `agents-plugin/.codex-plugin/plugin.json` -> `version`
- `agents-plugin/runtime.json` -> `plugin_version` and `release_tag`

Version coupling:
- `agents-plugin/.claude-plugin/plugin.json` mirrors the Codex-first candidate
  version for compatibility metadata inside `agents-plugin/`.
- `agents-plugin/runtime.json.release_tag` must be `v<version>`.
- `agents-plugin/runtime.json.required_mcp`, all runtime tool ranges, and all
  command ranges must cover `<version>-dev` and stay below the next minor.
- `agents-plugin-tool/cmd/ws-mcp/main.go` uses `<version>-dev` before tagging;
  release assets embed `<version>` when built from the tag.

Bump rules:
- **Minor** (`0.X.0`): new Codex-visible skill, MCP tool family, runtime
  command family, or plugin-managed install capability.
- **Patch** (`0.0.X`): behavior change or bug fix to existing skills, MCP
  tools, runtime, launcher, docs, or packaging with no new public entry point.
- **Major** (`X.0.0`): breaking change to canonical workflow, plugin layout,
  MCP protocol expectations, or install/repair contract.

At ship time:
1. Run `git tag --list 'v*' --sort=-v:refname | head -n1`.
2. Run `git log <last-tag>..HEAD --oneline`.
3. Classify commits by the bump rules.
4. If the version must change, run
   `agents-plugin-tool/scripts/bump-ws-version.sh <version>`.
5. Verify the changed files named by the bump helper before committing.

## Pre-flight

- `git status --porcelain` - must be empty before release preparation starts.
- `python3 -m unittest discover agents-plugin/tests`
- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `cd agents-plugin-tool && go test ./...`
- `cd agents-plugin-tool && scripts/smoke-ws-mcp.sh ..`
- `claude plugin validate agents-plugin`
- `claude plugin validate agents-plugin-wsflow`
- `bash -n install.sh`
- Confirm `.agents/plugins/marketplace.json` contains both `ws` and `wsflow`
  local Codex plugin entries when packaging changed.
- Confirm `install.sh update` snapshots and installs only the `ws` Claude
  plugin, and does not enable or install `wsflow@kang-sw-devenv`.

## Changelog

Update `CHANGELOG.md` before tagging:

```markdown
## v<version> - YYYY-MM-DD

### Added
- <new Codex skill, MCP tool, runtime command, or install capability>

### Changed
- <behavior change>

### Fixed
- <bug fix>
```

One entry per shipped version. Derive content from
`git log <last-tag>..HEAD --oneline`.

## Build

Local release-asset verification:

```bash
cd agents-plugin-tool
scripts/build-release-assets.sh <version>
host_os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$(uname -m)" in arm64|aarch64) host_arch=arm64 ;; x86_64|amd64) host_arch=amd64 ;; esac
"dist/ws-mcp-${host_os}-${host_arch}" version
cd dist
shasum -a 256 -c SHA256SUMS
```

Notes:
- Do not commit `agents-plugin-tool/dist/` unless a separate ticket changes the
  artifact policy.
- The build script refreshes `agents-plugin/runtime.json` prompt bundle metadata
  when the host binary can report it; commit any resulting metadata drift before
  tagging.
- wsflow reuses the same runtime assets. Keep
  `agents-plugin-wsflow/runtime.json` and package tests aligned with the
  selected wsflow no-agent runtime surface before tagging.

## Tag

Format: `v<version>` (for example, `v0.16.0`)

Command:

```bash
git tag -a v<version> -m "v<version>"
```

Do not push the tag until the final confirmation gate.

## Publish

Publish targets:
- `origin/main`
- annotated tag `v<version>`
- GitHub Actions release assets uploaded by `.github/workflows/ws-mcp-release.yml`
  when the `v*` tag is pushed
- Codex GitHub plugin marketplace install from repository
  `.agents/plugins/marketplace.json` pointing at `./agents-plugin` for `ws`
  and `./agents-plugin-wsflow` for `wsflow`

Publish command after explicit final approval:

```bash
git push origin main --follow-tags
```

Expected GitHub Actions behavior:
- Pull requests that touch the workflow, marketplace, plugin, or runtime paths
  run tests, build release assets, verify checksums, and upload workflow
  artifacts without publishing a GitHub release.
- Tag push runs tests, builds release assets, verifies checksums, creates or
  updates the GitHub release, and uploads `agents-plugin-tool/dist/*`.

## Post-ship

1. Confirm the GitHub Actions workflow succeeds for `v<version>`.
2. Confirm the GitHub release contains:
   - `SHA256SUMS`
   - `ws-mcp-darwin-arm64`
   - `ws-mcp-darwin-amd64`
   - `ws-mcp-linux-amd64`
   - `ws-mcp-linux-arm64`
   - `ws-mcp-windows-amd64.exe`
   - `ws-mcp-windows-arm64.exe`
3. Dogfood Codex GitHub plugin install from `kang-sw/devenv`.
4. In a fresh Codex session, confirm:
   - `ws` plugin is installed from the GitHub marketplace entry
   - `wsflow` plugin is installed from the GitHub marketplace entry when the
     agentless derivative is part of the release
   - `$ws:lead-workflow-manual` is visible
   - `$wsflow:lead-workflow-manual` is visible when wsflow is installed
   - MCP server `ws` starts from plugin-managed `.mcp.json`
   - MCP server `wsflow` starts from plugin-managed `.mcp.json` without
     named-agent or subquery tools
   - `runtime.info` reports the shipped version and matching prompt bundle hash
   - `project_tree` returns `ai-docs/` as its first non-empty line
5. Optional local dogfood: if a Windows host is available, build the Windows
   executable and run `ws-mcp-windows-amd64.exe smoke --root <repo>` before
   relying on the GitHub Actions result.
6. Keep the Claude package untouched unless a separate Claude compatibility ship
   is requested.
