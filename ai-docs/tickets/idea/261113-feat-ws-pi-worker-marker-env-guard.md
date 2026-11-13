---
title: "Pi worker processes carry a worker marker the ws Pi extension honours"
---

# Pi worker processes carry a worker marker the ws Pi extension honours

## Background

Spawned Pi workers run with `--no-extensions --no-skills --no-prompt-templates
--no-context-files`. The ws Pi extension, when loaded, registers a session and
arms a mailbox wait; a worker that loaded it would duplicate the lead's session
row and mailbox. Today the only guard is `--no-extensions`: the moment a worker
profile allows extensions (for example to give workers a tool the extension
provides), the duplication comes back.

Owner 2026-11-13, whiteboard (26)=ok: record this as an idea only; Windows
stabilization and shipping come first.

## Idea

- The runner that spawns a Pi worker sets a worker marker environment
  variable on the child.
- The ws Pi extension checks the marker at load and, when present, skips
  session registration and the mailbox wait (tools that are safe in a worker
  may still register).
- Test: an extension load under the marker registers no session and arms no
  wait; without it, behaviour is unchanged.

## Open

- Which extension features stay available to a marked worker.
- Compatibility tier: a new optional environment variable read by the
  extension is expected to be T0; confirm at promotion.
