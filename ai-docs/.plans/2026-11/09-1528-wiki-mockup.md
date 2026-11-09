# Static mockup: tracker wiki in the Items panel

User decisions (board, 2026-11-09): (128) ok - build a static mockup of the wiki UI before the browser side of `261108-feat-ws-tracker-wiki-git-branch` Phase 2 is built (the git graph precedent). (129) a - the wiki lives in the tracker's left **Items panel**: a header switch *Items | Wiki*; under *Wiki* that two-thirds area shows a page tree (left) and the page (right), and editing happens in place. In the narrow layout it is one tab. The store page (`#/<storeId>`, full-page) uses the same switch, wider. A worker builds Phase 2's server side meanwhile; this mockup decides the look.

## Inputs

- Ticket: `ai-docs/tickets/ready/261108-feat-ws-tracker-wiki-git-branch.md` (Decisions: wikis as `refs/heads/wiki/<name>`; page path = file path without `.md`; `wiki:<name>/<path>` absolute links from item bodies and pages; `attachment:<sha256>` images from the wiki tree's `_attachments/`; edit with base, three-way merge, refused save shows both versions; no whole-wiki delete; history and recovery through agent tools only; Phase 2 bullet list: wiki picker, page list, page rendering, edit with the shared editor and preview, image paste, refused-save view showing both versions, empty state with "new wiki").
- Tracker look and layout: `ai-docs/ref/tracker-mockup/` (README "Drawer layout" A / D / narrow, store page; `style.css`, `index.html`) and the shipped `agents-plugin-tool/internal/wsdashboard/assets/tracker.js` / its CSS: Items panel header, tab pane with icon tabs, the item window, the shared markdown editor (toolbar, preview) from `261109-feat-ws-tracker-shared-markdown-editor`.
- Format precedent: `ai-docs/ref/git-graph-mockup/index.html` (one self-contained static page, inline CSS/JS/SVG, theme toggle, fixtures).

## Outcome

`ai-docs/ref/wiki-mockup/index.html`: one self-contained static page (no network), opened from the filesystem, showing in the tracker drawer frame (layout A by default, with a width switch to show D, narrow, and the store page):

1. The Items panel header with the *Items | Wiki* switch; under *Wiki*: a wiki picker (several wikis plus "new wiki"), a page tree from folder paths, a search box, and the rendered page (tracker renderer look: headings, lists, code, a table, an `attachment:` image, a `wiki:` link to another page, a `#ID` item reference). The tab pane on the right stays as it is.
2. Edit mode in place: the shared editor with its toolbar and preview, a pasted image pending, Save / Cancel, base commit shown discreetly.
3. A refused save view: both versions side by side (yours / theirs) with the overlapping lines marked, and the ways out (copy mine, reload theirs, edit again).
4. Empty state: no wiki yet, a "new wiki" call to action (name input).
5. An item body (in the item window) with a `wiki:` link; clicking it switches the panel to that page.
6. Narrow layout: Wiki as one tab; store page: the same switch at full width.
7. Light and dark themes via the dashboard's tokens; a short on-page legend of what is shown.

Plain inline script with hard-coded fixtures; no build, no tests. Reuse the dashboard and tracker CSS tokens and class conventions so the Phase 2 worker can lift styles. No change to shipped code.

## Verification

Open the page in headless Chromium and take screenshots: wiki reading (light + dark), edit mode, refused save, empty state, narrow, store page. Save screenshots to the scratchpad, not the repo. Commit only the mockup page and this plan on branch `wiki-mockup`. Return the screenshot paths and the page path.
