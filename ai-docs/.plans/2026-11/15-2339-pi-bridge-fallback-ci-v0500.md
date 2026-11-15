# Hosted Pi bridge fallback Ubuntu CI v0.50.0 blocker

## Authority

MAIN assigned one task 2026-11-15T14:38:48Z, ETA30min (~15:09Z), progress due within15min (~14:54Z) if cause remains unknown. Read `/tmp/ci-red-round3-brief.md` fully and execute ONLY EXEC1 section. Base MUST be release cut a2093e47c, not moving develop. Owned full pool amused-earthy-tribune, key reorder-unlucky-wistful, branch fix/pi-bridge-fallback-ci-v0500. Ordinary Pi worker, no fork. Commit fix and unchanged plan with AI Context: v0.50.0 ship blocker/T0 unless broader compatibility needs discussion. No push, merge, CI dispatch or daemon swap. HOLD checkout until MAIN reports merge; lead owns release.

## Scope

`internal/mcp/TestPlaybookRenderHostedPiLeadSwitchesOnBridgeFallback` fails Ubuntu CI43724253758 at playbook_pi_mcp_only_test.go171: native Pi lead body lacks native SpawnIdiom ws-agent-spawn, ExploreAgent, ContinueIdiom. macOS isolated test passes. Investigate exact pinned source/test, detection/config/render dependencies and CI failure evidence `/tmp/ci-43724253758-failed.log`. Consider environment (bridge/Pi detection, HOME, PATH, config manifest), test order/shared state, and actual regression. Prior CI43721279531 on ffb038663 had no red; examine ffb038663..a2093e47c including 36ff36a40 assertion changes/980747660 mailbox owner changes but do not assume introduction without evidence.

Bounded likely paths: agents-plugin-tool/internal/mcp/playbook_pi_mcp_only_test.go and directly relevant render/detection/test helpers. Read applicable manuals before editing: shipped-surface-boundary, release-compatibility, ws-mcp; skill-authoring/wsflow-mirroring if playbooks or conventions become necessary. If changes reach the Pi harness implementation itself, read relevant installed Pi docs/examples in full before editing. Keep simplest portable fixture/bug fix. Product behavior/protocol changes STOP/report. Never delete/skip/weaken tests or merely increase timeouts. Do not work EXEC2 Windows/dashboard section.

## Verification

Never full internal/mcp or wsdashboard packages. Reproduce scoped isolated target and targeted interaction set `-run 'Playbook|Pi|Bridge|Hosted' -count=3` with shuffle to reveal ordering. Linux container if already available offline; no image pulls, dependency downloads, installs, providers or shared runtime. If Linux unavailable, reason from source and state limitation; do not present macOS or cross-compilation as Linux runtime proof. Final macOS -run ok lines on committed head plus Linux actual ok lines OR cited Linux-specific reasoning. Root cause file:line and introducing commit; if pure flake, report exact N/N host passes and uncertainty rather than inventing a fix.

Every Go shell invocation explicitly absolute cd OWN agents-plugin-tool and GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local, with isolated HOME/config/runtime/temp and warm existing caches. Nested test subprocesses inherit offline/isolation. Use Go's bounded timeout; never outer-kill Go. No full suites, daemon/provider/config changes, foreign keys/signals, forks, Electron, CI/push/merge/downloads. Windows is not required; if somehow needed, ask before expanding. Optional introspection probe failures are observations to skip; actual scoped failures should be diagnosed, not blindly repeated.

## Handoff

Evidence external fresh `/tmp` root: complete outputs read, commands/env/seed, scope and exact root cause/introduction/verdict/fix table, source hashes, final-head ok lines and Linux limitation. No broad CI-green claim. Material progress/blocker if cause unknown by14:54Z; final or ETA slip by15:09Z. Reports concise complete English with VERIFIED/INFERRED and file:line. Lead skims diff/report, does not rerun tests. Keep clean checkout HELD pending MAIN merge; never resume old worker after recycling; only MAIN assigns next task.
