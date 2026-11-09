---
title: "ws/git.commit resolves paths against the main checkout for a delegate's session"
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

## Phases

### Phase 1: reproduce and bind the child session to the parent's worktree

- Reproduce: render a playbook from a worker session in a worktree, call
  `ws/git.commit` with the child key and a repo-relative path, and record
  which checkout the stage happens in.
- Fix according to the cause: bind rendered child sessions to the
  rendering session's worktree, or resolve relative paths against the
  caller's worktree when the key carries one; either way, make the
  "must be relative" refusal name the base directory.
- Add a test covering a child session key committing in a non-main
  worktree.
