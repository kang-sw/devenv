## Command syntax

`commands` is one string of commands in standard XML. Every tag carries the
prefix its tool's section names (for example `wb:`), with no namespace
declaration and no root element.

- Text and attribute values → escape `<` as `&lt;` and `&` as `&amp;`. The
  decoded text is what is stored; for a Markdown field, that is the Markdown
  source the page renders.
- Text that must go in unchanged (HTML or SVG mockups, code, XML examples,
  text copied from the board for an exact match) → wrap it in
  `<prefix:raw>...</prefix:raw>`. Everything up to the first closing
  `prefix:raw` tag is taken as written, CDATA included.
- Quote attribute values with `"` or `'`. A tag with no content may
  self-close.
- A tool may name one group element, whose attribute applies to the commands
  inside it. Groups do not nest.
- Commands apply in order. Any problem refuses the whole batch, and the
  refusal lists every problem with its position and line.
