# Diagnose and fix wsagent CI hang

## Authority and target

MAIN assigned one ship-blocking regression for v0.50.0 from `/tmp/ci-red-wsagent-brief.md`. Use `fix/wsagent-ci-hang`, based on exact local develop commit `1a9ffdccf`, in this full pooled worktree. Target head/root-cause handoff is approximately 22:44 KST on 2026-11-15. Retain the lease until MAIN's merge report.

CI run 43715993555 on develop d87831f92 cancelled Ubuntu after 25 minutes with wsagent.test still alive; Windows reported wsagent failure after 1171.118 seconds. Supplied local logs are `/tmp/ci-132817535359.log` and `/tmp/ci-132817535299.log`. Last green run was 43362574058; establish its SHA from available local evidence or report that specific evidence gap.

## Bounded execution

1. Read applicable manuals and relevant package/tests. Reproduce with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsagent -count=1 -timeout 5m -v`; preserve exact before output and timeout panic stack if it hangs. Isolate from real HOME/provider state where tests might otherwise reach it, using explicit existing Go caches. Do not execute providers or make network requests.
2. Trace the hanging test and introducing commit through local source/history since the last green SHA. A Linux container may be used only if already available, without image downloads or installations; otherwise state the macOS/platform evidence boundary.
3. Apply the minimal code/test root-cause fix within wsagent. Never delete a test, raise timeouts to hide the issue, or skip without owner approval. Stop before any behavior/protocol change or broader scope.
4. Run post-fix package acceptance with count 1 and count 3, bounded by an explicit timeout, recording total time and exact ok lines. No whole-repository, internal/mcp, or internal/wsdashboard suite.

## Handoff and limits

Commit the unchanged plan and logical bug fix, with AI Context recording the v0.50.0 ship-blocker authority and T0 compatibility. Report exact clean head, root cause with file:line, introducing commit and supporting evidence, complete before/after/count-3 logs and commands, and platform gaps. MAIN alone merges, pushes, dispatches CI or operates the shared daemon. No network, providers, downloads, installations, credentials, unrelated signals/stashes/artifacts, push, CI dispatch, daemon swap/reload/build, or worktree release. Return at a clean checkpoint; stop before scope expansion.
