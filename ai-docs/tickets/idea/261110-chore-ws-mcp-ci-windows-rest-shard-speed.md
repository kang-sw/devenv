---
title: "Bring the ws-mcp CI windows `rest` shard down to the ~7 min of the other shards"
---

# Bring the ws-mcp CI windows `rest` shard down to ~7 min

## Problem

`ws-mcp-ci.yml` gates every release (the ship flow dispatches it on the exact
commit to tag). Its wall time is the slowest job, and that is now always
`go test (windows-latest, rest)`:

| run | rest shard | other windows shards (mcp-0..3) | ubuntu all |
|---|---|---|---|
| 42821152376 (release/0.48.17) | 14m51s | 5m36s - 6m20s | 3m45s |
| 42820094730 (develop 00fd2e5d9) | 18m12s | | |
| four earlier runs on 2026-11-10 | 14m08s - 20m10s | | |

The user's goal (2026-11-10, whiteboard): "속도 좀 다른거랑 비슷하게 7분대로
끌어내릴 수 있게요" - bring `rest` to about the same ~7 min as the others.

## Current shape (evidence)

- `agents-plugin-tool/scripts/ci-test-shard.sh rest` runs
  `go test -count=1 -timeout 30m` over every package except `internal/mcp`,
  in one job. The `mcp I N` shards split `internal/mcp` four ways by sorted
  top-level test index.
- The workflow comment records why the split exists: `internal/mcp` alone took
  ~15 min serially on Windows (run 36662026067). Since then `rest` has grown
  (wsdashboard proxy/daemon tests, cmd/ws-mcp launcher and daemon proxy tests,
  wstrackerhost, wsdoc, wsgit...) and is now the long pole.
- The coverage invariant: `rest` plus every `mcp I N` covers exactly the
  tests of `all`. Any new split must keep that invariant.

## Directions to evaluate (not decided)

- Measure first: per-package wall time on Windows for the `rest` set
  (`go test -json` timings, or `-v` timestamps) to find the dominant packages.
- Split `rest` into two or more shards by package (for example `cmd/ws-mcp` +
  `wsdashboard` vs. everything else), keeping the coverage invariant
  machine-checked (a `-list` union test in the script or CI).
- Check whether package-level parallelism (`-p`) is limited on the Windows
  runner, and whether the dominant packages run serially by design
  (git-process-heavy tests, `t.Parallel` absent).
- Reduce slow individual tests (sleep/poll-based waits, real daemon spawns)
  where they are the cause, without weakening what they pin.

## Constraints

- Heavy go build/test must not run on the user's local machine; measure via
  CI runs or `~/.local/bin/ws-rgo` (Linux remote, so Windows timings need CI).
- Keep the `timeout-minutes: 25` hang guard meaningful.
- The ship flow (`ai-docs/ship/ws.md`) checks every `go test (<os>, <shard>)`
  job; a renamed or added shard must stay matched by that wording.
