# Brief: 260620-chore-pre-shipping-windows-surface-verification (Phase 1)

## Intent

Phase 1 (P0) of the pre-shipping Windows-surface hardening. Add a cross-platform
behavioral test proving that cancelling an agent/exec job terminates the **entire
spawned process tree** (not only the root PID), and fix the Windows cancel paths
to reap the child tree. On Unix the process-group kill already does this
intentionally; Windows currently kills only the root PID, orphaning
runner-spawned children. Verification boundary for THIS phase: the new test is
green on Linux and the Windows kill path is structurally exercised (the real
Windows run is Phase 3).

## Scope Boundary

In scope (Phase 1 only):

- New cross-platform behavioral test: a parent process spawns a child that blocks
  on a sentinel; drive the cancel path; assert the child is reaped. Table-driven
  on `runtime.GOOS` where the spawn helper differs.
- Fix Windows process-tree termination in BOTH cancel paths:
  - wsagent mercenary cancel: `cancelAsyncProcessTree` (`internal/wsagent/cancel_process_windows.go`)
    and any helper in `process_windows.go` / `async_command_windows.go` /
    `runner_command_windows.go`.
  - execjob exec abort: `internal/execjob/process_windows.go` `cancelProcess`.
- Review (cover or document) the Windows `processAlive` zombie / exited-handle
  issue (`internal/wsstate/process_alive_windows.go`).

Deferred / excluded:

- Phase 2 (stabilize `260616` flaky abort), Phase 3 (full Windows suite run),
  Phase 4 (worktree path-layout). Do NOT touch later-phase surfaces.
- Do not run the Windows kill path on the live host as part of this phase —
  Phase 1 verifies on Linux only.

## Caller-Visible Contract

No new caller-visible contract. BOTH cancel paths already have best-effort
contracts the fix conforms to without expanding:

- mercenary cancel — `#260505-agent-cancel-recovery`: "best-effort local process
  cancellation for the stored worker pid" plus a `cleanup_needed` signal.
- exec abort — `#260524-exec-job-mcp-tools` (`spec/mcp-tools.md`): `exec.abort`
  "best-effort terminates a running job while preserving partial output and
  terminal state metadata". Platform-specific abort mechanics are unspecified
  there, so the Windows tree-reap is an implementation detail under the same
  best-effort umbrella.

The fix STRENGTHENS Windows best-effort coverage (reap the spawned child tree)
without elevating the promise beyond best-effort and without changing either tool
surface or return shape. `cleanup_needed` remains the mercenary signal when
reaping is incomplete.

## Contract Instructions

- `internal/wsagent/cancel_process_windows.go` — `cancelAsyncProcessTree(pid)`:
  replace `os.FindProcess(pid).Kill()` (root-only) with a subtree-scoped kill of
  the whole tree rooted at `pid`.
- `internal/execjob/process_windows.go` — `cancelProcess`: same subtree-scoped
  termination.
- Mirror the INTENT of the Unix counterpart (`syscall.Kill(-pid, SIGKILL)`
  process-group kill). The survey maps the exact Unix file/function and the
  runner's process-group setup (likely `Setpgid` on `SysProcAttr`) so the Windows
  side spawns and kills with a matching tree boundary.
- The runner/exec spawn site may need to create the child in a killable group on
  Windows (e.g. `CREATE_NEW_PROCESS_GROUP`, or assign to a job object at spawn) so
  the cancel path can reap the subtree. If a spawn-side change is required, keep
  it minimal and platform-guarded (`*_windows.go`).
- Mechanism: pick the simplest that reliably reaps runner-spawned children.
  Evaluate `CREATE_NEW_PROCESS_GROUP` + `GenerateConsoleCtrlEvent`,
  `taskkill /T /PID <root>`, or a Windows job object. Record rejected
  alternatives in the completion report.
- Do not invent a new cancel entry point — modify the existing
  `cancelAsyncProcessTree` / `cancelProcess` in place; reuse the Unix path's
  structure and the runner `SysProcAttr` setup as the model.

Forbidden wiring:

- **HARD CONSTRAINT (live-host safety):** the kill MUST be scoped to the spawned
  root's subtree by PID or job object. Image-name termination (`taskkill /IM`,
  process-name sweeps) is FORBIDDEN — the dogfooding WSL2 host runs a live
  `claude.exe` that must never be reached.
- No temporary / mock kill; no broad process sweeps.

## Integration Test Instructions

- Boundary type: behavioral process-lifecycle test (spawns real OS processes).
- New test file: cross-platform test in `internal/wsagent` (and/or
  `internal/execjob` if the abort path is independently testable). Prefer a
  shared helper that spawns a parent which spawns a child, both blocking on a
  sentinel (a temp file or a long sleep), table-driven on `runtime.GOOS` for the
  spawn idiom.
- Deterministic (no sleep-races): spawn child blocking on a sentinel; cancel;
  then poll-with-timeout that the child PID is no longer alive (reaped) with a
  bounded deadline — never a fixed `time.Sleep`.
- Pass criteria: after cancel, BOTH parent and child are terminated. On Linux
  this must be green now. The Windows branch must compile and be structured to
  run under a Windows host in Phase 3.
- Run: `go test ./internal/wsagent/... ./internal/execjob/...` (confirm exact
  packages during survey).

## Implementation Strategy Decisions

- Mirror the Unix process-group intent on Windows rather than inventing a
  divergent model.
- Keep the cancel contract best-effort; surface incomplete reaping via the
  existing `cleanup_needed` path rather than promising guaranteed termination.
- Scope kills to the spawned subtree (PID/job), never image name.

## Rejected Alternatives

- Image-name kill (`taskkill /IM`) — forbidden (live `claude.exe` risk).
- Elevating the contract to guaranteed tree termination — out of scope;
  best-effort + `cleanup_needed` is retained.

## Approach

- Survey maps: the Unix cancel function + runner `Setpgid` spawn; execjob
  `cancelProcess` + its spawn; the `processAlive` Windows implementation.
- Add the test helper + table-driven test.
- Implement Windows subtree kill in both paths; add spawn-side group/job creation
  only if required to make the subtree killable.
- Review/document the `processAlive` zombie-handle issue (cover with a guard or a
  documented note per the survey finding).
- Run `go test` on Linux; read full output.

## Constraints

- Live-host safety hard constraint (above): PID/job-scoped kills only.
- Deterministic test (no sleep-races).
- No new caller-visible contract; best-effort + `cleanup_needed` retained.
- Platform-specific code stays in `*_windows.go` / Unix-tagged files; shared test
  logic is `runtime.GOOS`-table-driven.
- Exec-surface audit rule (`mental-model/mcp-runtime.md` Extension Points): keep
  launch/status/result/abort/raw readers together; lifecycle/path/byte-count
  metadata stays in `wsstore.ExecJob`, stdout/stderr/combined bytes stay in
  job-owned files. The cancel fix must not migrate stream payloads into SQLite.

## Out of scope

- Phases 2–4; running the Windows suite on the live host.

## Details

<!-- Filled by the survey plan: exact Unix cancel symbol/file, runner SysProcAttr
process-group setup, execjob spawn/cancel symbols, processAlive Windows impl. -->

## Verification Contract

- `go test ./internal/wsagent/... ./internal/execjob/...` green on Linux (full
  output read, no introduced warnings).
- The new test asserts the child is reaped after cancel.
- The Windows kill code compiles under build tags, is scoped to the spawned
  subtree, and contains no image-name kill.

## References

<!-- [Must]: read before starting. [Maybe]: consult if uncertain. -->
- [Must] `ai-docs/mental-model/named-agent-runtime.md` — cancel intent (line 75:
  Unix process-group kill is intentional; `#260505-agent-cancel-recovery`) and
  Windows liveness debt (line 82: weaker than Unix, can keep dead calls active).
- [Must] `ai-docs/spec/named-agent-runtime.md` (`#260505-agent-cancel-recovery`) —
  best-effort + `cleanup_needed` contract; do not introduce a new contract.
- [Must] `ai-docs/spec/mcp-tools.md` (`#260524-exec-job-mcp-tools`, lines 830–898) —
  `exec.abort` best-effort contract for the execjob path; platform abort mechanics
  unspecified (so the Windows fix is an impl detail under best-effort).
- [Must] `ai-docs/mental-model/mcp-runtime.md` (Extension Points + Common Mistakes,
  lines 77–106) — exec-surface change audit rule; do not migrate stream payloads
  into SQLite.
- [Must] `ai-docs/ref/ws-agent-runtime.md` (lines 385–391) — documents the
  pre-fix limitation ("does not yet provide backend-specific process-group
  cleanup") this work addresses.
- [Must] `ai-docs/tickets/ready/260620-chore-pre-shipping-windows-surface-verification.md` —
  Phase 1 scope, constraints (live-host safety), kill-mechanism guard.
- [Maybe] `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` —
  mercenary lifecycle context (already read by lead).
