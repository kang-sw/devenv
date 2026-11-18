# Fix: agents stop printing the whiteboard login token; retire the per-turn `Whiteboard:` line

## Context

Owner, 2026-11-18 11:14 KST, showing a screenshot of a lead session's chat that
ends with `Whiteboard: http://127.0.0.1:<port>/s/<board>?login=<token>`:
"Is the agent saying this a security leak? The `Whiteboard:` line itself is a
leftover from the past; let's clean it up."

Why it happens:
- `whiteboard.update` (and possibly other whiteboard or dashboard tool results)
  returns a `full:` line whose URL carries the `?login=` token, a credential
  for the session's dashboard page.
- The shipped `whiteboard` playbook's `#### Each turn` rule tells the lead to
  end the turn with `Whiteboard: <full>`, so leads copy the token into chat.
  Chat transcripts, screenshots and logs then hold a live credential.
- The workflow manual also carries a user-preference line, "Whiteboard: end
  each turn with the `Whiteboard:` line naming the full page path that
  whiteboard.update returns ...". Find where that text comes from. It may be a
  config value (user or machine config, or a manifest default).

The owner already watches the board in the Werkschaft app or a browser tab, so
the per-turn line carries no information they need.

## Outcome

1. **No token in agent-visible tool output.** Whiteboard tool results stop
   including `?login=<token>` in any URL: `full:`, `embed:`, refusal text,
   query output, and the workflow manual.
   - Keep a bare page URL only if something still needs it. Otherwise drop the
     line. Choose the simplest option and state why.
   - Before removing it, confirm the owner can still open a board without the
     token: the Werkschaft app, the dashboard home page or another existing
     login path. Report which path. If the token URL is the ONLY way to open a
     board in a fresh browser, STOP and report; do not invent a new login
     flow.
   - Grep for other agent-facing emitters of `login=`, `localhost-secret` or
     `lan-token` values (MCP tool results, playbooks, CLI output meant for
     agents) and report them. Fix them in the same change only if each is the
     same one-line kind of fix.
2. **Retire the per-turn `Whiteboard:` line.**
   - In the shipped `whiteboard` playbook (`agents-plugin/rsrc/...` and its
     wsflow mirror, per `ai-docs/manuals/wsflow-mirroring.md`), remove the
     rule that the lead ends each turn with `Whiteboard: <full> - (ids)`.
   - Keep the rest of `#### Each turn` (the watching-the-board conversation
     rule).
   - Fix any other shipped text that tells a lead to print that line: grep the
     skills, playbooks, conventions and the workflow manual template for
     `Whiteboard:`.
   - Keep the host-embed-folder behaviour unless it only exists to feed that
     line. Report what you decided.
   - If the user-preference line lives in a tracked config default, remove it.
     If it lives in the user's own config outside the repo, do NOT edit it.
     Report its exact location and key so the lead can tune it.
3. **Tests.** Update the Go tests and golden outputs that pin the `full:` line
   or the playbook text. Add one assertion that whiteboard update output
   contains no `login=`. Keep the wsflow mirror/drift tests green.

## Constraints

- **Read first:**
  - `ai-docs/manuals/shipped-surface-boundary.md`;
  - `ai-docs/manuals/skill-authoring.md`: the playbook edit; apply its
    checklist;
  - `ai-docs/manuals/wsflow-mirroring.md`;
  - `ai-docs/manuals/release-compatibility.md`. The expected tier is T0:
    tool-result text only, no schema, hello or store change. State the tier in
    the commit with source lines.
- **Not in release 0.50.6.** The release is cut at 94de8f53f. This branch
  merges to develop afterwards; do not touch the release.
- **No local runs on this PC** (antivirus rule): no `go test`, Go tools,
  gofmt, ws/ws-mcp exes, launcher, Electron, or a full desktop-electron npm
  test.
  - Go runs only on w5090:
    `D:/sys_lock.sh remote w5090 run wb-url 30 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/guiding-dragonish-consumer' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && gofmt -l <touched dirs> && go vet ./... && go test -count=1 <touched packages>'`
  - Redirect the output to `D:/Temp/wb-url/`, run it with run_in_background,
    and wait for it to finish.
  - Run only the touched packages. The w5090 queue is shared with the release
    tests, so be patient.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- **Never print, log or commit any real token value.** Use placeholders in
  tests and fixtures.
- **Temp files** go only under `D:/Temp/wb-url/`.
- **Process:** Simplest complete change. R1 review, then a fixes-only R2 only
  for substantive findings. If the scope grows (a new login flow, a schema
  change, many emitters), stop and report.
- **Commit:** Commit on `impl/develop/wb-url-no-token` with your own model's
  Co-Authored-By trailer. Do not push.

## Report

- every emitter found and what you did with it (file:line);
- how the owner opens a board without the token;
- the playbook and shipped-text edits;
- where the user-preference line lives;
- the tests and the w5090 result;
- the commit;
- anything left alone.
