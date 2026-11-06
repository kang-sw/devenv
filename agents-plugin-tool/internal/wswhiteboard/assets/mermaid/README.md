# Vendored mermaid

- Package: `mermaid` 11.17.2 from npm, file `dist/mermaid.min.js` (the
  single-file IIFE build that sets `globalThis.mermaid`).
- License: MIT, see `LICENSE` beside this file (copied from the package).
- Why the IIFE build: browsers block ES module scripts over `file://`, and the
  whiteboard pages are opened as local files.
- Update: `npm pack mermaid@<11.x.y>`, copy `package/dist/mermaid.min.js` and
  `package/LICENSE` here, and bump the version above. The shared file name the
  pages load carries a hash of the embedded bytes, so a new version is written
  beside the old one instead of replacing it.
