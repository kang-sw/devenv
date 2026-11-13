# Delegate plan: session-spawn modal - plugin isolation control

## Outcome

Owner decision (392=c, 2026-11-13): the hosted-lead plugin isolation level is
picked in the dashboard's session-spawn (new hosted lead) modal. The backend
landed on develop in 876dd14b0 (`impl/develop/query-shush-bulge`); this unit
adds only the modal control.

Backend contract (from the main track lead):

- The lead-create HTTP request JSON gains `plugin_isolation`:
  - `"default"`: the bundled ws/wsflow replaces the installed copies; other
    plugins/extensions stay.
  - `"isolated"`: only the bundled ws/wsflow are loaded; the user's
    permissions, hooks, memory and MCP settings stay.
  - Omitted means `"default"`.
- The value is persisted on the session record across resume/relaunch.
- Limitation: for Pi, `"default"` needs the JS SDK next to the Pi CLI; a
  standalone Pi binary supports `"isolated"` only. The backend may record a
  `PluginWarning` on the session for this.

## Settled decisions (lead)

1. A compact two-choice control (segmented radio or select, whichever matches
   the modal's existing controls) labelled in the modal's existing UI
   language, default preselected. Each option gets a one-line hint that
   restates the contract above in plain words.
2. Submit always sends the chosen value as `plugin_isolation`. Sending the
   default explicitly is harmless and keeps the request self-describing.
   Rejected: omitting the field for the default (an extra branch, nothing
   gained).
3. Pi limitation: when the Pi harness is selected, show a short muted note
   under the control that a standalone Pi binary supports "isolated" only.
   Do not try to detect the Pi install from the browser. If the dashboard
   already renders a session's `PluginWarning` somewhere, leave it as is;
   adding a new warning surface is out of scope (report it as a Forward).
4. If the modal remembers the last choices (e.g. localStorage), remember this
   one the same way; otherwise don't add persistence.
5. T0: dashboard asset content only, no Go data-model change. The request
   field already exists server-side.

## Constraints

- Dashboard assets only (`agents-plugin-tool/internal/wsdashboard/assets/`)
  plus their Go/Node tests. If the modal's request is built in Go-rendered
  template data rather than JS, a minimal template change is fine. Stop and
  report if a Go handler change is needed.
- Keep shared-file diffs minimal: another worker (`impl/develop/lunar-coil-phony`,
  rail usage gutter) edits dashboard assets in parallel.
- Never restart or replace the user's live dashboard daemon. LOOK via a
  rendered fixture or a separate dev instance in a headless browser.

## Verification

- A test asserting the modal markup carries the control with both values and
  that the create request includes `plugin_isolation` (an existing Node
  contract test for the modal/JS if one exists; otherwise a Go asset test in
  the style of the existing `wsdashboard` asset tests).
- `go test ./internal/wsdashboard/` on remote w5090:
  `bash D:/sys_lock.sh remote w5090 run <who> <min> --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/exceeding-fever-folk' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wsdashboard/'`
  (Node-dependent tests locally under `bash D:/sys_lock.sh run <who> <min> -- <cmd>`).
  One passing full check closes the work.
- LOOK: one screenshot of the open spawn modal showing the control (Pi harness
  selected so the note shows) under `D:/Temp/spawn-modal-isolation/`. One
  retry on an environmental capture failure, then report.

## Return boundary

Commit on `impl/develop/spawn-modal-isolation` in this worktree (this plan
rides the commit). Do not merge.

Machine rules: temp files only under `D:/Temp/<purpose>/`; never create
folders in C:/ or D:/ roots; stop if D: free < 20 GB; never print credential
tokens.
