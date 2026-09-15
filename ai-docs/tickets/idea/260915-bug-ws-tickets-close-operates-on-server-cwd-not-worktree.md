---
title: "INVESTIGATION: a worktree batch worker's tickets.close appeared to touch the main checkout — server-cwd hypothesis REFUTED, real cause unconfirmed"
related:
  260915-feat-ws-worktree-pool-default-out-of-tree: same worktree-run dogfood session
---

# Investigation: worktree worker's `tickets.close` appeared to touch the main checkout

> **Status correction (2026-09-15, code-verified).** The original hypothesis
> in this ticket's stem — "`tickets.close` operates on the MCP server cwd, not
> the caller's worktree" — is **refuted by the source**. See *Code findings*.
> The observation below is real; the mechanism is not what was first reported.
> This is now an investigation ticket, not a confirmed contract bug. The stem
> is kept for continuity; do not treat its wording as the conclusion.

## Observed (worker report, unverified)

During a parallel `ws:lead-run` batch with per-ticket isolated worktrees, the
`260914-feat-ws-pi-mailbox-native-steer-push` worker (session bound to
`.ws-worktrees/uptown-swagger-schnapps`) reported that after it called
`ws/tickets.close`, a `ready → .done` rename plus a `completed:` frontmatter
edit appeared **staged in the lead's MAIN checkout**
(`/Users/kang-sw/devenv`, branch `goal/develop/hazel-quartz-meadow`), not in
its own worktree. The worker reverted that stray staged change in the main
checkout and re-did the close manually on its impl branch. The lead
independently confirmed the main checkout was clean afterward.

## Code findings (2026-09-15, verified against source)

- `resolveToolRoot` (`agents-plugin-tool/internal/mcp/server.go:2839`) resolves
  the working root **authoritatively from the `session_key`'s bound
  `entry.root`**. There is **no server-cwd / `os.Getwd` fallback**: with no
  key it returns `mandatory_session_key`; with an unknown key,
  `unknown_session`. `tickets.close`, `tickets.move`, `tickets.query`,
  `tickets.create_empty` all resolve root through this one helper.
- `playbook.render(root_override=<worktree>)` binds the render-minted child
  session key to the worktree: the dispatch sets `mintRoot = rootOverride`
  (server.go render case) and `renderPlaybookBody` mints the child key against
  `mintRoot` (`playbook_tools.go:727-741`).
- `worktree.acquire` likewise mints a worktree-bound `worker_key`.

So **a `tickets.close` call carrying any worktree-bound key must resolve to the
worktree root**, and the "server cwd" path the report names does not exist.

## Open questions (the actual investigation)

- **Which session key did the worker actually pass** to `tickets.close`? If it
  used a key whose `entry.root` was the main checkout (e.g. a lead/parent key,
  or a mis-bound key), the close correctly wrote to main — a worker-key-usage
  problem, not a `tickets.close` problem.
- **Was the "staged change in main" a real ws-tool write, or a worker
  observation artifact** — e.g. the worker ran `git status` from a shell whose
  cwd was the main checkout while its ws key pointed at the worktree, conflating
  the two trees?
- If a worker *can* end up holding a main-bound key while operating in a
  worktree, that is the real defect surface — in session minting / key handoff,
  not in `tickets.close`.

## Candidate follow-ups (only after the cause is pinned)

- Reproduce with explicit key logging to identify the resolved `entry.root`.
- Consider a guard: warn when a mutating tickets call's resolved `entry.root`
  differs from the git worktree the caller's shell is in (defense in depth).
- Separately (workflow, not a bug): decide whether workers should call
  `tickets.close`/`move` at all, or whether board transitions are lead-owned
  post-merge.

## Notes

Captured under the "Dogfood surprises get captured" discipline. The original
server-cwd framing was a worker-model diagnosis (no user authority) and did not
survive code verification. Sibling same-session tickets:
`260915-bug-ws-ticket-facts-new-public-symbol-likely-enum`,
`260915-bug-ws-route-resolve-implement-branch-handling-random-codename`.
