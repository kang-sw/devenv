---
title: claude-dash — Windows native build verification
---

# claude-dash — Windows native build verification

## Background

`claude-dash` is intended to run natively on Windows (as `claude-dash.exe` spawning `claude.exe`). The core dependencies (`portable-pty` via ConPty, `ratatui`+`crossterm`, `termwiz`) support Windows. Path escaping for session discovery was pre-fixed (1fb2a9d). A native build hasn't been attempted yet.

## Tasks

- [ ] Cross-compile or natively build `claude-dash` for Windows and verify it starts.
- [ ] Verify `claude.exe` spawns correctly as a PTY subprocess via ConPty.
- [ ] Verify session directory escaping matches `claude.exe`'s actual project dir naming (the `escape_path` Windows impl may need tuning based on real `claude.exe` output).
- [ ] Verify `git worktree list --porcelain` output on Windows (path separators in `"worktree "` lines).
- [ ] Verify mouse events work in Windows Terminal.
- [ ] Fix any issues found.
