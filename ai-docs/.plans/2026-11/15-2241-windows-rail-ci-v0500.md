# Windows Rail CI v0.50.0 ship blocker

## Authority and scope

MAIN assigned one task at2026-11-15T13:41:21Z, ETA25minutes: repair five Windows-only TestRail failures from CI43715993555. Read `/tmp/ci-red-rail-windows-brief.md` in full, including safety rules, and the cited CI excerpt/log. Pool checkout closable-lurk-stubborn, branch fix/windows-rail-ci-v0500, owned key reverse-wrist-upgrade, based on current develop (must be30a1e8b4e or newer). Ordinary Pi worker only, no fork. Commit with AI Context noting v0.50.0 ship blocker and compatibility T0. No push, merge, daemon swap, or CI dispatch. Keep checkout held after final; release ONLY after MAIN's merge report for this task.

## Investigation and repair

Inspect `agents-plugin-tool/internal/wsdashboard/rail_lifecycle_test.go`, `rail_worktree_admission_test.go`, and directly relevant rail implementation/fixtures. Five targets:
- TestRailFourStateLifecycle: expected rfold data-key /w/repo missing at95; likely path canonicalization assumption.
- TestRailAdmissionPersistenceAndAtomicBatch: reject prompt payload mismatch at59.
- TestRailAdmissionIdentityFailureAndPruning: fixture writing detected/.git denied at128.
- TestRailAdmissionRetriesInvalidationAfterWriteFailure: chmod fixture did not inject failure at222.
- TestRailAdmissionRecreatedUserAddKeepsAdmission: POST /rail/worktrees/add timeout at301.

For EACH derive source-backed root cause and decide portable test-fixture repair versus real product defect. Fix only necessary tests/fixtures and, if a behavior-preserving implementation bug is proven, the smallest implementation repair. Stop/report before any product behavior/protocol change. Do not delete, weaken, or skip tests on Windows or anywhere without MAIN's explicit approval. Read all path-applicable manuals before editing (shipped-surface-boundary and release-compatibility for internal source; skill-authoring only if relevant conventions/docs change). Keep assertions meaningfully equivalent; portable failure injection must actually exercise durable write failure and unchanged recovery behavior.

## Verification

On this checkout's agents-plugin-tool run ONLY `go test ./internal/wsdashboard -count=1 -run 'TestRail'` on macOS and Windows. Never full wsdashboard or mcp package. Every Go shell invocation explicitly absolute cd into OWN agents-plugin-tool, including the unique copied Windows checkout, and GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local; use isolated HOME/config/runtime fixtures and existing warm caches. No downloads. Never outer-kill a Go test; use Go's own bounded timeout if appropriate and record actual completion. Lead does not rerun your tests.

Windows host ssh ki608@192.168.33.6. Use a NEW unique per-run folder (copy or clone the fixed head). Never touch `C:\Users\ki608\devenv`, default `%APPDATA%\Werkschaft`, or previous task roots. No image-name kills. Signals only recorded owned PID plus matching CIM creation ticks; NEVER signal2640. No shared daemon/provider/runtime/config changes, no witness holder, no foreign keys/signals. Never launch Electron on Mac (no Electron needed at all). Protect existing daemon PID2640 CIM639302110386172090; Get-Process ticks639302110386172097 is known precision artifact, CIM authority. Optional introspection probe failures are observations to skip; actual tests/build/native/safety failures require evidence-based bounded handling within the diagnosed tests, no blind retries or broad scope expansion.

## Deliverable

Committed fixed head, each test's root cause and verdict with file:line, complete actual scoped test output including Windows/macOS `ok` lines on the final head, command/isolation evidence, unchanged test inventory/no skips, safety/owned-cleanup statement. Store large evidence externally under a fresh `/tmp` task root. Push only material blocker, head-ready, or ETA slip beyond promised deadline. Report concise complete English, VERIFIED/INFERRED labels and citations. No ordinary acknowledgements. No next task until MAIN assigns. Keep checkout held until MAIN confirms this head merged; lead owns release.
