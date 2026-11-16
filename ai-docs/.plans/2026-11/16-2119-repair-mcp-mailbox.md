# Repair: mailbox wait command, worktree release lock, hosted-lead mail delay, wsindex Windows test (I1, I11, I12, B1-wsindex)

I1. `agents-plugin-tool/internal/mcp/playbook_tools.go` (~:333, :340-345, b5b73efb5): `CLICommandNameFrom` accepts `WS_MCP_CLI_NAME` only when it matches `launcherCommandPattern`, else prints the literal placeholder `python3 <plugin>/bin/ws-mcp-launcher.py`. That rejects the declared shim names `ws-cli` / `wsflow-cli` (set in each package's `.claude-plugin/plugin.json`) and any launcher path containing `@` (Homebrew `python@3.x`). Claude sessions and Homebrew Codex/Pi now get a non-runnable mailbox-wait command and re-arm line. Accept the declared shim names and allow `@` (and other ordinary path characters) in the launcher form, while still never naming the runtime exe (the original ticket's intent). Fix `cmd/ws-mcp/mailbox_test.go:400`, which asserts the regression, and add cases for both shim names and an `@` path.

I11. `internal/mcp/worktree_tools.go` (~:840, bb2c53eec): release holds the pool-wide acquisition lock for its whole run (pre_release hooks, reset --hard, clean -ffdx, preserved-size measurement), so a concurrent acquire or another release gives up after the 2 s `worktreeAcquireLockTimeout` while a long hook runs; the error is also misprefixed "worktree.acquire: contention" inside a release. Keep the serialization the unleased-worktree race needs, but hold the lock only around the lease/state transitions, not around hooks and cleanup; fix the error prefix. Add a test where a slow release hook does not make a concurrent acquire of another worktree fail.

I12. `internal/mcp/lead_mail.go` (~:144) via `internal/wsmailbox/wait.go` (~:103, e99ef18a1 raised the backstop 10 s -> 5 min): after a failed claim (Drain or lockRow error) `LeadMailbox.watch` waits the 5-minute backstop to retry. The 5-minute value was scoped to the lost-notification backstop; a failed claim should retry on a short bounded backoff (about the old 10 s). Add a test.

B1 (Windows) `internal/wsindex` TestA9IndexDiscoveredAfterAbsenceTTL failed on windows-latest CI: "Read after TTL = {State:unknown ...}; want the new index". Determine whether it is a timing flake (e.g. TTL vs coarse timer) or a real defect, and fix the test or code. You cannot run Windows here: reason from the code, make the test robust to coarse clocks if that is the cause, and say what you could not verify.

Verify: narrow `-run` for each touched area in internal/mcp, cmd/ws-mcp `-run 'Mailbox'`, whole `./internal/wsmailbox` and `./internal/wsindex`.

## Common rules (all repair delegates)
- Source of findings: lead range review e93073373..b8e434d81 (release gate, verdict NEEDS FIX). Owner decision (1187)=a: fix the blocking set and the Important set before v0.50.2, then the lead re-reviews the same base.
- Go tests: always `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`. Never run the whole `./internal/wsdashboard` or `./internal/mcp` package; use narrow `-run`. Node is available for the Node-harness tests.
- Read the AGENTS.md `### Implementation Conventions` manuals that cover the paths you edit before editing.
- Do not touch `agents-plugin/rsrc/whiteboard/whiteboard.md` or its wsflow mirror (owner is editing them).
- Do not ssh anywhere, do not launch Electron or any GUI, do not restart or replace the shared dashboard daemon, do not run `scripts/dev.py rerun`.
- Mirrors stay in sync: the three `ws-mcp-launcher.py` copies are byte-identical; wsflow mirrors follow `ai-docs/manuals/wsflow-mirroring.md`.
- Any change to a cross-version surface states its compatibility tier in the commit `## AI Context` (expected T0; stop and report before anything above T2).
- The repository is public: no machine-local paths, hostnames, PIDs, or session keys in code, tests, commits, or this plan.
- Every behavior fix lands with a test that fails before the fix.
- Commit on your branch only; do not merge. If a fix turns out to need a product decision or grows beyond the listed files' area, stop and report instead of expanding.
- Return: commits, tests run with results, anything left unfixed and why.
