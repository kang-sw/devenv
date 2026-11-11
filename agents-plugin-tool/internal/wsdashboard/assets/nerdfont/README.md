# Vendored Symbols Nerd Font Mono, fitted to a terminal cell

- Source: the Nerd Fonts release `v3.5.1`
  (https://github.com/ryanoasis/nerd-fonts/releases/tag/v3.5.1), archive
  `NerdFontsSymbolsOnly.zip`
  (sha256 `fdca3682534f6f65e1ccb2345b0362ccf67d9b8eca7c8025330946e93e2473bc`),
  file `SymbolsNerdFontMono-Regular.ttf`
  (sha256 `fe471e538392f51910faab985fa8e192a39dd3426125edd15b71b3680df0e749`).
- `WsNerdSymbols-Regular.woff2` is that ttf redrawn by `fit.py` beside this
  file (fontTools 4.68.0 and brotli 1.2.1, no subsetting):
  `python fit.py SymbolsNerdFontMono-Regular.ttf WsNerdSymbols-Regular.woff2`.
  The output is reproducible: sha256
  `b3845c28dde5b102b4844c9298fb23ab5a5d6ee6ded9733ff2944cca93b197d7`,
  1,141,016 bytes.
- Why it is redrawn: the release's Mono font draws each glyph in a 1em cell,
  while a terminal cell in the page's monospace stack is 0.55-0.62em.
  xterm.js's DOM renderer corrects only a glyph's advance to the cell
  (per-span letter-spacing), so a 1em glyph spilled into the next cell and
  that cell's background painted over it: icons and separators showed cut
  in half. `fit.py` squeezes powerline separators into the cell at full
  height (the ones that lean on their right edge end at the widest expected
  cell, so they always meet the next segment) and scales icons uniformly to
  the narrowest expected cell; its docstring has the numbers. It also drops
  the hinting and renames the font `WsNerdSymbols`, since it is no longer
  the release's font.
- It holds only the Nerd Fonts symbols (powerline separators, file, folder,
  and branch icons, ...), each one cell wide. The terminal card lists it after
  the page's monospace stack, so letters keep the viewer's own monospace font
  and only the symbols that font lacks come from this one.
- Served under a name that carries a hash of the bytes (`terminal_routes.go`),
  declared by an `@font-face` in the stylesheet a page that offers terminals
  inlines; the browser fetches it only when a terminal first draws.
- License: the Nerd Fonts MIT license, see `LICENSE` beside this file (copied
  from the archive). The glyphs come from these icon sets, under their own
  licenses (from the archive's README):

  | Icon set               | Upstream                                              | License     |
  |------------------------|-------------------------------------------------------|-------------|
  | Codicons               | https://github.com/microsoft/vscode-codicons          | CC BY 4.0   |
  | Devicons               | https://github.com/devicons/devicon                   | MIT         |
  | extraglyphs            | https://github.com/source-foundry/Hack                | MIT         |
  | Font Awesome           | https://github.com/FortAwesome/Font-Awesome           | CC BY 4.0   |
  | Font Awesome Extension | https://github.com/AndreLZGava/font-awesome-extension | MIT         |
  | Font Logos             | https://github.com/lukas-w/font-logos                 | unlicensed  |
  | MaterialDesign         | https://github.com/Templarian/MaterialDesign-Font     | Apache 2.0  |
  | Octicons               | https://github.com/primer/octicons                    | MIT         |
  | Seti and original      | https://github.com/jesseweed/seti-ui                  | MIT         |
  | Pomicons               | https://github.com/gabrielelana/pomicons              | OFL 1.1 RFN |
  | Powerline Extra        | https://github.com/ryanoasis/powerline-extra-symbols  | MIT         |
  | Powerline Symbols      | https://github.com/powerline/powerline                | MIT         |
  | Power Symbols IEC      | https://github.com/jloughry/Unicode                   | MIT         |
  | Weather Icons          | https://github.com/erikflowers/weather-icons          | OFL 1.1     |

- Update: download `NerdFontsSymbolsOnly.zip` from a newer release, run
  `fit.py` on its `SymbolsNerdFontMono-Regular.ttf` as above, copy its
  `LICENSE`, and update the tag, hashes, size, and icon-set table here.
