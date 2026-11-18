# Fix: launcher fails fast with a clear message when the daemon is another minor

## Context

Commit 302f38aac (merged on develop) makes the Werkschaft desktop app, the
plugin launcher and `daemon-start` all replace an older-minor shared daemon
automatically. Its worker found that since v0.50.0 the plugin launcher has no
direct-path fallback.

What a session sees today when its daemon is replaced by a newer minor:
- `DaemonLink.connect` raises "outside this plugin's minor" (about lines
  1761-1766 of `agents-plugin/bin/ws-mcp-launcher.py`).
- `proxy_serve` (about lines 2074-2081) runs `McpProxy` with no stream.
- `recover()` (about lines 1940-1962) re-probes `/health` every 3 s.
- Unsent MCP requests fail only after 75 s, and so do CLI calls
  (`wait_daemon_connection`, about line 1512).

So when a 0.51 starter replaces the daemon, a 0.50 session waits 75 s per
call before failing, and the user cannot tell why.

On 2026-11-18 at 09:50 the owner answered (1346)=b, which accepts silent
cross-minor replacement together with this mitigation. In the urgent 0.50.6
patch, the launcher must fail immediately with a clear "update the plugin"
error when the running daemon is a different (newer) minor. This prepares
for 0.51. The release is super urgent, so keep the change small.

This branch also carries the doc fixes from the release-gate review (range
af60e8895..302f38aac).

## Outcome

1. **Fail fast on a minor mismatch.** When the launcher finds a healthy daemon
   whose version is outside its minor, it does not wait for the 75 s
   deadline. That applies at initial connect, at `recover()`, and for CLI
   calls.
   - Each pending and new MCP request gets an immediate JSON-RPC error. CLI
     calls exit non-zero at once.
   - The message is clear, roughly: "The shared ws daemon is version X
     (newer minor); this plugin is Y. Update the ws plugin and restart this
     session." Keep it English and generic; it ships downstream.
   - Write one stderr line rather than one per 3-second probe.
   - Recovery still works: if a same-minor daemon comes back (for example a
     temporary swap), the session reconnects as it does today. A periodic
     re-probe may stay; only the per-request 75 s wait and the log spam go.
2. **Older daemon is not this case.** An older running daemon is
   (re)started/replaced by the launcher as today. Do not change it.
3. **Mirror.** Apply the identical change to every shipped copy of the
   launcher. `agents-plugin-wsflow` carries shared launcher copies; read
   `ai-docs/manuals/wsflow-mirroring.md`, and find the copies and the drift
   test with `git ls-files | grep ws-mcp-launcher`.
4. **Tests.** Add a launcher unit test for the fast-fail. Use the existing
   Python test harness for the launcher (find it next to the launcher or under
   the package tests). Keep the existing launcher tests green.
5. **Doc fixes** from the review sweep:
   - `ai-docs/tickets/ready/261115-feat-werkschaft-electron-daemon-override.md`.
     It still states the old cross-boundary contract:
     - "Cross-boundary replacement" announced default, reaffirmed as
       (1201)/(1202): confirmation, cancel attaches, fail closed when hello
       versions are unknown;
     - Phase 3 plan about lines 336-342;
     - Phase 3 Result about line 371 (Ensure refuses cross-minor);
     - T0/T3 framing about lines 54-55 and 225.
     Record the owner's 2026-11-18 reversal without editing text under a
     `### Result`:
     - add a dated Decisions entry. Owner (1343)=a at 09:15: always upgrade
       to the newest core. Owner at 09:27: replace across minors too. Owner
       (1346)=b at 09:50: launcher fail-fast. Auto-replace across minors is
       accepted; the confirmation dialog and the fail-closed rule are
       retired; never downgrade stays.
     - state the tier (T0 behaviour; the previous-minor lockout is the
       existing hello boundary; since 0.50 previous-minor sessions fail fast
       and must update the plugin and restart). Cite source lines.
     - append `#### Edition (<short-hash>) - 2026-11-18` under Phase 3 that
       points to commits 6b91aff4e and 302f38aac and this change.
     - Phase 4 acceptance should judge the new contract. Note that in the
       Decisions or Edition entry.
   - `AGENTS.md` around lines 121-125: rewrap the edited paragraph to the
     neighbouring width (about 79 characters). Split it into two sentences if
     it reads as a run-on. Keep the meaning, and add that the launcher now
     fails fast.
   - `ai-docs/manuals/release-compatibility.md`:
     - adjust the prose at about lines 27-31 to mention the fail-fast;
     - add one Scenario Sheet row (about lines 94-106) for "a newer-minor
       starter replaces a previous-minor daemon", with source lines.
   - `ai-docs/.plans/2026-11/18-0931-cross-minor-replace.md`: do not rewrite
     it. Append one line at the end: a correction note that the "falls back
     to the direct path" premise holds only for 0.48/0.49, with a pointer to
     commit 96253c58e.

## Constraints

- **Read first:**
  - `ai-docs/manuals/shipped-surface-boundary.md` (the launcher and its
    messages ship downstream);
  - `ai-docs/manuals/wsflow-mirroring.md`;
  - `ai-docs/manuals/release-compatibility.md`. The expected tier is T0: no
    wire change; only the timing and text of a failure that already happens.
    State it in the commit.
- **Scope:** the launcher copies and their tests, plus the docs above. No Go
  change and no desktop-electron change: another worker may touch
  desktop-electron.
- **No local runs on this PC** (antivirus rule). Do not run the launcher,
  ws/ws-mcp exes, the daemon, `go test` or any Go tool.
  - Python unit tests that import the launcher module also stay off this PC.
    Run them on w5090 (Linux; check it has python3; Go at `$HOME/sdk/go/bin`;
    no Node):
    `D:/sys_lock.sh remote w5090 run launcher-fastfail 20 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/guiding-dragonish-consumer' -- '<cmd>'`
  - Redirect the output to `D:/Temp/launcher-fastfail/`, run it with
    run_in_background, and wait for it to finish.
  - If the wsflow drift test is Go, run it there too.
  - The w5090 queue is shared with other workers. Be patient.
- **Urgency:** the owner is waiting to release. Aim to commit within about
  40 minutes. If something larger turns up, stop and report rather than
  expand the scope.
- **Temp files** go only under `D:/Temp/launcher-fastfail/`.
- **Process:** Simplest complete change. R1 review, then a fixes-only R2 only
  for substantive findings.
- **Commit:** Commit on `impl/develop/launcher-minor-fastfail` with your own
  model's Co-Authored-By trailer. Do not push.

## Report

- what changed (file:line), with the message text;
- the behaviour before → after (initial connect, mid-session replace, CLI);
- the tests and the w5090 result;
- the doc edits;
- the commit;
- anything left alone.
