---
related:
  261108-feat-ws-tracker-ledger-attachments: "the run where the delegate symptom was observed"
title: "ws/git.commit resolves paths against the main checkout for a delegate's session"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 398e6a37489a43dd
sage-review-completeness-reviewed: 398e6a37489a43dd
completed: 2026-11-10
---

# ws/git.commit resolves paths against the main checkout for a delegate's session

## Background

Dogfood surprise from running `261108-feat-ws-tracker-ledger-attachments`
in a ws worktree (`~/.ws-worktrees/devenv/<name>`). The ticket worker's
own session committed through `ws/git.commit` with repo-relative paths and
the commit landed in the worktree as expected. The delegate implementer the
worker spawned (session key minted by `ws/playbook.render(name:
"delegate-implementer")`) reported that the same tool refused absolute
paths ("must be relative") and resolved relative paths against the main
checkout rather than the worktree it was told to work in, so it fell back to
plain `git commit`.

Reasonable caller expectation: a session rendered from inside a worktree
commits in that worktree, the same way the parent session does; at minimum
the refusal should name the directory paths are resolved against so the
caller can tell which checkout it is bound to.

Unverified: whether the rendered child session key is bound to the project
root instead of the parent's worktree, or whether the delegate's shell
`cwd` reset between tool calls (subagents start every shell in the main
checkout) is what the tool used. Reproduce with a child session key from a
worktree before changing anything.

## Decisions

1. **A session rendered by a session bound to a worktree is bound to that
   same worktree**, unless the render passes `root_override`, which wins;
   `ws/git.commit` with that child key stages and commits in that worktree.
   The tree already does this for lead-scoped callers (`server.go`
   L2206-L2217: `mintRoot` is `root_override`, else the caller's bound
   root), so this decision is held by a regression test, and the fix work
   is Decision 3's actual cause. Rejected: guessing the checkout from the
   caller's shell `cwd` (subagent shells start in the main checkout, which
   is the reported symptom).
2. **The "must be relative" refusal names the directory paths resolve
   against** (`agents-plugin-tool/internal/wsgit/git.go` near L872), so a
   caller can tell which checkout it is bound to.
3. **If reproduction shows the child key is already bound correctly and the
   symptom came from something else** (for example a delegate that received
   no child key because the render's minting runs only for lead callers,
   `internal/mcp/playbook_tools.go` near L913-916), fix that actual cause
   within the same intent - a delegate rendered from a worktree session
   commits in that worktree - and record the finding in the Result.

## Constraints

- Read `ai-docs/manuals/ws-mcp.md` and
  `ai-docs/manuals/shipped-surface-boundary.md` before editing
  `internal/mcp/`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 260609-refactor-ws-spawn-runtime-deletion-session-auth (2026-06-11, Result): "playbook.render(session_key, name, context?, root_override?) mints a fresh child session key when the caller key is lead-scoped AND the playbook frontmatter role is delegate-eligible ... binds it" — bearing: constrains
- dd722573 (2026-06-09, commit): "playbook.render mints+injects a fresh child key when session_key.role==lead, with a root_override param that rebinds both the auto-include root and the child-key root for worktree children" — bearing: supports
- 260909-epic-ws-worker-interpreter-refoundation (2026-09-09, Cross-Child Decisions): "role: worker is what makes playbook.render mint a lead-scoped key ... A subagent whose prompt is a rendered playbook gets its child key from playbook.render" — bearing: constrains
- 92e72d58 (2026-09-15, commit): "root_override binds the render-minted child key to the worktree root (server.go playbook.render: mintRoot = rootOverride when set) ... what actually isolates the worker's ws calls to its own worktree." — bearing: supports
- 260731-bug-playbook-render-root-override-manifest-resolution (2026-09-12, Result): "Artifact allocation and render-minted child-session roots retain the override; resource inspection and playbook.read retain their existing behavior." — bearing: constrains
- 260619-feat-ws-session-lineage-children (2026-06-19, Decisions): "Lineage edge is one optional parent field on the session record ... children() labels each child by its stored scope, never by comparing roots." — bearing: constrains
- dcaa0366 (2026-05-04, commit): "Safety: rejected absolute, option-like, and repository-escaping paths; refused commits with unrelated staged paths; hid git.commit from leaf-role MCP servers." — bearing: constrains
- 261108-feat-ws-tracker-ledger-attachments (2026-11-09, Result): "Dogfood: 261109-bug-git-commit-paths-in-delegate-worktree (the delegate implementer could not commit through ws/git.commit from the worktree)." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsgit/git.go, agents-plugin-tool/internal/mcp/playbook_tools.go, agents-plugin-tool/internal/mcp/server.go playbook.render mint-root branch L2206-L2217 and git.commit root resolution L1526-L1530 |
| scope.surface | cross-module | wsgit validateCommitPath error text plus mcp session-key root binding; no exported symbol named |
| scope.new_public_symbol | no | none; fix adjusts existing render mint and refusal text |
| scope.new_type_contract | no | none; sessionKeyRecord Root field already exists in internal/mcp/session_auth.go#L169-L180 |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/playbook_render_surface_test.go and playbook_tools_test.go cover child-key mint roots; agents-plugin-tool/internal/wsgit/git_test.go |
| complexity.reuse_points | confirmed | sessionStore.mint and the existing mintRoot = entry.root or root_override branch in server.go#L2206-L2217 |
| complexity.side_effect_risk | moderate | changing child-key root binding affects every render-minted delegate key and which checkout its git.commit stages in |
| risk.correctness | moderate | root cause unreproduced; tree already binds lead-scoped callers' children to the caller's root, so the real cause may lie elsewhere |
| risk.fit | moderate | must respect root_override precedence and the lead-scoped mint gate set by prior decisions |
| risk.test | moderate | needs a non-main worktree fixture with a child key committing through git.commit |
| risk.security_or_contract | moderate | session-key root binding is a capability boundary; a wrong root lets a delegate stage in another checkout |

## Phases

### Phase 1: reproduce, guard the binding, and fix the actual cause

- Reproduce: render a playbook from a worker session in a worktree, call
  `ws/git.commit` with the child key and a repo-relative path, and record
  which checkout the stage happens in.
- Add a regression test pinning Decision 1: a child key rendered by a
  session bound to a non-main worktree commits in that worktree, and
  `root_override` wins. The key's root stays the only root source for
  `git.commit`; do not add a second path-resolution mechanism.
- Fix the cause the reproduction finds (Decision 3). If it is caller-side
  (the parent rendered with the wrong key or `root_override`), the fix may
  be shipped playbook prose, which needs the shipped-surface reads above.
  If nothing fails, the phase is done with the regression test, the
  refusal-text change, and a Result note naming the likely cause (for
  example the delegate's shell `cwd`).
- Decision 2: `validateCommitPath` has no root parameter, so pass the root
  in or wrap the error at its caller; assert the refusal text names the
  base directory in `internal/wsgit/git_test.go`.

### Result (151ffac04) - 2026-11-10

- Reproduction: a worker key bound to a linked ws worktree rendered
  `delegate-implementer`; the minted delegate key's `key.json` root was that
  same worktree. Decision 1 already held, so nothing in the binding changed.
- Regression test `TestRenderFromWorktreeSessionCommitsInThatWorktree`
  (`internal/mcp/playbook_render_surface_test.go`): a child key rendered by a
  worktree-bound worker key inherits the worktree, `root_override` wins, and
  `git.commit` with the child key and a repo-relative path commits on the
  worktree's branch while the main checkout's HEAD stays put.
- Decision 2: `validateCommitPath(root, path)` now runs in `Commit` (its only
  caller of `normalizeCommitOptions`, so the validation order did not change).
  The "must be relative" and "must stay inside" refusals name the commit root
  that every path resolves against. An absolute path under that root also gets
  its relative spelling (`pass "src/file.go"`). Absolute paths are still
  refused, never rebased. Pinned in `internal/wsgit/git_test.go`.
- Decision 3 finding (likely cause): the binding was correct. Subagent
  shells start in the main checkout, and harnesses tell subagents to use
  absolute paths. The old refusal said only "must be relative" and named no
  base directory, so the delegate inferred that relative paths resolve
  against its shell cwd (the main checkout) and fell back to plain
  `git commit`. The new refusal text removes that inference. No caller-side
  playbook change was needed.
- Not done (possible policy follow-up): accepting absolute paths under the
  root. That would reverse the deliberate absolute-path refusal from
  dcaa0366.
- Review (lite): clean. One Minor finding is not fixed: in the CLI path, the
  root can stay `.` when neither `--root` nor `WS_MCP_PROJECT_ROOT` is set, so
  the refusal reads "commit root ." with no relative suggestion. The MCP path
  always passes an absolute session root.
- Verification: on the remote build server, `ws-rgo -- test ./internal/mcp/
  ./internal/wsgit/ -count=1 -timeout 30m` in `agents-plugin-tool` gave
  `internal/mcp` ok (104.0s) and `internal/wsgit` ok (0.2s). Locally,
  `go vet ./internal/wsgit/ ./internal/mcp/` was clean. A local
  `go test ./...` was cut short before it reached `internal/mcp`, so no
  full-suite pass is claimed.
