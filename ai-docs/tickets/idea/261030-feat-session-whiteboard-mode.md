---
title: Session whiteboard mode - a per-session HTML page as the lead's primary user channel
---

# Session whiteboard mode - a per-session HTML page as the lead's primary user channel

## Background

The dominant use case is a Claude terminal and a browser page open side by
side. Asked items (Open Decision Queue entries, discussion policy questions)
and explanations are hard to read in terminal prose, and the model cannot draw
there. Whiteboard mode gives each lead session one local HTML page, the
session whiteboard, that the lead keeps current and the user watches; the
terminal carries only a pointer to it.

Naming: "blackboard" was rejected because in multi-agent literature it names
shared inter-agent memory (blackboard architecture); "canvas" was rejected
because it collides with the HTML `<canvas>` element inside an HTML file.

## Decisions

- **Knob.** `workflow.whiteboard_enabled` (boolean), default off in both ws
  and wsflow, default scope global (side-by-side viewing is the user's habit,
  not a project property). Naming follows `claude.status_band_enabled` /
  `claude.mcp_watchdog_enabled`. Off by default because it changes the
  model's output channel. A knob change applies at the next workflow-manual
  load.
- **Location.** `<session root>/.ws-workflow/whiteboards/<reply-id>.html`,
  where `<reply-id>` is the session's mailbox reply id
  (`wsmailbox.ReplyID`) without the `id:` prefix. Only the subpath
  `/.ws-workflow/whiteboards/` is registered in the repository's local
  `.git/info/exclude` (common dir, so every worktree is covered; the
  committed `.ws-workflow/config.json` is unaffected), in the idempotent
  manner of `registerPoolExclude` (`internal/mcp/worktree_tools.go`).
  User decision: no new root-level directory.
  Rejected: a new root dir `.ws-blackboard/` (root clutter); the session key
  as the file name (the key is a lead-scope capability, `session_auth.go`,
  and a file name exposes it to any directory listing; the reply id is the
  existing public, deterministic handle already shown in the status band).
- **Authoring and change signal (user decision (43)).**
  - The lead edits `<reply-id>.html` directly as plain HTML, partial edits
    allowed.
  - After editing, the lead calls the new MCP tool
    `ws/whiteboard.refresh(session_key)`. It:
    - creates the html from the bundled template when missing (this is the
      only creation path; the workflow manual does not pre-create the file);
    - re-inserts the template's loader block when the html lacks it (guards
      a whole-file rewrite that dropped it);
    - writes `<reply-id>.stamp.js` containing `wbStamp("<version>")` through
      a temp file and rename;
    - registers the exclude entry;
    - returns the whiteboard path.
  - The template's `<head>` holds a loader block marked do-not-edit. Every
    1-2 s it injects `<script src="<reply-id>.stamp.js?t=<now>">`. It records
    the first stamp seen, and on a different stamp it calls
    `location.reload()`, restoring the scroll position through
    `sessionStorage`.
  - Reload therefore happens only on an explicit refresh, so interactive
    state survives between updates. A refresh after the edit completes also
    removes the torn-read window.
  - Rejected:
    - Blind periodic reload: resets interactive state even when nothing
      changed.
    - `fetch`/XHR or iframe DOM read of the html: blocked for `file://`
      pages, which are opaque or unique origins in Chrome/Edge and in
      Firefox defaults.
    - A sidecar data `.js` carrying section HTML in template literals:
      escaping of backticks and `${` is messy for authoring.
    - File System Access API: Chromium-only, needs a re-grant per tab.
    - A local HTTP server with SSE: port and lifecycle burden, and a local
      file is the cleanest medium.
- **Template.** A header (session reply id, last-updated time) and three
  marked sections the lead edits in place:
  - **Needs your answer:** asked items with their `(nn)` IDs.
  - **Now:** the current explanation; free HTML, diagrams, and tables.
  - **Status:** one line.

  The lead may add sections. There is no per-project template override in
  this ticket.
- **Workflow-manual section (rendered only when the knob is on; compact,
  since the manual is injected every session).**
  - The whiteboard is the primary channel (user decision (40a)). Every item
    asked of the user (discussion items and Open Decision Queue items, with
    their IDs) and every explanation goes on the whiteboard, and the
    terminal prints only a short pointer ("see the whiteboard").
  - A ticket's Open Decision Queue section remains the authority; the
    whiteboard displays it. The user still answers in the terminal by ID.
  - Skill-reserved asks (the lead-review next-step handoff, the lead-ship
    publish confirmation, Approval Protocol asks) remain asks, shown on the
    whiteboard.
  - Output lines a playbook mandates in a fixed format (for example
    `Ticket: <path>`) stay in the terminal.
  - The page is a current-state page (user decision (41)): updated
    immediately, resolved items removed (decisions persist in tickets and
    commits).
  - Order per turn: edit the html, call `whiteboard.refresh`, then print the
    terminal pointer.
  - Lead sessions only; workers and subagents do not use the whiteboard.
  - The manual text is host-neutral (Claude, Codex, Pi).
- **GC.** At workflow-manual load, at most once per day per repository,
  bookkept with a marker file in `.ws-workflow/whiteboards/` (the
  `maybePrune` marker pattern, `internal/mcp/session_prune.go`): delete every
  whiteboard html/stamp pair whose reply id matches no live session key
  record. Rejected: mtime-based expiry (deletes a live session's
  rarely-touched page); hooking the machine-global key prune (it does not
  know per-repo directories). There is no explicit session-delete path;
  key expiry is the deletion.
- **Claude status band (claude-mod).**
  - Row 3:
    - Left: the whiteboard path relative to the session root, computed from
      the reply id the band already reads plus the knob. It shows even
      before the file exists. Make it an OSC 8 link to the absolute
      `file://` URL when the host's element table supports links.
    - Right end: one capsule `todo N/M · K agendas`.
  - Row 4 is removed.
  - With the knob off, row 3 left falls back to the focus todo (current
    behavior).

## Constraints

- Manuals that bind this work: `ai-docs/manuals/shipped-surface-boundary.md`
  (tool strings, manual section, template ship downstream),
  `ai-docs/manuals/skill-authoring.md` (manual section prose),
  `ai-docs/manuals/wsflow-mirroring.md` (knob in both config manifests;
  manual and band mirrors), `ai-docs/manuals/ws-mcp.md` (new tool).
- The new tool is lead-only, follows the existing schema/precheck path in
  `Server.callTool`, and its schema declares every argument it reads.
- Remote sessions without local file access are out of scope; the knob stays
  off there.
- Out of scope for this ticket: auto-refresh from a Claude mod hook on Edit of
  the whiteboard path (possible later reinforcement against a forgotten
  refresh).
- Accepted risk: a lead that forgets `whiteboard.refresh` leaves the page
  stale; the manual's fixed edit-refresh-pointer order is the mitigation.

## Phases

### Phase 1: Probe file:// stamp polling on Windows browsers

Verify on native Windows Chrome and Edge that a `file://` page can repeatedly
inject `<script src="x.stamp.js?t=<now>">`, observe a rewritten stamp within
one poll interval (no stale cache), and reload with scroll restoration. Also
check a WSL path opened through `\\wsl.localhost\...`. Record the result; if
re-injection does not observe changes, stop and return to the lead before
Phase 2 (the authoring design depends on it).

### Phase 2: Implement whiteboard mode

Implement the Decisions: the knob in both config manifests, the bundled
template, `ws/whiteboard.refresh`, the exclude registration, the daily GC at
workflow-manual load, the conditional workflow-manual section, and the status
band row-3 rework.

Verification:

- Go tests in `internal/mcp`:
  - refresh creates the file from the template when missing;
  - refresh re-inserts a removed loader block;
  - refresh writes and bumps the stamp atomically;
  - the exclude entry is added once (idempotent);
  - GC removes orphaned pairs and keeps live ones, at most once per day;
  - the manual section renders only when the knob is on.
- claude-mod engine tests for the row-3 layout with the knob on and off, and
  for row 4's removal.
- A manual browser check of the shipped template against a real refresh.
