# Plan: hosted runner repairs from the 0.49 pre-ship sweep (findings 1 and 3)

Source: 0.49 pre-ship sweep `62304312b..32b702129` (stamped concern,
ea0b38f34). Findings file (read section "Hosted / Pi" and the matching
Minor lines first; it is the evidence):
`/home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/05aaa35c-01-findings.md`.
Background for the existing rejoin hold: plan
`ai-docs/.plans/2026-11/12-1419-holder-ring-wrap.md` and merge 91be5e087.

## Outcome

Fix, each with a failing-then-passing regression test:

1. **Wrapped ring while attached** (`internal/wsagent/holder.go:280`
   `h.wrapped = first > h.skipTo+1`, `runner.go:1724-1748` `replayDone`):
   a ring that wrapped while the daemon stayed attached (a long turn pushed
   its own turn start out of the 32 MiB ring) is not detected on rejoin
   after a graceful restart: `FirstSeq <= HolderSeq+1`, no gap, no
   TurnStart replayed, `c.turn` stays false, and `replayDone` kicks held or
   queued input into the running turn; Pi refuses the prompt, the item is
   already cleared from `Record.Queued`, so the message is lost and the
   runner closes a still-running turn. Settled fix (user board (34)): when
   the ring has dropped records (its first seq > 1) and the replayed window
   shows no turn boundary, treat the turn state as lost and apply the
   existing `wrapHold` (submit nothing until the harness's next idle
   signal). Rejected: persisting turn state in the Record.
3. **No retry after a failed send** (`runner.go:1438-1442`): after a failed
   `Send`/`runCommand`, `stopProcess("")` returns; `Record.Queued` keeps the
   items but `processEnded` does not kick, so nothing retries until an
   unrelated inbox or mail signal. Settled fix (user board (35)):
   `processEnded` kicks when `Record.Queued` still holds items, through the
   existing restart path. Rejected: a separate retry timer. Make sure this
   cannot loop hot when every send fails (reuse whatever backoff or failure
   guard the restart path already has; if none exists, stop and report
   rather than invent one).

Finding 2 (Pi subagent widget volume) is out of scope here; it waits on a
user decision and goes to a separate change.

## Settled constraints

- Compatibility tier T0: no record, registry, snapshot, holder protocol, or
  MCP shape change. Stop and report if a fix needs one.
- Stay within `internal/wsagent` and `internal/wsholder` (plus tests). Do not
  touch the sweep's Minor follow-ups.
- Heavy `go build`/`go test`/`go vet` only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m <min> -- <go args>`.

## Verification

- Tests: a rejoin over a ring whose first seq > 1 but <= HolderSeq+1 with
  no replayed turn boundary holds a queued message until the idle signal,
  then delivers it once; an unwrapped rejoin (first seq == 1) behaves as
  before; a failed send leaves the item queued and it is delivered after
  the process restarts without any further inbox signal; a persistently
  failing send does not spin.
- `go test ./internal/wsagent/... ./internal/wsholder/...` and `go vet`
  through ws-rgo; `git diff` reviewed against this plan.
- One commit per fix on `impl/develop/hosted-sweep-049`; the plan file rides
  the first commit.
