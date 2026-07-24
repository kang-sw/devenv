# Plan: 260724-bug-windows-mcp-mid-session-disconnect — Phase 2: Launcher-side abnormal-exit diagnostics

## Relevant Ticket Contract

- On the Windows branch of the launcher, record the Go child's exit code/reason
  on abnormal (non-zero/signal) termination — the value `subprocess.call(args)`
  already returns and currently discards at `ws-mcp-launcher.py:868` — via a
  breadcrumb that **complements**, not overwrites, the startup-only
  `last-launch-error` path.
- Optionally redirect the Windows child's stderr to a timestamped runtime-dir
  file so a Go-side crash stack survives even before Phase 1 ships. Phase 1 has
  now landed (`agents-plugin-tool/internal/mcp/server.go`, always-on
  `<cache-root>/crash/mcp-panic.log`), so re-evaluate whether this is still
  worth it rather than building it by default.
- Must be cross-platform-safe: the POSIX `os.execvpe(...)` exec-replace path at
  `:869` is completely unaffected (exec never returns; this diagnostic applies
  only to the Windows `subprocess.call` branch).
- Launcher edits must be applied to `agents-plugin/bin/ws-mcp-launcher.py` and
  kept byte-identical with the `agents-plugin-wsflow` mirror (ticket
  Constraints section).
- Spec Impact (ticket-level): `plugin-runtime.md` should document the
  abnormal-child-exit breadcrumb as a "documented runtime-dir location"
  (mirrors how Phase 1 added `{#260724-serve-request-panic-resilience}` to
  `mcp-tools.md`).

## Out of Scope

- Phase 1 (server-side panic recovery + crash file) — already landed, see
  ticket `### Result (325f368f)`.
- Phase 3 (Windows Job Object / parent-death detection) and Phase 4 (SQLite
  point-read retry + WAL re-assert) — not touched.
- Optional Windows child stderr-to-file redirect — evaluated and **dropped**
  for this phase (see Implementation Plan step 2 rationale). Not implemented.
- Any change to the POSIX `os.execvpe` branch at `:869` — must stay byte-for-byte
  untouched.
- Broader mental-model rewrite — only the one stale sentence at line 75 is
  corrected, as an accuracy fix riding along with this edit.

## Codebase Findings

- `agents-plugin/bin/ws-mcp-launcher.py#L24-L37` — `write_launch_breadcrumb`:
  best-effort, writes `_BREADCRUMB_DIR / "last-launch-error"`. This is the
  existing breadcrumb the new one must complement, not collide with.
- `agents-plugin/bin/ws-mcp-launcher.py#L40-L46` — `clear_launch_breadcrumb`:
  unlinks `last-launch-error` (missing_ok). Called once, at `:855`, right
  before handoff — confirms `last-launch-error` only reflects the most recent
  *startup* failure and is unrelated to a later mid-session child exit.
- `agents-plugin/bin/ws-mcp-launcher.py#L49-L52` — `fail()`: the only caller of
  `write_launch_breadcrumb`; always raises `SystemExit(1)`. The new
  abnormal-exit breadcrumb must NOT go through `fail()` (the process should
  still return the child's real exit code, not force exit(1)).
- `agents-plugin/bin/ws-mcp-launcher.py#L816-L817` — `runtime_dir` resolved
  (`WS_MCP_RUNTIME_DIR` env override or `plugin_dir/.runtime/<platform>`), then
  `set_breadcrumb_dir(runtime_dir)`. This runs well before the handoff block, so
  `_BREADCRUMB_DIR` is already populated when `subprocess.call` executes at
  `:868` — no new dir-resolution plumbing needed.
- `agents-plugin/bin/ws-mcp-launcher.py#L866-L870` — the handoff block:
  ```
  866  args = [str(binary), *sys.argv[1:]]
  867  if os_name == "windows":
  868      return subprocess.call(args)
  869  os.execvpe(str(binary), args, os.environ)
  870  return 1
  ```
  Line 868 must change from a bare `return` to capturing the exit code, writing
  the breadcrumb when it is non-zero, then returning it. Lines 869-870 (POSIX)
  are untouched.
- `diff agents-plugin/bin/ws-mcp-launcher.py agents-plugin-wsflow/bin/ws-mcp-launcher.py`
  — verified zero differences right now (both 874 lines). Confirms the ticket's
  source-verified claim: the two launcher files are currently byte-identical,
  so this edit is a straight two-file mirror, not a divergence to reconcile.
- `ai-docs/mental-model/plugin-runtime.md#L75` — stale/false claim: "the wsflow
  launcher intentionally diverges from the canonical one — it is not the
  Windows shipping target and did not receive these fixes." This directly
  contradicts the verified byte-identical diff above and must be corrected as
  part of this change's doc pass (per explicit instruction).
- `agents-plugin-wsflow/tests/` — contains only
  `test_wsflow_runtime_contract.py` and `test_wsflow_skill_bundle.py`; there is
  no dedicated wsflow launcher test module. Regression coverage for the shared
  launcher lives solely in `agents-plugin/tests/`; the wsflow suite is run only
  to confirm nothing in the wsflow package breaks from the mirrored file
  change, per `ai-docs/mental-model/plugin-runtime.md`'s existing "Change
  copied plugin packaging or launcher behavior" recipe.
- `agents-plugin/tests/test_ws_mcp_launcher_capabilities.py#L390-L417` —
  `test_main_exports_plugin_roots_before_exec`: the established idiom for a
  full `main()` invocation test. It stubs `host_os`, `bootstrap_runtime_forced`,
  `local_devenv_runtime_enabled`, `runtime_install_forced`,
  `compatibility_stamp_current`, `set_breadcrumb_dir`/`clear_launch_breadcrumb`
  (no-op), `detect_project_root`, `wait_for_rsrc_tree`, `note`, then patches
  `os.execvpe` and runs against the real `plugin_dir` (`LAUNCHER_PATH.parent.parent`)
  with `os.environ` cleared. This is the template to extend for the new
  Windows-branch test — same stubs, but `host_os -> "windows"`, patch
  `subprocess.call` instead of `os.execvpe`, and — critically — do NOT stub
  `set_breadcrumb_dir`/`clear_launch_breadcrumb` (need the real breadcrumb
  write), instead pointing `WS_MCP_RUNTIME_DIR` at a temp directory so the test
  never writes into the real repo tree.
- `agents-plugin/tests/test_ws_mcp_launcher_coldload.py#L21-L29` — shared
  `load_launcher()` helper (importlib-based module load) used by both existing
  launcher test files; reuse it rather than adding a new loader.
- `ai-docs/spec/mcp-tools.md#L31-L45` — Phase 1's spec precedent: a new
  anchored paragraph (`{#260724-serve-request-panic-resilience}`) documents the
  always-on `<cache-root>/crash/mcp-panic.log` crash file. Phase 2's spec
  update should follow the same shape in `plugin-runtime.md`.
- `ai-docs/spec/plugin-runtime.md#L129-L184` — "Runtime Launcher Repair And
  Project-Root Detection" `{#260505-runtime-launcher-repair-project-root}` is
  the existing section describing launcher behavior before handoff; it does not
  currently document `last-launch-error` or any breadcrumb at all. The new
  abnormal-exit breadcrumb documentation is best added as its own short
  anchored paragraph near this section rather than folded into the existing
  anchor (keeps the existing anchor's meaning stable).

## Implementation Plan

1. **Add `write_exit_breadcrumb(exit_code: int) -> None`** in
   `agents-plugin/bin/ws-mcp-launcher.py`, placed after `clear_launch_breadcrumb`
   (after `:46`, before `fail` at `:49`), mirroring `write_launch_breadcrumb`'s
   best-effort try/except shape: guard on `_BREADCRUMB_DIR is None`, `mkdir(parents=True, exist_ok=True)`,
   write `_BREADCRUMB_DIR / "last-abnormal-exit"` with a timestamped line
   (`f"{time.strftime(...)} ws-mcp-launcher: child exited abnormally with code {exit_code}\n"`),
   swallow all exceptions. Naming `last-abnormal-exit` (not `last-launch-error`)
   is the complementing-not-overwriting mechanism the ticket requires — the two
   files answer different questions (startup failure vs. mid-session death) and
   coexist.
2. **Edit the Windows branch** at `:867-868`:
   ```python
   if os_name == "windows":
       exit_code = subprocess.call(args)
       if exit_code != 0:
           write_exit_breadcrumb(exit_code)
       return exit_code
   ```
   Leave `:869-870` (POSIX `os.execvpe` / unreachable `return 1`) byte-for-byte
   unchanged. A zero exit code (clean shutdown, e.g. stdin EOF per the ticket's
   Verified Findings) must NOT write a breadcrumb — only abnormal (non-zero)
   exits are recorded, matching the ticket's "on abnormal... termination"
   scope.
   - **Optional stderr redirect: dropped.** Phase 1 already persists the
     panic value + full stack for the confirmed root-cause trigger (unrecovered
     request-goroutine panic) to an always-on, unbounded-safe single-line-per-event
     file (`mcp-panic.log`). A raw stderr-to-file redirect for the entire
     long-lived stdio-inheriting Windows child would need its own
     growth/rotation management and would only add value for crash classes
     Phase 1 does not cover (e.g., a panic outside the recovered goroutine, or
     an OS-level kill) — none of which are the ticket's confirmed trigger. Given
     the exit-code breadcrumb from step 1 already flags "something abnormal
     happened, check the Phase-1 crash file first," building the stderr capture
     now is not justified; revisit in a later phase only if a real
     abnormal-exit breadcrumb with an *empty* Phase-1 crash file is observed.
3. **Mirror the exact same two edits** into
   `agents-plugin-wsflow/bin/ws-mcp-launcher.py` (same line positions, since the
   file is currently byte-identical) so the two files stay byte-identical.
4. **Add a regression test** to
   `agents-plugin/tests/test_ws_mcp_launcher_capabilities.py`, next to
   `test_main_exports_plugin_roots_before_exec`, following its full-`main()`
   invocation idiom:
   - `test_windows_abnormal_exit_writes_complementary_breadcrumb`: stub
     `host_os -> "windows"` (and the same no-op stubs as the existing test
     *except* `set_breadcrumb_dir`/`clear_launch_breadcrumb`, which must run for
     real), set `WS_MCP_RUNTIME_DIR` to a `tempfile.TemporaryDirectory()` path
     via `mock.patch.dict(launcher.os.environ, ...)`, patch
     `launcher.subprocess.call` to return a non-zero code (e.g. `3221225477`,
     Windows' unsigned view of `0xC0000005`), call `launcher.main()`, and assert:
     (a) `main()` returns that exact code; (b) `<runtime_dir>/last-abnormal-exit`
     exists and contains the code; (c) `<runtime_dir>/last-launch-error` does
     NOT exist (proves complement, not collision/overwrite).
   - `test_windows_clean_exit_does_not_write_breadcrumb`: same setup, patch
     `subprocess.call` to return `0`, assert `main()` returns `0` and
     `last-abnormal-exit` is absent.
   - Both tests reuse `load_launcher()` from the shared idiom (`test_ws_mcp_launcher_coldload.py:24-29` / already imported at the top of the capabilities test file).
5. **Update `ai-docs/spec/plugin-runtime.md`**: add a short new anchored
   paragraph near `{#260505-runtime-launcher-repair-project-root}` (after
   `:184`, before the "Release Asset Build And Checksum Pipeline" heading)
   documenting: startup failures write `last-launch-error` (cleared before every
   handoff); on Windows only, a non-zero child exit additionally writes
   `last-abnormal-exit` to the same runtime dir, recording the exit code, and
   does not disturb `last-launch-error`. Give it its own anchor, e.g.
   `{#260724-launcher-abnormal-exit-breadcrumb}`.
6. **Fix the stale claim** at `ai-docs/mental-model/plugin-runtime.md#L75`: the
   sentence claiming the wsflow launcher "intentionally diverges" and "did not
   receive these fixes" is false against the current byte-identical source;
   replace it with an accurate statement (the wsflow launcher is a maintained
   byte-identical mirror of the canonical one; both carry the same robustness
   fixes, verified by `diff`).

## Verification Plan

- `python3 -m unittest discover agents-plugin/tests` — run the full launcher
  test suite, including the two new tests.
- `python3 -m unittest discover agents-plugin-wsflow/tests` — confirm the
  wsflow package's own tests are unaffected by the mirrored launcher change
  (no dedicated launcher tests exist there, so this only guards the wsflow
  package-level tests).
- `diff agents-plugin/bin/ws-mcp-launcher.py agents-plugin-wsflow/bin/ws-mcp-launcher.py`
  must show zero differences after both files are edited — this is the
  explicit byte-identical-mirror check the ticket requires.
- Manual: re-read the final `:866-871` handoff block in both files side by side
  to confirm the POSIX `os.execvpe` line and its surrounding lines are
  unchanged (line-for-line) from the pre-edit version.

## Escalations

- None.
