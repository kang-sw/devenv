# Harness badge font: Nunito ExtraBold (subset)

- Source: https://github.com/googlefonts/nunito, `fonts/variable/Nunito[wght].ttf`
  (sha256 `bb55a5ca5c2042335b3991af27c4d0705d0ef41cac6164ac737fd8f2a1e85207`).
  License: SIL Open Font License 1.1 (`OFL.txt` beside this file, copied from
  that repository).
- `Nunito-ExtraBold-subset.woff2` is that font instanced at wght 800 and
  subset to `ClPioAg?0123456789` (the harness badge labels) with fontTools and
  brotli (`fontTools.varLib.instancer`, then `fontTools.subset` with no layout
  features, woff2 flavor).
- Served under a name that carries a hash of the bytes (`harness_font.go`) and
  declared by the page's inline `@font-face` (family `WsHarnessBadge`).
