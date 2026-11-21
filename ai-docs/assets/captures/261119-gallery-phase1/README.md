# 261119 paged docking gallery — Phase 1 captures

Live captures of the dashboard gallery next to the approved mockup
`ai-docs/assets/mockups/261119/right-pinned-panel-concept/paged-docking-gallery-v4.html`.
These files are produced by the gallery Playwright suite:

```sh
cd agents-plugin-tool
WS_PLAYWRIGHT_MODULE=<playwright> WS_GALLERY_CAPTURES=<dir> \
  go test ./internal/wsdashboard -run TestGalleryBrowser -count=1
```

The live captures use the dark color scheme, with two docked pages (alpha, deep).

| Live | Reference | Compared |
|---|---|---|
| `live-wide-2400x1000.png` | `mockup-v4-wide.png` | The rail sits immediately right of the `.bp-edge` handle. Dots read `1 · 1 · +` and show pane counts. The docked pane is full-bleed in the column. |
| `live-narrow-1200x900-page.png` | `mockup-v4-narrow-page.png` | The rail reads `⌂ · 1 · 1 · +` left of the board column. The docked page covers the Main Board full-bleed. |
| `live-narrow-1200x900-home.png` | `mockup-v4-narrow-home.png` | `⌂` is active and the Main Board shows as Page 0 under the same rail. |
| `live-touch-420x860.png` | `mockup-v4-narrow-page.png` | The touch viewport uses the same narrow rail and a full-bleed page. |

Deliberate differences:

- The live rail is a rounded, shadowed strip in the dashboard's panel style. The mockup rail is a flush strip with a border.
- The live rail has no thin separator under `⌂`; it uses an 8px gap instead. The live `⌂` green is `#3fa853` (mockup `#5cc46a`).
- There is no page meta line ("도킹 Page 1 · 패널 1개") above the viewport, because the ticket allows no extra chrome.
- A docked pane keeps the dashboard's own topic or card header instead of the mockup's kind-tag `.gal-phead`, because the topic markup stays unchanged.
