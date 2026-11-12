# Plan: interrupt badge resets on a server-reported turn boundary

Ticket: `261112-bug-ws-dashboard-interrupt-badge-stuck-on-back-to-back-turn`
(idea/; read it first, it has the reproduction and code pointers).

## Outcome

On a hosted session page, after the user clicks the interrupt badge, the badge
returns to its normal "Interrupt" state as soon as the interrupted turn ends,
**even when the next turn starts at once** (held message or queued mail
delivered at turn end), so that the new turn can be interrupted. The existing
guard still holds: a click in flight for the old turn must not cut the next
turn.

## Settled constraints

- Root cause (lead code read): `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js`
  `syncBadge` resets `interrupting` only when it observes the convlive
  `data-phase` outside the working set. A back-to-back turn never shows a
  non-working phase to the page.
- The fix keys the reset to a turn boundary the server reports on the convlive
  region, for example a per-turn identity (turn counter or turn-start stamp)
  rendered as a `data-` attribute. The page records the turn identity at click
  time and clears `interrupting` when the identity changes or the phase leaves
  the working set. Reuse an existing per-turn value from the live state or
  snapshot if one exists; add a minimal one only if none does.
- The interrupt POST and runner semantics (`wsagent/runner.go` `Interrupt`,
  `checkInterrupt`) are unchanged. This is a page-state fix.
- Rejected: a timer-based reset, which could re-enable mid-interrupt or stay
  stuck on a slow stop. Rejected: resetting on any convlive swap, which would
  let a double click cut the next turn.
- Compatibility tier: T0. Page markup and asset only, plus at most an additive
  field. No MCP, store, or tracker shape change.

## Scope and files

Expected: `internal/wsdashboard/assets/dashboard.js`, the convlive template in
`internal/wsdashboard/assets/dashboard.tmpl` and/or its view model
(`internal/wsdashboard/pageview.go`), with matching tests. If the fix needs a
new field in `internal/wslive` state or snapshot, keep it additive. Stop and
report if it needs more than that.

## Verification

- Go tests for any server-side change (template/view model renders the
  turn-identity attribute), run through `~/.local/bin/ws-rgo` per the machine
  note. Never run heavy `go build`/`go test` locally.
- Existing JS tests for the dashboard, if the package has a JS test harness;
  otherwise a focused unit test or a clear manual check description in the
  commit.
- `git diff` reviewed against this plan before commit.
