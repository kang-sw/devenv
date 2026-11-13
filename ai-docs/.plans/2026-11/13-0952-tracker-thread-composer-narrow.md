# Dashboard tracker: thread comment composer breaks in a narrow item pane

## Problem

Owner report 2026-11-13 (screenshot at `D:/Temp/tracker-composer/broken.png`,
not tracked): in the tracker item detail pane at a narrow width (~280 px, the
pane beside the conversation), the thread's comment composer under
"스레드 N" is broken:

- the 작성 / 미리보기 (write / preview) tabs render as a tall vertical
  strip, each label wrapping one character per line;
- the comment textarea is squeezed to a sliver (its placeholder is cut,
  showing only "...)로 댓글 작"), with its resize grip floating beside it;
- the send button drops below the textarea instead of sitting beside or
  under it as a proper row.

The item's own edit form above it (title/body editor, 저장 / 취소) renders
fine at the same width.

## Outcome

- The thread composer stays usable at narrow pane widths: tabs on one row
  with labels unwrapped, the textarea at full available width with a
  readable placeholder, the send button aligned with the composer (same row
  when it fits, else its own row aligned to the right), no element
  overflowing the pane.
- Wide layout looks the same as today.
- Fix in the dashboard assets (`agents-plugin-tool/internal/wsdashboard/
  assets/` — likely `tracker.css` / `tracker.js` / `mention.js`, wherever
  the thread composer's markup and styles live). Reuse the item edit form's
  existing styles if the composer should match it; no new layout framework.
- Dashboard asset changes are T0 (asset content only).

## Out of scope

- Other tracker pane layout issues, unless the same rule causes them (then
  note them in the report).
- Restarting the dashboard daemon or any Claude session (the lead does the
  live check).

## Verification

- The existing wsdashboard Go tests pass (run them on the remote w5090 per
  the clone note `remote-build-w5090`), and any JS test runner the assets
  already use passes.
- Visual evidence at ~280 px and at a wide width, if a headless browser is
  available locally without installing new global tooling (e.g. render the
  composer markup in a static HTML page using the real CSS); otherwise state
  that the visual check is left to the lead.
