---
title: "ws/git.commit cannot carry the harness's attribution trailer lines"
---

# ws/git.commit cannot carry attribution trailer lines

## Observation (2026-11-10)

A ticket worker reported that `ws/git.commit` has no field for the commit
trailer lines a harness asks every commit to end with (for example
`Co-Authored-By:` and a session link). A worker that must not amend then
either leaves those lines out or falls back to plain `git commit`, so one
branch ended with uneven trailers across its commits.

## Directions (not decided)

- An optional `trailers` field (lines appended after the generated body), or
- let the caller's harness configuration supply default trailers the tool
  appends.

## Constraints

- MCP tool schema change under `agents-plugin-tool/internal/mcp/`; read
  `ai-docs/manuals/ws-mcp.md` and `shipped-surface-boundary.md` first.
