# Revalidate inherited Windows Go fixes

## Approved outcome

Resume the settled Windows Go fixes inherited from fix/windows-go at b572c2aac (425b68b15 process-tree cancellation, edf30f571 Windows file retry/callers, b572c2aac portability tests), validate against reconciled develop, and repair only concrete regressions within that contract. Owner authorized full delegation and parallel worktrees on 2026-11-14. Lead integrated develop d1c3b9d6 into the resumed copy as 89835f783; original branch/worktree remain intact.

## Scope

Files are limited to the existing delta from develop in agents-plugin-tool/internal/{gitbatch,mcp,wsdashboard,wsindex,wsledger,wsnote,wsstate,wstrackerhook,wstrackerhost,wswhiteboard}. Recover the exact settled A1-A6/C1-C3 findings from branch commits and the existing Windows findings artifact before changing anything. No new architecture, public API/protocol/store semantics, deletions or minor/version bump. Stop with evidence for any broader requirement.

## Verification

Read applicable AGENTS/manuals before edits. Run touched-package Go tests, internal/mcp with -timeout 25m, cmd/ws-mcp, and appropriate Windows compile/targeted native checks only in isolated temporary locations. Distinguish native Windows execution from cross-compile checks. Preserve logs and exact commands/results. Independent review must inspect the inherited delta plus any repair; do not merge or push. Report existing baseline failures honestly, never label a failing tree green-equivalent. Do not silently expand into unrelated historical Windows baseline failures.

## Safety and delivery

Use only this full-profile worktree and remembered fix/windows-go-resumed branch. Source builds/tests and isolated test servers are permitted. No installed/source dev launcher, scripts/dev.py rerun, shared daemon replacement/restart, live snapshot/cache/config mutation, release, remote integration, or worker-owned branch merge. Only lead integrates. Keep unrelated untracked files out of commits. Commit verified repairs with AI Context and this write-once plan; if no repair/commit is needed, report that fact and leave the plan for lead cleanup rather than manufacturing a source change. Return concise English complete-sentence bullets, VERIFIED/INFERRED labels, file:line citations, exact HEAD, commands/results, independent review findings, and remaining deployment/Windows acceptance boundary.
