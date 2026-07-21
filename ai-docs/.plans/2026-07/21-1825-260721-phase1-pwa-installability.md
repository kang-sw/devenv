# Plan: 260721-feat-dashboard-suppress-browser-shortcuts — Phase 1: PWA installability

## Relevant Ticket Contract

- Phase 1 scope (`ai-docs/tickets/ready/260721-feat-dashboard-suppress-browser-shortcuts.md:141-151`):
  add a web app manifest (name, icons, `start_url`, `display: standalone`) and
  the minimal service worker needed to satisfy installability, so Chrome/Edge
  offers "Install app" from the served origin (`127.0.0.1:4300`, a secure
  localhost context). Optionally listen for `beforeinstallprompt`.
- Verification boundary stated in the ticket (`:149-151`): installed standalone
  window shows no tab strip/address bar; tab-management shortcuts have nothing
  to act on; Ctrl+R still reloads the installed app. This is a manual/browser
  check, not automatable in this environment.
- Spec Impact (`:182-190`): Phase 1's installability surface is contract-first
  spec — yes. A spec entry documenting the manifest fields, `display:
  standalone`, and the service worker's role must be added once implemented.
- Lead-decided defaults (from the task authority, not to be re-opened):
  - App name / short_name: `"ws-dashboard"`.
  - `theme_color` / `background_color`: derive from existing CSS tokens, not
    invented colors.
  - `display: "standalone"`; `start_url` is the app root the daemon serves.

## Out of Scope

- Phase 2 (Class-A `keydown` suppression / `preventDefault` interceptor) —
  `ai-docs/tickets/ready/260721-feat-dashboard-suppress-browser-shortcuts.md:153-168`.
- Phase 3 (Keyboard Lock API / fullscreen residual-key handling) — `:170-180`.
- Any Tauri/Electron wrapper — explicitly out of scope per the ticket's
  Delivery-Mode Spectrum (`:126-133`).
- Precaching app-shell assets, offline support, or any service-worker
  behavior beyond the minimum Chrome/Edge installability requires (an
  installability-qualifying SW only needs to exist, be registered, and have a
  `fetch` handler — see Codebase Findings below on why a no-op/network-first
  SW is deliberately chosen over a caching one).
- Icon *design* (visual identity/branding beyond reusing existing dark-theme
  tokens) — out of scope; this plan produces functionally-adequate icons only.

## Codebase Findings

### Build tooling

- `ws-dashboard/frontend/package.json:8` — `"build": "tsc -b && vite build"`.
  Vite 8.0.13 (`package.json:57` devDependency), plugin-react only
  (`vite.config.ts:89` `plugins: [react()]`). No `vite-plugin-pwa` (or any PWA
  package) appears anywhere in `package.json` dependencies/devDependencies —
  confirmed by full read of the dependency lists. **Decision: hand-roll**, not
  `vite-plugin-pwa`. Rationale: adding a new plugin dependency for one static
  manifest + a ~10-line SW is disproportionate, and `vite-plugin-pwa`'s
  default (`generateSW`/Workbox precache) is exactly the caching behavior the
  ticket/user does NOT want (the user explicitly cares about Ctrl+R reload
  behaving normally during dogfooding — a mis-scoped precaching SW risks
  serving stale `index.html`/JS after every rebuild). Hand-rolling keeps the
  SW's behavior fully explicit and auditable in one small file.
- `ws-dashboard/frontend/vite.config.ts:1-90` — no `publicDir` override, so
  Vite's default (`public/`, currently absent — see Icons below) applies: any
  file placed in `frontend/public/` is copied byte-for-byte to `dist/` root at
  build time, unhashed, at a stable path (e.g. `public/manifest.webmanifest`
  → `dist/manifest.webmanifest`). This is the mechanism to use for the
  manifest and icon files. The service worker script must also land at
  `dist/sw.js` (root, not under `dist/assets/`) so its default max scope is
  `/`, covering the whole app — Vite does not hash or transform files copied
  from `public/`, so `public/sw.js` → `dist/sw.js` verbatim, which is exactly
  what's needed (a plain, unbundled top-level script).
- `ws-dashboard/frontend/index.html:1-13` — no `<link rel="manifest">`, no
  service-worker registration script, no `<link rel="icon">`. Manifest link
  and SW registration must be added here (SW registration goes in
  `src/main.tsx` alongside the React bootstrap, or inline in `index.html`;
  `main.tsx` is the natural place since it already runs once at app start).

### Icons — none exist; must be produced

- No `public/` directory exists under `ws-dashboard/frontend/` at all
  (confirmed by directory listing — only `.gitignore`, `package.json`,
  `dist/`, `src/`, config files, etc.).
- No `src/assets/` directory, no favicon reference in
  `ws-dashboard/frontend/index.html` (grep for `rel=.icon`/`manifest`/
  `favicon` returned nothing), no `.ico`/`.png`/`.svg` app icon anywhere in
  `frontend/` outside `node_modules`.
- **This is a hard gap**: a valid installable manifest needs at least one
  `192x192` and one `512x512` PNG icon (maskable strongly recommended for
  Android/Chrome install UX, though Chrome desktop install on `127.0.0.1`
  does not strictly require maskable — `any` purpose icons are sufficient to
  pass installability).
- Recommended default (open implementation-time choice, not a design
  blocker): generate two flat, single-glyph PNGs programmatically (e.g. a
  small Node/Python script or an inline SVG rendered to PNG) using the
  existing dark palette — `--ws-color-canvas` (`#0f1117`,
  `ws-dashboard/frontend/src/styles.css:2`) as the icon background and
  `--ws-color-action` (`#78a9ff`, `styles.css:37`) as the glyph/foreground,
  echoing the same tokens chosen for `background_color`/`theme_color`. A
  single bold initial/mark (e.g. "W" or a simple geometric mark) at 192px and
  512px is sufficient; no illustration or multi-color branding work is
  needed. If no PNG-capable tool is available in the execution environment
  at implementation time, a same-color flat SVG icon declared in the
  manifest (`"type": "image/svg+xml"`) is an acceptable fallback for Chrome,
  which does accept SVG manifest icons, but PNG is the safer universal
  choice — implementer should prefer PNG and fall back to SVG only if PNG
  generation is unavailable.

### Static serving — the critical constraint

- `ws-dashboard/crates/daemon/src/router.rs:423-427` — the protected router
  only maps three static-serving routes: `/assets/{*asset_path}` →
  `static_asset` handler, and `/`, `/servers`, `/servers/{*app_path}` → `index`
  handler (serves `static_dir/index.html`). Everything else falls through to
  `.fallback(not_found)` at `:427`.
- `ws-dashboard/crates/daemon/src/router.rs:501-527` — `index()` only ever
  reads `static_dir.join("index.html")`; `static_asset()` only ever reads
  under `static_dir.join("assets").join(&asset_path)` (with
  `safe_relative_path` traversal guarding at `:540-550`). There is **no**
  generic catch-all serving arbitrary files at `static_dir` root (no
  `tower_http::services::ServeDir` usage anywhere in the daemon — confirmed by
  grep).
- Confirmed independently in
  `ai-docs/mental-model/ws-web-dashboard.md:86`: "Static dashboard serving is
  configuration-gated: `/`, `/servers`, and `/servers/{*app_path}` serve
  `static_dir/index.html`, `/assets/{*asset_path}` serves only safe relative
  paths below `static_dir/assets`, and all stay inside the owner-auth
  protected router... asset requests 404" otherwise.
- **Consequence**: dropping `manifest.webmanifest`, `sw.js`, and icon PNGs
  into `frontend/public/` (so they land at `dist/manifest.webmanifest`,
  `dist/sw.js`, `dist/icon-*.png`) makes Vite dev server (`npm run dev`) serve
  them correctly, but the **production daemon build will 404 on all of them**
  unless new routes are added to `router.rs`. This is a required
  implementation step, not optional polish — without it, Chrome cannot even
  fetch the manifest against the served origin the ticket targets
  (`127.0.0.1:4300`), so installability never triggers in the actual serving
  path the ticket cares about.
- `ws-dashboard/crates/daemon/src/router.rs:552-559` — `content_type_for_asset`
  only special-cases `css`/`js`/`svg`; anything else (including `.png`,
  `.webmanifest`) falls to `application/octet-stream`. New routes for the
  manifest/SW/icons should set correct content types explicitly
  (`application/manifest+json` or `application/json` for the manifest,
  `text/javascript`/`application/javascript` for `sw.js`, `image/png` for
  icons) rather than relying on the existing helper, since the SW file in
  particular benefits from an explicit JS content type for reliable
  registration, and reusing `content_type_for_asset` unmodified would mis-type
  all three as octet-stream.
- Auth boundary: `router.rs:93-99` documents the CONTRACT that `/`, static UI,
  and WebSocket routes stay behind the owner-auth boundary when enabled (see
  also `ai-docs/spec/ws-web-dashboard/index.md:696-714`, "Protected Frontend
  Shell": "Static asset serving does not add another unauthenticated
  top-level route beside `/pair`"). New manifest/SW/icon routes must be added
  inside the same `protected` router block (alongside `/assets/{*asset_path}`
  at `router.rs:423`), not as new top-level unauthenticated routes, to
  preserve this invariant.

### Service worker minimalism (informs implementation, not just survey)

- No existing service worker, `serviceWorker` registration call, or
  `manifest` reference exists anywhere in `ws-dashboard/frontend/src` (grep
  confirmed zero matches). This is fully new surface.
- Chrome/Edge installability requires a registered SW with (at minimum) a
  `fetch` event listener present in the script (an empty
  `self.addEventListener('fetch', ...)` — even a no-op pass-through —
  satisfies the installability heuristic) plus a valid manifest linked from
  the served document. It does **not** require any caching behavior.
- Given the ticket's explicit dev-iteration concern (Ctrl+R must keep
  reloading correctly, no stale assets), the SW must be a no-precache,
  network-passthrough SW: register with default scope `/`, and either omit
  caching entirely (a `fetch` listener that does nothing / lets the request
  proceed normally) or, if any caching is added for future phases, use a
  network-first strategy. This plan's Phase 1 scope is the no-op/passthrough
  form only — do not add `cache.addAll`/precache logic.

## Implementation Plan

1. **Icons** — create `ws-dashboard/frontend/public/icon-192.png` and
   `ws-dashboard/frontend/public/icon-512.png` (flat single-glyph mark,
   background `#0f1117`, foreground `#78a9ff`, per the palette tokens in
   `src/styles.css:2` and `:37`). Generate via any available scriptable
   method (Node canvas, ImageMagick, Python/Pillow, or a hand-authored SVG
   rasterized to PNG); fall back to a manifest-declared SVG icon only if no
   PNG rasterizer is available in the implementation environment.
2. **Manifest** — create `ws-dashboard/frontend/public/manifest.webmanifest`:
   ```json
   {
     "name": "ws-dashboard",
     "short_name": "ws-dashboard",
     "start_url": "/",
     "display": "standalone",
     "background_color": "#0f1117",
     "theme_color": "#78a9ff",
     "icons": [
       { "src": "/icon-192.png", "sizes": "192x192", "type": "image/png" },
       { "src": "/icon-512.png", "sizes": "512x512", "type": "image/png" }
     ]
   }
   ```
   `start_url: "/"` matches the daemon's served app root
   (`router.rs:426`, `/` → `index`).
3. **Service worker** — create `ws-dashboard/frontend/public/sw.js`: minimal
   no-op passthrough —
   ```js
   self.addEventListener('install', () => self.skipWaiting());
   self.addEventListener('activate', (e) => e.waitUntil(self.clients.claim()));
   self.addEventListener('fetch', () => {});
   ```
   (An empty/no-op `fetch` listener with no `respondWith` lets every request
   fall through to normal network handling — no caching, no interception —
   satisfying installability without risking stale-asset behavior on
   reload.)
4. **`index.html`** (`ws-dashboard/frontend/index.html`) — add inside
   `<head>`:
   `<link rel="manifest" href="/manifest.webmanifest" />` and
   `<link rel="icon" type="image/png" href="/icon-192.png" />`.
5. **SW registration** — in `ws-dashboard/frontend/src/main.tsx`, after the
   React root render (or in a small `if ('serviceWorker' in navigator)`
   guard), register `navigator.serviceWorker.register('/sw.js')`. Optionally
   also add a `window.addEventListener('beforeinstallprompt', ...)` listener
   here or in `App.tsx` per the ticket's "optionally surface an in-app install
   suggestion" — keep this minimal (e.g. just `event.preventDefault()` +
   stash the event; no UI polish required for Phase 1 unless trivial).
6. **Daemon routes** (`ws-dashboard/crates/daemon/src/router.rs`) — add three
   new routes inside the `protected` router, near the existing
   `/assets/{*asset_path}` route (`:423`):
   - `GET /manifest.webmanifest` → serve `static_dir.join("manifest.webmanifest")` with content type `application/manifest+json; charset=utf-8`.
   - `GET /sw.js` → serve `static_dir.join("sw.js")` with content type `text/javascript; charset=utf-8`.
   - `GET /icon-192.png` and `GET /icon-512.png` → serve `static_dir.join("icon-192.png")` / `icon-512.png` with content type `image/png`.
   Reuse the existing `serve_static_file` helper (`router.rs:533-538`); each
   new handler is a thin wrapper analogous to `index()`/`static_asset()`.
   These stay behind the same owner-auth boundary as `/`, satisfying the
   Protected Frontend Shell contract (spec `260516-ws-web-dashboard-protected-frontend-shell`).
7. **Doc/spec step** — add a new subsection to
   `ai-docs/spec/ws-web-dashboard/index.md`, most naturally near "Protected
   Frontend Shell" (`:696-714`) since it extends the same static-serving/auth
   contract, or as its own short section directly after it. New anchor stem:
   `260721-ws-dashboard-pwa-installability`. Content to cover: the daemon
   serves `manifest.webmanifest`, `sw.js`, and app icons at fixed root paths
   behind the same owner-auth protected router as `/` and `/assets`; the
   manifest declares `display: standalone`, `start_url: "/"`, and app
   name/icons; the service worker is intentionally a no-precache
   network-passthrough script whose only role is satisfying browser
   installability heuristics, not offline caching; installability enables
   Chrome/Edge's "Install app" affordance from the served origin. Also update
   `ai-docs/mental-model/ws-web-dashboard.md:86` (the static-serving bullet)
   to mention the new manifest/SW/icon routes alongside the existing `/`,
   `/servers`, `/assets` description, since that line will become stale
   otherwise.

## Verification Plan

- `cd ws-dashboard/frontend && npm run build` must stay green (`tsc -b && vite
  build`) after adding the new `public/` files and `index.html`/`main.tsx`
  changes — this is the primary automatable gate for this phase.
- Confirm `dist/manifest.webmanifest`, `dist/sw.js`, `dist/icon-192.png`,
  `dist/icon-512.png` exist after build and that the manifest file is valid
  JSON (`node -e "JSON.parse(require('fs').readFileSync('dist/manifest.webmanifest'))"` or equivalent).
- `cargo build -p ws-dashboard-daemon` (or the workspace build) must stay
  green after the `router.rs` route additions.
- Manual/dogfood-only (cannot be automated in this session, no live daemon
  instance running): start the daemon with `--static-dir` pointing at the
  built `dist/`, open `http://127.0.0.1:4300` in Chrome/Edge, confirm (a) no
  console errors on SW registration, (b) DevTools Application panel shows the
  manifest parsed without errors and the SW as activated, (c) the omnibox/
  menu offers "Install app", and (d) after installing, the standalone window
  has no tab strip/address bar and Ctrl+R still reloads. This matches the
  ticket's own stated verification boundary
  (`260721-feat-dashboard-suppress-browser-shortcuts.md:149-151`) and is
  explicitly out of scope for automated CI in this phase.

## Escalations

- None.
