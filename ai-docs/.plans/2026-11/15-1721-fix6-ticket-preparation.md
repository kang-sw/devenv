# FIX 6 ticket preparation

## Authority
MAIN's owner-approved triage (1060)=ok, relayed 2026-11-15 08:20:20 UTC, authorizes the six bounded fixes below. Prepare actionable ticket contracts only; root lead owns fact-population review, Sage gates, ready moves, commit and implementation dispatch.

## Scope
Read and prepare these idea tickets using their existing evidence and the current source:
1. 261115-bug-ws-worktree-acquire-concurrent-pool-claim: prevent simultaneous acquire calls from selecting/using one pooled checkout. Read duplicates as historical evidence only; do not edit terminal tickets.
2. 261008-bug-ws-git-merge-switches-worker-occupied-root: refuse merge checkout switches against a worker-occupied root.
3. 261030-bug-tickets-move-stages-on-occupied-root: prevent ticket move staging into a worker-owned index; reuse the guard from item 2. Item 2 must land first; serialize their implementation.
4. 261115-bug-ws-git-commit-cannot-conclude-merge: let the ws commit primitive conclude a conflicted merge without treating already-staged incoming merge paths as unrelated. Preserve safety against genuinely unrelated staging.
5. 261115-research-git-commit-empty-ticket-updates: only match the existing tool description: an explicit updated_tickets: [] suppresses inference; omission retains inference. No broader protocol/API semantic change. Report the research category's non-executable status and the smallest convention-following actionable derivation; do not create/rename/drop tickets yet. Item 4 must land before item 5 implementation.
6. 261029-bug-ws-mcp-git-lookup-per-spawn-slow-path-head: cache resolved git path rather than looking up PATH on each spawn.

## Contract
Read current AGENTS.md, ticket conventions, and applicable manuals, including current directional T0–T4 release compatibility rules. State source-backed compatibility tier in each Decisions section; target T0/bug-fix scope, never silently broaden into T3. Record confirmed owner scope, constraints, a bounded actionable phase, regression verification and applicable conventions. Preserve unique evidence, existing Decisions/Results/Editions and creation dates. No unapproved mechanisms or new product policy; return genuine choices as gaps. Draft item 5 only with owner-confirmed matching-description scope, preserving the research ledger/narrative and reporting required derivation.

Use edit for targeted changes; sequential ticket/Git index mutations only. Do not change source code, move status, stamp review, commit, acquire/release worktrees, use network/providers, install/reload/build daemons, push or signal processes. Source reads and bounded tests only if needed to verify an existing factual claim; no implementation. Return dirty paths, full concise contract summary per ticket, proposed item-5 actionable derivation, compatibility reasoning, decisions/gaps and evidence citations. Root will inspect diffs and run normal per-ticket fact population and readiness gates. Do not spawn children.
