# E3 coverage and evidence tidy-up

## Authority and boundary
MAIN assigned range-review E3 as one bounded test/docs/comments task under owner grant (1170)=a. Base is exact develop 619237f2331c3dcd95a76d34e5da102dc8612ba5. No product behavior changes; T0. Existing panel-first branch/worktree at 1b7e7cd65 is retained separately and must remain untouched.

## Deliverables
1. Add a wslive symlink guard regression holding a same-model unique record older than the most recent eight; pass on Darwin and skip on Windows only if symlink creation is unavailable. Verify the test fails when c.regular && is temporarily removed, then restore production bytes.
2. Change the two remaining dashboard CSS comments from agents panel to agents popover; grep the dashboard tree for zero remaining agents panel occurrences.
3. Fix the ws-mcp manual's stale send-loop citation using a verified function-name reference resilient to EXEC2's current sse.go consolidation.
4. Sanitize public retention-ticket evidence: remove environment dumps/local worktree and captured session names; identify dangling /tmp and E references as not retained or remove them; remove stale held-lease README claim. Append a correction Edition for the removed final-copy-manifest.json and session-name redaction, preserving historical Result text except the specifically authorized captured session-name redaction. Preserve evidence substance and actual results, never fabricate retained artifacts.
5. Move 261116-bug-ws-worktree-acquire-stale-branch-facts from ready to todo through ws tickets.move, preserving its frozen completed phase and owner-1184-gated remaining phase.

## Verification and reporting
Read all path-matched repository manuals before edits and ticket conventions before ticket changes. Run whole internal/wslive offline/local in this owned checkout with isolated HOME/config/runtime/temp and warm caches; every Go shell invocation starts with explicit absolute module cd and GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local. Include the negative guard mutation result and restored source check. Verify ticket writes/move, grep, evidence links/redaction and git diff checks. No product code changes, other package suites, downloads, SSH/native Windows, shared daemon/runtime, providers, signals, branch integration or push. No independent reviewer for this test/docs/comment-only task. Commit explicit paths with AI Context, report HEAD READY and clean held checkout; MAIN integrates directly. Reports are concise complete English with VERIFIED/INFERRED and source citations. Original plan is write-once.
