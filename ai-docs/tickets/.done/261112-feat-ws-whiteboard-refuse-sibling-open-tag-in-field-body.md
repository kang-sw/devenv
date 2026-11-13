---
title: "whiteboard.update refuses a field body that holds a sibling field's opening tag"
completed: 2026-11-13
---

# whiteboard.update refuses a field body that holds a sibling field's opening tag

## Background

Dogfood surprise 2026-11-12, item (420): a `wb:add` meant to carry three
options was written with the first option closed by `</option>` instead of
`</wb:option>`. A field body is raw text up to the first exact closing tag,
so the first option swallowed `</option>`, a newline, `<wb:option>` and the
second option's text; the sanitizer then dropped the two unknown tags. The
batch was accepted and the item showed option a), an unlettered line, then
b) for what was meant as c). The user noticed the broken lettering.

Idea: when a `wb:` field body contains the opening tag of a field the same
command accepts (`<wb:option>`, `<wb:title>`, `<wb:why>`, ...), refuse the
batch with the usual positioned diagnostic ("field body holds an opening
`<wb:option>` - missing `</wb:option>`?"), since that text is almost always
a missed close; a body that really needs the text wraps it in CDATA, which
the command syntax already supports.

## Phases

### Phase 1: Refuse sibling opening tags inside field bodies

Parser check in `agents-plugin-tool/internal/wswhiteboard` command parsing,
with a table test; CDATA keeps the literal text accepted.

### Result

Whiteboard container fields reject literal openings of fields accepted by the same command, with a positioned diagnostic; CDATA preserves literal openings. The shared parser enables this check only for the whiteboard grammar. Verified on w5090 with the whiteboard and MCP package tests and the full Go suite (known host-specific failures excluded).
