# Plan: whiteboard Now slot takes a file path only

Branch `delegate/whiteboard-now-path-only` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`). Commit on
this branch; do not merge or push.

## Decision (user, 2026-11-07)

Remove the inline `now` field from the `whiteboard.update` payload and keep
only `now_path`. The user: "now쪽은 차라리 일반 now를 없애고 now_path만 남기는
방향으로". Reason: the Now slot is the largest and most often resent part of
whiteboard payloads. A lead resent a whole multi-kilobyte Now text to fix one
reference number, because inline `now` cannot be partially edited. With a file,
a lead edits the file with its host's edit tool and makes a short update call.

## Current state

- `agents-plugin-tool/internal/wswhiteboard/store.go` around lines 358-385:
  `now` and `now_path` are exclusive; `now_path` is read once via
  `env.ReadFile`; `now: ""` clears the slot (NowSeq reset).
- `updateFields` (store.go ~398) lists both fields.
- Tests: `internal/wswhiteboard/{store,render,regions}_test.go`,
  `internal/mcp/{whiteboard_tools_test,session_prune_test}.go` use `now`.
- Playbook text: `agents-plugin/rsrc/whiteboard/whiteboard.md`, mirrored in
  `agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md` and
  `agents-plugin-pi/rsrc/whiteboard/whiteboard.md` (currently identical).
  Find any other caller or text that sends or documents `now` (MCP tool
  description in `internal/mcp`, other playbooks, Pi bridge, dashboard code).

## Outcome

1. Payload: `now` is no longer accepted; a payload carrying it is refused
   like any unknown field, with a message that points at `now_path`.
   `now_path: ""` clears the Now slot (today's `now: ""` semantics).
   A non-empty `now_path` is read once at that update, as today; same
   full-document guard.
2. A stable per-session Now file: every `whiteboard.update` result also
   prints a `now_file:` line, a per-session path in a gitignored location
   (follow where the embed snapshots already live, `ai-docs/whiteboards/`,
   or the session's own state location if that is cleaner; it must be
   unique per session and must not contain the session key itself). The
   server does not create or delete the file's content beyond what is
   needed; the lead writes it, then passes it as `now_path`. Decide whether
   the session prune path should delete it with the board; prefer yes if it
   is a small change.
3. Playbook (all three copies identical, per
   `ai-docs/manuals/wsflow-mirroring.md`): replace the `now` description with
   the file flow: write the Now markdown to the `now_file` path with the host
   file tool, edit it in place for later changes, pass `now_path` on the
   update. Keep the existing HTML-in-Now rules. Follow
   `ai-docs/manuals/skill-authoring.md` and
   `ai-docs/manuals/shipped-surface-boundary.md` for the shipped text.
4. Tests updated and a test for: `now` refused, `now_path: ""` clears,
   `now_file` printed and stable across updates of one session, distinct
   across sessions.

## Verification

From `agents-plugin-tool/`: `go build ./...`, `go vet ./internal/... ./cmd/...`,
`go test -count=1 ./internal/wswhiteboard/ ./internal/mcp/`. Run the package
tests that check playbook mirror drift (agents-plugin-wsflow, agents-plugin-pi)
if they exist.

## Return

Changed files, the `now_file` location chosen and why, prune decision,
verification output, anything not checked.
