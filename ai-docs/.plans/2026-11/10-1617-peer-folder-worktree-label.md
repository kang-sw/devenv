# Peer lead folder prefix names the worktree's main repository

## Request

User (whiteboard, 2026-11-10), about the dashboard's agent panel "피어 리드" rows,
which prefix each peer's label with its folder name (`worktree-260825-1316/id:...`):
when the peer's folder is a git linked worktree, show the prefix as
`<root-dir-name>(<worktree-dir-name>)`, for example `devenv(gamma-washcloth-everyone)/`.

## Outcome

- `PeerRow`'s label prefix (`PeerRow.Folder()`, `agents-plugin-tool/internal/wsdashboard/pageview.go`
  ~L602, rendered by the `pfolder` template in `assets/dashboard.tmpl` L118) becomes
  `<main-root-base>(<path-base>)` when `PeerRow.Path` is a linked worktree, else today's
  last path element.
- Detection: `<Path>/.git` is a file whose `gitdir:` line points into a main repository's
  `.git/worktrees/<name>` directory; the main root is the directory holding that `.git`
  (prefer the gitdir's `commondir` file when present, resolved relative to the gitdir).
  A submodule (`.git` file pointing into `.git/modules/...`), a plain repo (`.git`
  directory), a missing or unreadable path, or a Windows path seen from another OS all fall
  back to today's plain folder name. Never run `git` for this.
- The lookup must not add per-render I/O: resolve once per distinct Path and memoize
  (worktree identity of a path does not change while a daemon runs; a cache bounded by the
  number of distinct peer paths is fine). Compute it where rows are built (the merge loop
  around pageview.go L510-600, or a field set there), not inside the template method on
  every render. Keep the pure label formatting testable without the filesystem
  (inject the resolver or test with temp dirs holding fake `.git` files).
- Tests in the existing wsdashboard pageview tests: linked worktree, plain repo, submodule,
  missing path, trailing separators / backslashes.

## Constraints

- Another worker is concurrently editing `pageview.go` (lazy snapshot at the ok/HasBoard
  sites) on a different branch; keep the change surgical so the merge stays clean.
- Never run go build/test/vet locally (it freezes the user's machine). Run remotely only:
  `~/.local/bin/ws-rgo -C /home/swkang/.ws-worktrees/devenv/aim-hydration-granite/agents-plugin-tool -m 30 -- test ./internal/wsdashboard/... -count=1 -timeout 30m`
  and `-- vet ./internal/wsdashboard/...`.
- Never touch the user's dashboard daemons on ports 29417 / 45743.
- Manual: `ai-docs/manuals/shipped-surface-boundary.md` (agents-plugin-tool).
