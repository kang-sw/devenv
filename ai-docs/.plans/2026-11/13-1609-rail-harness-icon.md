# Delegate plan: session rail rows - harness icon, sleeping-session styling, repo icon after title

## Outcome

User dashboard UI feedback (2026-11-13, confirmed on the whiteboard as
(118)-(120)). In the dashboard **session rail** (the left list of
workspaces -> branches -> session rows: harness badge, repo icon, title,
question badge, age, and a second line with the last-message preview):

1. Replace the text harness badge (`CLAUDE`, `PI`, ...) with a small square
   letter icon, to save horizontal space:
   - claude: background `#D97757`, white bold `C`
   - pi: background `#000`, white bold `P`, with a muted grey border (e.g.
     `#666`) so it stays visible on the dark theme
   - codex: background `#10A37F`, white bold `X`
   - unknown / missing harness: grey background, `?`
   Size it to the row's text line (about 16px), rounded ~3px corners. Keep
   the harness name available as a `title` tooltip / accessible label.
   No third-party logo assets.
2. Where the rail currently shows a running/busy spinner for a session,
   drop the separate spinner and rotate the harness icon itself instead
   (CSS animation on the icon while the session is running; static
   otherwise). Respect `prefers-reduced-motion` only if the existing spinner
   already does.
3. A sleeping session (the rail's existing "sleep"/put-to-sleep state - find
   how the rail marks it today) renders its title text in greyscale (muted
   grey instead of the link colour).
4. A sleeping session renders as ONE line only: hide the second line (the
   last-message preview text).
5. Move the repo icon (the small server-shaped icon currently between the
   harness badge and the title) to sit right after the title text.

## Constraints

- Dashboard assets / rail rendering only (expected: `wsdashboard/rail.go`
  and/or its template, `assets/dashboard.css`, possibly `assets/dashboard.js`
  where the spinner or rows are built client-side). Find the exact files;
  keep the diff minimal and follow existing styles (dark theme variables).
- If a session row is rendered in more than one place (server template and
  a client-side re-render / SSE update), change both consistently.
- The main track is concurrently changing dashboard assets
  (`impl/develop/lead-wake-events`: wake-reason chips; `impl/develop/wb-messages`:
  whiteboard rendering). Do not refactor shared code; touch only the rail row.
- Do not restart or replace the live dashboard daemon. For visual checks run
  a separate dev/preview instance (or a Go test/HTML fixture rendered in a
  headless browser), never the user's running daemon.

## Verification

- Existing wsdashboard tests that assert rail markup: update them; add a
  small assertion for the harness icon class/letter and the sleeping-row
  single-line rule if a rail render test exists.
- `go test ./internal/wsdashboard/` passes (remote w5090 per the clone note;
  Node-dependent tests locally under `bash D:/sys_lock.sh run <who> <min> -- <cmd>`).
- LOOK gate: one screenshot of the rail showing at least a running claude
  row (icon mid-rotation is fine), a pi row, and a sleeping row. Save it under
  `D:/Temp/rail-harness-icon/` and report the path. One retry on an
  environmental capture failure, then report.

## Return boundary

Commit on `impl/develop/rail-harness-icon` in this worktree (this plan rides
the commit). Do not merge. Stop and report if "sleep" state or the spinner
does not exist in the rail as described, or if the change needs a non-asset
data change (e.g. a new field in the rail data model) beyond passing an
existing value into the row.
