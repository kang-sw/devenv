## Command syntax

`commands` is one string of flat commands. Every tag carries the literal
prefix its tool's section names, with no namespace declaration and no root
element.

- A body is raw text up to the first exact closing tag of its element, so
  `<`, `&`, HTML, and mermaid are written as is, never escaped. A body that
  must contain its own closing tag puts that text in `<![CDATA[...]]>`. A
  body loses one newline right after its opening tag and one right before
  its closing tag; nothing else is trimmed.
- An attribute value is quoted with `"` or `'` and is not decoded either, so
  a value holding `"` is quoted with `'`. A tag with no content may
  self-close.
- Text between commands or between a command's fields, an unknown tag, and
  an unknown attribute are refused.
- Commands apply in the order written. A batch is validated whole: any
  problem refuses all of it, and the refusal lists every problem with its
  command position, tag, and line, then repeats these rules and the tool's
  command section.
