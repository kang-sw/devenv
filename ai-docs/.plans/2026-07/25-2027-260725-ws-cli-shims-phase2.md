# Plan: 260725-feat-ws-cli-mcp-fallback-surface — Phase 2: bin/ shims, namespace env baking, and launcher warm-path trim

## Relevant Ticket Contract

- Add `bin/ws-cli` + `bin/ws-cli.cmd` to `agents-plugin/`, and `bin/wsflow-cli` +
  `bin/wsflow-cli.cmd` to `agents-plugin-wsflow/`, each delegating to its own
  plugin's launcher and exporting that plugin's environment
  (`WS_MCP_NO_AGENT`, `WS_MCP_NAMESPACE`, `WS_MCP_SETUP_TOOL` for wsflow)
  before doing so.
- Extend `skills_mirror.go` with the narrow `\bws-cli\b` -> `wsflow-cli`
  pattern; must not rewrite `ws-mcp` or `ws-plugin`.
- Make `import urllib.request` lazy in the launcher, keeping the two launcher
  copies byte-identical.
- Decision (ticket): "Ship a `bin/` shim, not a copied binary" — the shim
  delegates to the existing launcher, which already resolves the binary and
  forwards argv (`ws-mcp-launcher.py:883`, unchanged since Phase 1).
- Decision (ticket): "Shim names are namespace-scoped: `ws-cli` for ws,
  `wsflow-cli` for wsflow" — both plugins ship an identically-named
  `bin/ws-mcp-launcher.py`, so a shared shim name would resolve arbitrarily by
  `PATH` order.
- Decision (ticket): "Each shim bakes in its own environment" — a
  `PATH`-invoked shim inherits none of `plugin.json`'s `mcpServers.env`, so
  wsflow's shim must set `WS_MCP_NO_AGENT=1`, `WS_MCP_NAMESPACE=wsflow`,
  `WS_MCP_SETUP_TOOL=setup` itself.
- Verification (local), from the ticket's Phase 2 text: both shims resolve as
  bare names on `PATH` from a POSIX shell; wsflow shim yields namespace
  `wsflow` + agentless profile + no mercenary tools in `wsflow-cli tools`
  output; mirror generation rewrites `ws-cli` to `wsflow-cli` while leaving
  `ws-mcp`/`ws-plugin` untouched; launcher warm path improves by ~22ms;
  `diff` between the two launcher copies stays empty.
- Verification (downstream dogfood, non-gating): bare-name resolution from
  `cmd`/PowerShell on real Windows — record later, do not block this phase.
- Constraint: launcher edits go to `agents-plugin/bin/ws-mcp-launcher.py` and
  must be mirrored byte-identical into `agents-plugin-wsflow/bin/ws-mcp-launcher.py`.
- Constraint: the fallback keeps the launcher's existing `python3`-on-`PATH`
  requirement (no change needed, just don't violate it).
- Depends on Phase 1 (landed, `6b1038b7`): `cmd/ws-mcp/main.go` already has
  `tools` / `tools <name>` / `call <name> '<json>'` subcommands that the
  shims front — no `main.go` change is in scope here.

## Out of Scope

- Phase 1 content (`tools`/`call` subcommands) — already landed per the
  ticket's `### Result (6b1038b7)` section.
- Phase 3 content: `mcp-server-repair` skill authoring, registering it in
  `substitutionMirroredSkills` / `skills_manifest_test.go` /
  `test_wsflow_skill_bundle.py` / `ai-docs/ref/wsflow-mirroring.md`, and the
  eight front-door pointer-line edits (`lead-discuss`, `lead-sprint`,
  `lead-revive`, `lead-proceed` x2 namespaces). None of that content exists
  yet and must not be added in this phase.
- Windows per-call latency measurement (explicitly out of scope per the
  ticket's Constraints — "no Windows test environment is available to this
  work").
- Spec reflection (`mcp-tools.md`, `plugin-runtime.md`) — the ticket's
  "Contract-first spec: no" applies; reflect at closeout, not per-phase.
- Migration-anchor scope: `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md`
  discusses the skill/subagent-entry-shim pivot (skills becoming thin
  `playbook.print` shims, recursive subagent dispatch). It does not mention
  `bin/` executables, launcher shims, or CLI transport at all — this phase's
  `bin/ws-cli` shim is an unrelated concept (a POSIX/cmd executable, not a
  skill) and the anchor imposes no constraint on it. Noted per the task's
  request to check; no conflict found.

## Codebase Findings

- `agents-plugin/bin/ws-mcp-launcher.py:1-13` — current imports, including
  `import urllib.request` at line 12 (top-level, unconditional).
- `agents-plugin/bin/ws-mcp-launcher.py:173-178` — `download_file()`, the only
  caller of `urllib.request.urlopen`; called only from `install_downloaded_runtime`
  (release-asset path) and `install_runtime`'s `bootstrap_url` branch
  (line ~643). Both are download/repair paths, never the warm/no-repair path.
  Moving `import urllib.request` inside this function makes the ~22ms import
  cost pay-per-use instead of always-on.
- `agents-plugin/bin/ws-mcp-launcher.py:824-890` — `main()`. Binary resolution:
  `runtime_dir / runtime_binary_name(...)` under `<plugin>/.runtime/<platform>/`.
  Argv passthrough is already generic: `args = [str(binary), *sys.argv[1:]]`
  (line 883); POSIX uses `os.execvpe(str(binary), args, os.environ)` (line 889),
  Windows uses `subprocess.call(args)` (line 885). No new argument plumbing
  needed — the shim only needs to invoke
  `python3 <plugin>/bin/ws-mcp-launcher.py <cli-args...>` and the launcher does
  the rest, including env passthrough via `os.environ` (so shim-exported env
  vars reach the binary automatically).
- `agents-plugin/bin/ws-mcp-launcher.py` vs
  `agents-plugin-wsflow/bin/ws-mcp-launcher.py` — confirmed byte-identical now
  (`diff` exit 0, 894 lines each). No automated test enforces this pairwise
  byte-identity (searched `agents-plugin/tests/`, `agents-plugin-wsflow/tests/`,
  no `filecmp`/`byte-identical` hits); the only prior ticket that dealt with
  this (`ai-docs/tickets/.done/260622-bug-wsflow-launcher-coldload-divergence.md`)
  explicitly noted "launchers are curated (not byte-identical, unlike the
  generated rsrc subtree)" and verified parity via a plain `diff` command, not
  a repo test. Treat "keep byte-identical" as a manual/local verification
  step (apply the identical edit to both files, then `diff` them), not
  something requiring new test infrastructure.
- `agents-plugin/.claude-plugin/plugin.json:10-21` — ws plugin's `mcpServers.ws`
  has **no** `env` block at all (full-ws mode uses code defaults).
- `agents-plugin-wsflow/.claude-plugin/plugin.json:10-27` — wsflow's
  `mcpServers.wsflow.env` is exactly
  `{"WS_MCP_NO_AGENT":"1","WS_MCP_NAMESPACE":"wsflow","WS_MCP_SETUP_TOOL":"setup"}`.
  This is the literal env baking target for `wsflow-cli`.
- `agents-plugin-tool/internal/mcp/server.go:118-119,444-459` —
  `envNoAgent = "WS_MCP_NO_AGENT"`, `envNamespace = "WS_MCP_NAMESPACE"`;
  `RuntimeNamespace()` defaults to `"ws"` when `WS_MCP_NAMESPACE` is unset, and
  `NoAgentMode()` defaults false when `WS_MCP_NO_AGENT` is unset. This confirms
  `ws-cli` needs **no** env baking (defaults already match full-ws behavior,
  consistent with ws's `plugin.json` having no `env` block) — only
  `wsflow-cli` needs the three exports.
- `ai-docs/ref/ws-mcp.md:106` — `WS_MCP_SETUP_TOOL` unset/empty defaults to
  `ws.setup`; wsflow overrides to `setup`.
- `agents-plugin-tool/cmd/ws-mcp/main.go:32,60,62` — `tools` and `call`
  subcommands already dispatch in the `os.Args[1]` switch (Phase 1, landed).
  Nothing to change here.
- `agents-plugin-tool/internal/wsrsrc/skills_mirror.go:9-18` — current
  substitution patterns: `wsColonPattern = \bws:`, `wsSlashPattern = \bws/`,
  both left-word-boundary anchored. `GenerateWsflowSkillBody` (lines 41-48)
  applies `guardSubstitutionEligible` first, then both patterns in sequence.
  `"ws-cli"` contains neither `:` nor `/` immediately after `ws`, so the
  existing patterns never touch it — the new `\bws-cli\b` pattern is additive
  and does not interact with them.
- `agents-plugin-tool/internal/wsrsrc/skills_mirror.go:24-33` —
  `disqualifyingTokens` includes the bare substring `"ws."`. Confirmed
  `"ws-cli"` does not contain the literal substring `"ws."` (hyphen, not
  period, follows `ws`), so text containing `ws-cli` is not disqualified by
  the existing guard. No guard change is needed for this phase.
- `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go:15-19` —
  `substitutionMirroredSkills = ["lead-goal-step", "lead-prefer-subagent",
  "lead-verify-discussion"]`. None of these three skill bodies currently
  reference `ws-cli` (that only happens in Phase 3's `mcp-server-repair`
  skill and the entry-point pointers, both out of scope here), so
  `TestWsflowSkillsMirrorUpToDate`'s real-file loop will not exercise the new
  pattern yet. Verification of the new pattern in this phase must use a
  synthetic fixture, following the existing style at lines 157-176
  (`TestSubstitutionMirrorRespectsWordBoundaries`) and lines 178-193
  (`TestSubstitutionGuardAcceptsNamespaceOnlyContent`).
- `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go` is confirmed as
  the pinning test file (per the render task's ask) — it owns both the
  curated list and the substitution behavior tests.
- No `.cmd` file exists anywhere in the live tree today (`find -iname "*.cmd"`
  returned nothing outside `.worktree/`) — the two `.cmd` shims are a new file
  type with no local precedent to match; Windows-side correctness of the
  `.cmd` syntax cannot be executed on this Linux dev machine and is
  necessarily a downstream/manual check (consistent with the ticket's
  Constraints section on unmeasured Windows behavior).
- `agents-plugin-tool/scripts/smoke-ws-mcp.sh:1-10` — repo's existing shell
  script pattern for self-locating scripts: `set -euo pipefail` and
  `TOOL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"`. The
  `bash`-specific `BASH_SOURCE` construct is not directly reusable in a
  git-bash-resolvable POSIX shim (ticket requires "POSIX shebang script,
  resolvable from git-bash"); use `dirname "$0"` instead, which works
  correctly here because the shim is invoked by a bare-name `PATH` lookup
  (the shell sets `$0` to the resolved full path in that case, not to the
  bare name), and the shim is a real file (not itself a symlink), so
  `readlink -f` resolution is unnecessary.
- `agents-plugin/.local-devenv-runtime` and
  `agents-plugin-wsflow/.local-devenv-runtime` are both present and valid
  local devenv contracts pointing at `agents-plugin-tool/` with `go` resolved
  to `/home/linuxbrew/.linuxbrew/bin/go`. A staged binary already exists at
  `agents-plugin/.runtime/linux-amd64/ws-mcp` (older; the launcher will
  rebuild it via the source-fingerprint check on first shim invocation). This
  means the "both shims resolve as bare names on PATH" and "wsflow shim
  yields namespace wsflow / agentless profile / no mercenary tools" local
  verifications are actually runnable end-to-end on this machine (temporarily
  add the `bin/` dirs to `PATH`, or invoke by relative path), not merely
  theoretical.
- Baseline confirmed green on this tree, with the concurrent session's 5
  files left exactly as-is: `cd agents-plugin-tool && go build ./...` (exit
  0), `go vet ./...` (exit 0), `go test ./internal/wsrsrc/... ./cmd/ws-mcp/...`
  (both `ok`, no other package touched).
- `install.sh:585` stages the whole `agents-plugin/` (and analogously
  `agents-plugin-wsflow/`) tree via
  `rsync -a --delete "$REPO_DIR/agents-plugin/" "$PLUGIN_CACHE/ws/"`
  (recursive, no per-file bin/ manifest) — new files under `bin/` are picked
  up automatically; no `install.sh` change is needed. No test enumerates
  `bin/` directory contents by name (searched `internal/wsrsrc/*_test.go` for
  `"bin"` — no hits), so adding two new files per plugin does not require
  touching a shipped-file-list test.

## Risk Signals

- **Executable bit.** The launcher is `-rwxr-xr-x`. `Write` does not set the
  executable bit on new files; the two new POSIX shims (`ws-cli`,
  `wsflow-cli`) must be `chmod +x`'d explicitly or they will not resolve as
  bare `PATH` commands on POSIX. The `.cmd` files do not need the POSIX
  executable bit (Windows resolves by extension).
- **Env-baking asymmetry is intentional, not a missed case.** `ws-cli` sets
  no environment variables at all — this matches `ws`'s `plugin.json` having
  no `env` block and `RuntimeNamespace()`/`NoAgentMode()` defaulting
  correctly. An implementer might over-apply the ticket's "baking that
  plugin's env" language to both shims uniformly and add spurious
  `WS_MCP_NAMESPACE=ws` / `WS_MCP_NO_AGENT=0` exports to `ws-cli`; the ticket
  text's "for wsflow" qualifier scopes all three env vars to the wsflow shim
  only. Keep `ws-cli` env-free.
- **Regex ordering is not actually a risk** — verified no collision is
  possible between `\bws-cli\b` and the existing `\bws:`/`\bws/` patterns
  (see Codebase Findings), so pattern-application order in
  `GenerateWsflowSkillBody` does not matter for this addition.

## Implementation Plan

1. **Lazy `urllib.request` import** in
   `agents-plugin/bin/ws-mcp-launcher.py`: remove the top-level
   `import urllib.request` (line 12), add `import urllib.request` as the
   first line inside `download_file()` (currently lines 173-178), immediately
   before the `try:`. Apply the byte-identical edit to
   `agents-plugin-wsflow/bin/ws-mcp-launcher.py` (same line numbers, since the
   files are currently identical). Verify with
   `diff agents-plugin/bin/ws-mcp-launcher.py agents-plugin-wsflow/bin/ws-mcp-launcher.py`
   (must print nothing / exit 0).

2. **`agents-plugin/bin/ws-cli`** (new file, POSIX shell, no shebang
   surprises for git-bash):
   ```sh
   #!/bin/sh
   # ws-cli: MCP-independent CLI fallback for the ws plugin.
   # Delegates to the plugin's launcher, forwarding all arguments unchanged.
   set -e
   script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
   exec python3 "$script_dir/ws-mcp-launcher.py" "$@"
   ```
   `chmod +x agents-plugin/bin/ws-cli` after writing.

3. **`agents-plugin/bin/ws-cli.cmd`** (new file, `cmd`/PowerShell):
   ```bat
   @echo off
   python3 "%~dp0ws-mcp-launcher.py" %*
   ```
   No executable bit needed.

4. **`agents-plugin-wsflow/bin/wsflow-cli`** (new file), identical shape to
   step 2 but with the three env exports baked in before delegating:
   ```sh
   #!/bin/sh
   # wsflow-cli: MCP-independent CLI fallback for the wsflow plugin.
   # Bakes wsflow's agentless environment before delegating to the launcher.
   set -e
   export WS_MCP_NO_AGENT=1
   export WS_MCP_NAMESPACE=wsflow
   export WS_MCP_SETUP_TOOL=setup
   script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
   exec python3 "$script_dir/ws-mcp-launcher.py" "$@"
   ```
   `chmod +x agents-plugin-wsflow/bin/wsflow-cli` after writing.

5. **`agents-plugin-wsflow/bin/wsflow-cli.cmd`** (new file):
   ```bat
   @echo off
   set WS_MCP_NO_AGENT=1
   set WS_MCP_NAMESPACE=wsflow
   set WS_MCP_SETUP_TOOL=setup
   python3 "%~dp0ws-mcp-launcher.py" %*
   ```

6. **`agents-plugin-tool/internal/wsrsrc/skills_mirror.go`**: add a third
   pattern next to `wsColonPattern`/`wsSlashPattern` (lines 15-18):
   ```go
   wsCliPattern = regexp.MustCompile(`\bws-cli\b`)
   ```
   and apply it in `GenerateWsflowSkillBody` (after the existing two
   `ReplaceAllString` calls, lines 45-46):
   ```go
   out = wsCliPattern.ReplaceAllString(out, "wsflow-cli")
   ```
   Add a one-line doc comment update alongside the existing pattern comment
   (lines 9-14) noting the new pattern is a literal-token substitution (not a
   namespace-prefix rule like the other two) and must not be broadened to
   match `ws-mcp` or `ws-plugin`.

7. **`agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go`**: add a new
   test function (near `TestSubstitutionMirrorRespectsWordBoundaries`,
   lines 157-176) proving the new pattern fires on `ws-cli` and leaves
   `ws-mcp`/`ws-plugin` untouched, e.g.:
   ```go
   func TestWsCliSubstitutionPattern(t *testing.T) {
       source := "---\nname: fixture\n---\n\n" +
           "Run ws-cli tools, never ws-mcp directly, and do not touch ws-plugin.\n"
       out, err := GenerateWsflowSkillBody(source)
       if err != nil {
           t.Fatalf("expected guard to accept fixture, got error: %v", err)
       }
       want := "---\nname: fixture\n---\n\n" +
           "Run wsflow-cli tools, never ws-mcp directly, and do not touch ws-plugin.\n"
       if out != want {
           t.Fatalf("ws-cli substitution mismatch:\ngot:  %q\nwant: %q", out, want)
       }
   }
   ```
   This is additive to the existing test file; do not modify
   `substitutionMirroredSkills` (that curated list only changes in Phase 3
   when `mcp-server-repair` is registered) or any other existing test in this
   file.

## Verification Plan

- `cd agents-plugin-tool && go build ./... && go vet ./...` — must stay
  clean (baseline already confirmed clean before this phase's edits).
- `go test ./internal/wsrsrc/...` — must stay green, including the new
  `TestWsCliSubstitutionPattern` and the pre-existing
  `TestWsflowSkillsMirrorUpToDate` (which is unaffected since none of the
  three curated skills mention `ws-cli` yet).
- `diff agents-plugin/bin/ws-mcp-launcher.py agents-plugin-wsflow/bin/ws-mcp-launcher.py`
  — must print nothing (exit 0), proving the lazy-import edit was mirrored
  byte-identically.
- Local shim resolution (POSIX): `chmod +x` the two new POSIX shims, then run
  e.g. `PATH="$PWD/agents-plugin/bin:$PATH" ws-cli tools` and
  `PATH="$PWD/agents-plugin-wsflow/bin:$PATH" wsflow-cli tools` from repo
  root — both should resolve as bare names and print the mapping rule plus a
  tool list (the local devenv runtime contract is active and will
  rebuild/repair the staged binary as needed; first call may be slower due to
  a `go build` repair pass, matching documented launcher behavior).
- Namespace/profile check: confirm `wsflow-cli tools` output's tool list
  omits any `mercenary`-named tool (agentless profile) and that behavior
  differs from `ws-cli tools` (full profile) — this exercises the baked
  `WS_MCP_NO_AGENT=1`/`WS_MCP_NAMESPACE=wsflow` env actually reaching the
  server process through the shim -> launcher -> `os.environ` ->
  `os.execvpe`/`subprocess.call` chain.
- `.cmd` files: syntax-review only on this Linux machine (cannot execute
  `.cmd`); functional Windows verification is downstream dogfood per the
  ticket's Constraints, not a local gate for this phase.
- Sanity re-run after all edits: `go test ./internal/wsrsrc/... ./cmd/ws-mcp/...`
  should still show `ok` for both packages, confirming no regression from the
  launcher/skills_mirror edits and that the concurrent session's untouched 5
  files (`agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md`,
  `agents-plugin-wsflow/rsrc/lead-write-ticket/lead-write-ticket.md`,
  `agents-plugin/rsrc/manifest.json`, `agents-plugin-wsflow/rsrc/manifest.json`,
  `ai-docs/_index.md`) remain consistent with a green tree.

## Escalations

- None.

---

**Environment note for the implementer (do not skip):** a concurrent session
holds *uncommitted* changes to exactly these 5 files, verified via `git status`
during this survey:
`agents-plugin-wsflow/rsrc/lead-write-ticket/lead-write-ticket.md`,
`agents-plugin-wsflow/rsrc/manifest.json`,
`agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md`,
`agents-plugin/rsrc/manifest.json`,
`ai-docs/_index.md`. None of them overlap this phase's file set (`bin/*`,
the two launcher `.py` files, `skills_mirror.go`, `skills_mirror_test.go`) —
disjointness confirmed. Do **not** read-for-edit, edit, stage, or commit any
of those 5 files, even incidentally (e.g. do not run a staging command that
would sweep them in). When committing, stage this phase's files by explicit
pathspec only — never `git add -A` / `git add .`. `go test ./internal/wsrsrc/...`
runs against a tree where those 5 files are already modified-but-consistent;
it was confirmed green in that state before this plan was written, and should
stay green after this phase's changes.
