---
title: "Git 저장소가 아닌 폴더에서도 ws 세션 생성 지원"
---

# Create a ws session in a folder that is not a Git repository

## Background

Owner request (2026-11-16 21:10 KST, verbatim): "그리고 이건 todo 티켓 감인데, 우리 그 git 아닌 폴더에서도 session 생성할 수 있게 하는 거 해결을 하긴 해야 합니다 ..."

Today a ws lead session can start only at the root of a Git worktree. Three places enforce this, read at develop `ba0926ee7`:

- **Lead bootstrap.** `agents-plugin-tool/internal/mcp/server.go` `canonicalSetupRoot` → `canonicalGitRoot` runs `git rev-parse --show-toplevel`. It refuses any other root with `root %q is not inside a Git worktree`. The same check covers `playbook.render` `root_override`, `git.merge` session roots, and worktree acquisition.
- **Dashboard spawn.** The comment on `RailRow.CanSpawn` in `agents-plugin-tool/internal/wsdashboard/pageview_rail.go` says the new-session button is hidden on a plain folder's bar, because "a lead session starts only at a Git worktree root and the spawn route refuses any other".
- **Root-aware tools.** Many tools assume a repository behind the session root: tickets, git.*, worktree pool, review ledger, the repo note layer, and the commit/follow-up tools.

The goal is that a user can open a session in an ordinary folder (no `.git` anywhere above it) from the dashboard, and from a harness lead bootstrap, and get a working lead whose Git-dependent features degrade clearly instead of refusing the whole session.

## Phases

### Phase 1: Session creation outside a Git repository

Not yet designed. The ticket is captured as backlog (owner: "todo 티켓 감"). These questions must be settled through the Open Decision Queue at `ready/` promotion. None of them is decided here:

- **Session root identity.** What identifies the session root and its project-scoped state for a non-Git folder? Today the project hash and session key binding derive from the Git root and common dir.
- **Folder kinds.** Which entry points must accept a non-Git folder: dashboard spawn, harness bootstrap (`workflow_manual` with `root`), Pi and Codex leads.
- **Non-Git behavior per tool family.** Pick one per family: refuse with a clear message, hide, or offer a non-Git equivalent. Families: tickets, git, worktrees, notes, review ledger.
- **`git init` later.** What happens when a folder that hosts a session becomes a repository, or sits inside one.
- **Compatibility tier.** Any store or hello shape change must state its tier per AGENTS.md `### Compatibility Tiers`.
