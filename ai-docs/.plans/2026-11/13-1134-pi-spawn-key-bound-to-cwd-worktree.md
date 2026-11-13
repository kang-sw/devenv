# pi.spawn: bind the subagent's ws session key to its cwd's worktree

Idea ticket `261113-bug-ws-pi-spawn-key-bound-to-lead-root-not-cwd` (read it).
Owner 2026-11-13 asked for this fixed alongside the Pi holder work: a Pi
subagent spawned with `cwd` inside a pooled worktree gets a ws key that
resolves to the lead's root, so its ws git/ticket tools act on the root
(wrong branch, risk of committing to the root's branch).

## Do

1. Find where `pi.spawn` mints/assigns the subagent's delegate ws key and its
   bound root (`agents-plugin-tool/internal/wsagent/spawn.go`,
   `runner.go` ~1733-1803 start composition, the key minting used by
   `playbook.render` root_override / `worktree.acquire` worker keys, and
   `internal/mcp/pi_tools.go`). State the root cause.
2. Fix: the subagent's key is bound to the Git worktree root that contains
   `cwd` (resolve with `git rev-parse --show-toplevel` semantics, same as
   render's root_override binding). If `cwd` is not inside a Git worktree, or
   is the lead's own root, keep today's behaviour. Nested spawns (a Pi
   subagent's own `pi.spawn` with a different cwd) follow the same rule.
   Keep parent ownership, tier caps, depth limit and the capability token
   unchanged. Do not change the hosted-lead key rule.
3. Simplest correct change; no new tool arguments unless unavoidable
   (if one is needed it must be optional — T0).

## Out of scope

- Holder/daemon-restart work (separate ticket in flight on develop).
- Restarting the dashboard daemon (other sessions' Pi turns would die).

## Verification

- Go test on the remote w5090: spawn with cwd in a second worktree of a test
  repo -> the child's key resolves to that worktree (e.g. its git.status /
  root resolution reports the worktree's branch); cwd = lead root -> lead
  root as today; nested spawn into another worktree -> that worktree.
  `D:/sys_lock.sh remote w5090 run <who> <min> --sync <worktree> -- 'export
  PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test
  ./internal/wsagent/... ./internal/mcp/... -count=1'`.
- Commit in the worktree with NATIVE git (`git -C <worktree> ...`): your own
  ws key currently resolves to the lead root because of this very bug, so do
  not use ws git.* / tickets.* tools. `## AI Context` names the root cause
  and the compatibility tier; final line
  `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Temp files only
  under D:/Temp/<purpose>/, deleted when done.
