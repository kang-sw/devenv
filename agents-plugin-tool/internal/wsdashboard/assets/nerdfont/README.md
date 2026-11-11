# Vendored Symbols Nerd Font Mono

- Source: the Nerd Fonts release `v3.5.1`
  (https://github.com/ryanoasis/nerd-fonts/releases/tag/v3.5.1), archive
  `NerdFontsSymbolsOnly.zip`
  (sha256 `fdca3682534f6f65e1ccb2345b0362ccf67d9b8eca7c8025330946e93e2473bc`),
  file `SymbolsNerdFontMono-Regular.ttf`
  (sha256 `fe471e538392f51910faab985fa8e192a39dd3426125edd15b71b3680df0e749`).
- The release publishes no woff2, so the ttf was converted unchanged (no
  subsetting) with fontTools 4.68.0 and brotli:
  `python -m fontTools.ttLib.woff2 compress SymbolsNerdFontMono-Regular.ttf`.
  `SymbolsNerdFontMono-Regular.woff2` sha256
  `b9e4e6812ea40d1a4679ecc349a74d38792acc8382f235c30b69a820bfef4109`.
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

- Update: download `NerdFontsSymbolsOnly.zip` from a newer release, convert
  its `SymbolsNerdFontMono-Regular.ttf` as above, copy its `LICENSE`, and
  update the tag, hashes, and icon-set table here.
