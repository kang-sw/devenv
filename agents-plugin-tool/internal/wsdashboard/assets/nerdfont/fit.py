"""Fit Symbols Nerd Font Mono's glyphs into a narrow terminal cell.

Usage: python fit.py SymbolsNerdFontMono-Regular.ttf WsNerdSymbols-Regular.woff2
(needs fontTools and brotli).

The release's Mono font draws every glyph in a 1em cell (2048 of 2048
units). xterm.js's DOM renderer draws each character at the font's own size
and only corrects its advance (letter-spacing) to the page font's cell,
about 0.55-0.62em, so a 1em glyph spills into the next cell, whose
background paints over it: icons and separators show cut in half. This
script redraws every glyph inside a cell of NARROW units:

- Powerline separators (U+E0B0-E0D7) are squeezed horizontally to NARROW,
  full height kept. One that leans on its right edge (it bleeds past the
  1em cell, or it starts well inside and ends at the edge) ends at WIDE, the
  widest cell expected, so on any cell up to WIDE its flat side reaches the
  next segment; what passes the cell is under the next cell's background,
  which is the same colour. The rest start at the left edge (keeping their
  bleed into the previous segment) and end at NARROW, so their point never
  passes the cell; a wider cell shows the next segment's colour after it.
- Every other glyph (icons) is scaled uniformly by NARROW/2048, or less when
  its ink is wider than that, centred in the NARROW cell around the font's
  vertical centre.

Hinting is dropped (the outlines moved) and the font is renamed, since it is
no longer the release's font.
"""

import sys

from fontTools.pens.boundsPen import BoundsPen
from fontTools.pens.transformPen import TransformPen
from fontTools.pens.ttGlyphPen import TTGlyphPen
from fontTools.ttLib import TTFont

EM = 2048
NARROW = 1126  # 0.55em: Consolas, the narrowest cell in the page's mono stack
WIDE = 1280  # 0.625em: SF Mono (0.618em) and DejaVu Sans Mono (0.602em) fit
MID = (1638 - 410) // 2  # the vertical centre of the font's ascent and descent
TOL = 16
FAMILY = "WsNerdSymbols"


def main(src, dst):
    f = TTFont(src, recalcTimestamp=False)
    gs = f.getGlyphSet()
    glyf, hmtx = f["glyf"], f["hmtx"]
    cps = f["cmap"].buildReversed()
    fitted = {}
    for name in f.getGlyphOrder():
        if glyf[name].isComposite():
            raise SystemExit(name + ": composite glyphs are not handled")
        bp = BoundsPen(gs)
        gs[name].draw(bp)
        if bp.bounds is None:
            continue
        x0, _, x1, _ = bp.bounds
        if any(0xE0B0 <= c <= 0xE0D7 for c in cps.get(name, ())):
            sx = NARROW / EM
            right = x1 > EM + TOL or (x0 > EM / 4 and x1 >= EM - TOL)
            m = (sx, 0, 0, 1, WIDE - EM * sx if right else 0, 0)
        else:
            s = min(NARROW / EM, NARROW / (x1 - x0))
            m = (s, 0, 0, s, NARROW / 2 - (x0 + x1) / 2 * s, MID - MID * s)
        pen = TTGlyphPen(None)
        gs[name].draw(TransformPen(pen, m))
        fitted[name] = pen.glyph()
    for name in f.getGlyphOrder():
        if name in fitted:
            glyf[name] = fitted[name]
        g = glyf[name]
        g.recalcBounds(glyf)
        hmtx[name] = (NARROW, getattr(g, "xMin", 0))
    for tag in ("cvt ", "fpgm", "prep", "PfEd"):
        if tag in f:
            del f[tag]
    f["OS/2"].xAvgCharWidth = NARROW
    names = f["name"]
    for rec in list(names.names):
        if rec.nameID in (1, 3, 4, 6, 16, 17, 21, 22):
            names.removeNames(nameID=rec.nameID)
    for nid, val in ((1, FAMILY), (2, "Regular"), (3, FAMILY + "-Regular;fitted from Symbols Nerd Font Mono"),
                     (4, FAMILY + " Regular"), (6, FAMILY + "-Regular")):
        names.setName(val, nid, 3, 1, 0x409)
        names.setName(val, nid, 1, 0, 0)
    f.flavor = "woff2"
    f.save(dst)


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
