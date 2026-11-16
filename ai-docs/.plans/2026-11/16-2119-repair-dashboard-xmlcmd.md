# Repair: dashboard board width, hire readiness, rail resumption burst, Windows git metadata gate, patch backtick swallow (I7, I8, I9, I10, B1-wsdashboard-windows)

B1 (Windows, hub-first polling Phase 3, ce9366ed3): on windows-latest CI
- TestGitMetadataLinkedPointersRefsIndexAndWorktrees: git_metadata_test.go:50 "unchanged metadata is nondeterministic"
- TestRailMetadataSelectiveSafetyAndVisibleResumption: git_polling_gate_test.go:101 "changed metadata reread unrelated repositories"
If production code is at fault, the Windows metadata gate never settles and git reads are never gated, the exact perf path the owner is dogfooding on Windows. Find which metadata field is nondeterministic on Windows (e.g. a file identity, access time, or timestamp resolution read from `os.Stat`/`SameFileMetadata` CreationTime) and fix the gate so unchanged metadata compares equal on Windows; fix the tests only if they are wrong. You cannot run Windows: reason from code, add a unit test that injects the Windows-like stat values, and say what is unverified.

I9. `internal/wsdashboard/railworktrees.go` `watchWithRefresh(true)` (ce9366ed3): every `display=1` visibility resumption sets full+force and wakes the lister with no minimum gap, so rapid tab switching starts a forced, ungated full rail read (git processes) each time. Add a minimum gap/coalescing like `gitWatcher` has; resumption still forces a read, just not more than once per gap. Test.

I7. `internal/wsdashboard/assets/cards.js` `persist()`/`stored()` (~:694-715, cf0dcc9a0): board width is stored under `bb` on the live page and `bw` otherwise, under the same key `ws.dashboard.cards.<rid>`, and every save drops the other key. A page switching between live and board mode (pageview.go ~:39-41) loses the width, and a pin can lose it for good. Keep both values (save only the current mode's key, never delete the other except the one-time legacy migration). Node test.

I8. `internal/wsdashboard/index.go` `hierarchyReady` (~:231-237) with `session_hire.go` (~:306-309, 9da24fcbb): readiness needs `len(x.readErrors) == 0` across all sessions, and Refresh re-adds every unreadable registration each sweep. One unrelated corrupt `sessions/<id>.json` makes an interrupted, owner-approved hire spin every second until it expires after 30 min. Gate the replay on the requester's own entry (and its parent chain if the hire needs it), not on unrelated rows. Test with one corrupt unrelated registration.

I10. `internal/wsxmlcmd/wsxmlcmd.go` (~:790-797) and `literal_code.go` (~:164-179, 01d14e317): a stray panel-level backtick in a named-panel patch that carries `recap` is accepted as an inert code span ending inside a structural `<wb:change>` operand, and the panel is silently replaced by the raw patch markup (Accepted=1, no problem). Repro (after seeding `<wb:panel name="p" recap="r">alpha beta</wb:panel>`):
`<wb:topic name="t"><wb:panel name="p" recap="r2">` + "`" + ` <wb:change><wb:old>alpha ` + "`" + ` x</wb:old><wb:new>z</wb:new></wb:change></wb:panel></wb:topic>`
Lead technical decision: refuse an inert span that starts at panel level and ends inside a structural wrapper operand (a loud problem, not a silent swallow), keeping the settled owner-b boundary rule otherwise unchanged. Regression test through Apply plus the parser.

Verify: narrow `-run` for each touched test in internal/wsdashboard (never the whole package), `go test ./internal/wsxmlcmd` (whole package OK), focused internal/wswhiteboard tests touching patches, and the Node test for cards.js.

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
