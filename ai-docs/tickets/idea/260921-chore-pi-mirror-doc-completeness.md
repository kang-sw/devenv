---
title: Name agents-plugin-pi as the third committed mirror in mirroring docs
---

# Name agents-plugin-pi as the third committed mirror in mirroring docs

## Background

`agents-plugin-pi/` maintains a committed, byte-identical copy of
`agents-plugin/`'s `rsrc/` tree, `runtime.json`, and `bin/ws-mcp-launcher.py`.
It is resynced by `agents-plugin-tool/scripts/bump-ws-version.sh`
(`sync_tree`/`sync_file`) and guarded per-commit by `TestPiMirrorUpToDate` in
`agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go` (byte-equality) — the
exact analogue of wsflow's `TestWsflowRsrcMirrorUpToDate`. (`agents-plugin-pi/`'s
`skills/` dir is a separate, gitignored, pack-time copy via `copy-skills.mjs`,
not a committed mirror.)

Despite being a real, guarded third mirror, `agents-plugin-pi/` is named in none
of: AGENTS.md `## Project Orientation` package list, AGENTS.md
`### Implementation Conventions` paths table, or `ai-docs/manuals/wsflow-mirroring.md`
(which documents only the ws↔wsflow pair). The only prose that mentions it is
`ai-docs/manuals/ws-mcp.md:148-151` in the version-bump-helper context.

Surfaced as non-blocking Fit finding F2 during the 0.46.15 ship-gate review.
This is a pure documentation-accuracy gap: enforcement already exists in code
(`TestPiMirrorUpToDate`), so there is no workflow hole — only the docs fail to
name the third mirror.

## Decisions

- Do NOT add an `agents-plugin-pi/` row to AGENTS.md's `### Implementation
  Conventions` paths table. Adding a row binds every future edit under those
  paths, which `TestPiMirrorUpToDate` already enforces mechanically — a table
  row would duplicate that obligation, not add coverage.
- Do NOT re-scope `wsflow-mirroring.md` as if pi were a wsflow-style curated
  mirror. Pi's rsrc is generated-sameness (byte-identical), like wsflow's rsrc
  subtree, but pi ships no curated lead-skill shims — its `skills/` is
  gitignored/pack-time. Document pi as what it is, not by analogy that overstates
  the parallel.

## Phases

### Phase 1: Document the pi mirror where readers already look

Add a concise, accurate mention of `agents-plugin-pi/` as a committed
byte-identical mirror (guarded by `TestPiMirrorUpToDate`, resynced by
`bump-ws-version.sh`) in the natural doc locations: AGENTS.md package list, and a
short note in `wsflow-mirroring.md` (or a sibling manual) that the rsrc mirror is
three-way, not two-way. Keep it descriptive of existing enforcement; introduce no
new per-edit obligation beyond what the guard test already imposes.
