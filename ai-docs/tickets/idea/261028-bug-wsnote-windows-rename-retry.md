---
title: Retry wsnote file replacement on Windows sharing violations
---
# Retry wsnote file replacement on Windows sharing violations

## Background

`wsnote` writes a note by renaming a temp file over the target with a plain
`os.Rename` (`agents-plugin-tool/internal/wsnote/store.go:204`,
`repo_store.go:174`). On Windows a concurrent unlocked reader (a `note.query`
in another ws-mcp process, an AV scanner) holds the destination open, the
replace fails with Access denied / ERROR_SHARING_VIOLATION, and `note.write`
fails. `wsmailbox` was hotfixed for the same exposure, and `wsconfig` and
`wsstate` carry their own retry helpers; `wsnote` never got one. Recorded as
review follow-up `runt-rise-lushness` and confirmed by the 2026-10-28
follow-up triage (Windows-only, transient, still present at `f400e5fbe`).

The fix has a design choice in it, so it is a ticket rather than a direct
repair: there are already three near-identical package-local helpers, each
with its own retry policy.

| package | file | policy |
|---|---|---|
| `wsmailbox` | `replace_file_windows.go` | 2 s deadline retry, runs under a flock whose LockTimeout is also 2 s |
| `wsconfig` | `replace_file_windows.go` | package-local |
| `wsstate` | `replace_file_windows.go` | 5 attempts, exponential back-off from 10 ms, sharing violations only |

Folded-in review follow-ups on the same family (resolved by the commit that
created this ticket, so this ticket owns them):

- `undergrad-stiffly-buddy` - the three helpers should be consolidated.
- `taunt-almost-accompany` - `wsmailbox`'s 2 s retry runs under the 2 s
  flock; a long hold can surface as a peer lock timeout (acknowledged in a
  comment at `wsmailbox/store.go:220`).
- `anyplace-wifi-shelf` - `wsmailbox/replace_file_windows.go` non-transient
  fast return and deadline give-up are untested; the Windows test does not
  check temp-file cleanup.

## Decisions

Open:

- Shared helper (one internal package used by all four) or a fourth
  package-local copy for `wsnote`? Consolidation removes the duplication but
  must pick one retry policy, and `wsmailbox`'s lock interplay may justify
  keeping its policy as a parameter.

## Phases

### Phase 1: Windows-safe replace for wsnote

Give `wsnote`'s two replace sites a sharing-violation retry, resolving the
open decision above first. Cover it with a Windows-tagged test like the
existing `replace_file_windows_test.go` files. Native-Windows verification
follows `ai-docs/manuals/windows-dogfood.md`.
