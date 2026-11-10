# Vendored xterm.js

- Packages: `@xterm/xterm` 5.5.0 from npm, files `lib/xterm.js` (the
  single-file UMD build that sets `globalThis.Terminal`) and `css/xterm.css`;
  `@xterm/addon-fit` 0.10.0, file `lib/addon-fit.js` (sets
  `globalThis.FitAddon`). The files are unchanged.
- License: MIT, see `LICENSE` beside this file (copied from `@xterm/xterm`).
  `@xterm/addon-fit` ships its own copy, which differs only in the copyright
  lines (`LICENSE.addon-fit`).
- Served as one script, `xterm.js` followed by `addon-fit.js`, under a name
  that carries a hash of the embedded bytes (`terminal_routes.go`), so a new
  version is a new file name instead of a changed one. A session page loads it
  when its first terminal card opens. The source maps the scripts name are not
  vendored.
- Update: `npm pack @xterm/xterm@<5.x.y> @xterm/addon-fit@<0.x.y>`, copy
  `package/lib/xterm.js`, `package/css/xterm.css`, `package/LICENSE`, and the
  addon's `package/lib/addon-fit.js` and `package/LICENSE` (as
  `LICENSE.addon-fit`) here, and bump the versions above.
