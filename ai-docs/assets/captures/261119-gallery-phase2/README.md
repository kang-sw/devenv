# 261119 paged docking gallery — Phase 2 captures

Live captures of split gallery pages next to the split pages of the approved mockup
`ai-docs/assets/mockups/261119/right-pinned-panel-concept/paged-docking-gallery-v4.html`.
These files come from the gallery Playwright suite's capture pass (the same run also writes the
Phase 1 captures; only the split-page files are kept here):

```sh
cd agents-plugin-tool
WS_PLAYWRIGHT_MODULE=<playwright> WS_GALLERY_CAPTURES=<dir> \
  go test ./internal/wsdashboard -run TestGalleryBrowser -count=1
```

The live captures use the dark color scheme at wide 2400x1000, with one docked page seeded through the
stored `gallery` record. The mockup's four-pane page is built by dropping a board item on the right edge
of each of its two panes (HTML5 drag in the mockup itself).

| Live | Reference | Compared |
|---|---|---|
| `live-wide-2pane-2400x1000.png` | `mockup-v4-wide-2pane.png` | match: a column of two equal full-width panes (alpha over beta; mockup term over panel) with a 6px gap, and the active dot reads `2`. |
| `live-wide-4pane-2400x1000.png` | `mockup-v4-wide-4pane.png` | match: a column of two rows, a 2x2 grid of equal panes (alpha, epsilon / beta, deep; mockup term, topic / panel, files) with 6px gaps, and the dot reads `4`. |
| `live-wide-zone-right-2400x1000.png` | `mockup-v4-wide-zone-right.png` | match: the right-edge zone preview tints the right half of the hovered pane, inset 3px, primary border and 24% fill. |

Deliberate differences:

- The live preview is one overlay element over the gallery viewport (`.gal-overlay`, placed by cards.js),
  not a child of every pane as in the mockup; the hovered pane carries `data-dz` the same way. The center
  zone (a docked pane only) previews the whole pane.
- The 6px gap between split children is the live divider (`.gal-div`, focusable separator); it is
  invisible at rest like the mockup's gap and shows a primary line on hover, focus, or drag.
- The mockup still is its `data-dz="right"` styling set directly (its own dragover sets it); the live
  capture is a real mid-drag of epsilon from the Main Board (the dimmed board topic and its status
  tooltip are the drag's own feedback).
- The Phase 1 differences still apply: the dashboard's own topic header instead of the mockup's `.gal-phead`,
  no page meta line, and the dashboard-styled rail.
