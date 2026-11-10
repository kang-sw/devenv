# Vendored Lucide icons

- Package: `lucide-static` 1.71.0 from npm, files `icons/<name>.svg`, copied
  unchanged. Only the icons the dashboard draws are here.
- License: ISC (with an MIT part for the icons derived from Feather), see
  `LICENSE` beside this file (copied from the package).
- Use: the daemon builds one inline sprite from `icons/` (`lucide.go`), a
  symbol `l-<name>` per file. A page script draws one by its name, as a
  string literal (`WsCards.icon("<name>")`); a test fails when a script asks
  for an icon that is not here, or when an icon here is named by no script.
- Add an icon: `npm pack lucide-static@1.71.0`, copy
  `package/icons/<name>.svg` into `icons/`, and draw it by its name.
- Update: `npm pack lucide-static@<version>`, copy the same file names over
  `icons/` and `package/LICENSE` over `LICENSE`, and bump the version above.
