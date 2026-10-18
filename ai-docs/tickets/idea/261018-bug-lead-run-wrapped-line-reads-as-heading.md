---
title: "lead-run playbook: a wrapped line starting with ### reads as a heading"
related:
  261018-feat-claude-mod-ws-manual-injection: found while dogfooding its repeat-read pointer
---

# lead-run playbook: a wrapped line starting with ### reads as a heading

## Background

`agents-plugin/rsrc/lead-run/lead-run.md` wraps an inline code span so that a
continuation line begins with `### `:

```text
`Resume: branch <branch> at <head>; your last report is in the ticket's
### Result and the branch log.`
```

A Markdown reader treats that line as an ATX heading (`### Result and the
branch log.``), splitting the paragraph and breaking the code span. It surfaced
live in the Claude mod's repeat-read pointer, whose heading list (the
`#`-`###` lines of the result, the same rule as the pi bridge) carried the
stray entry. The model reading the playbook sees the same malformed structure.

## Phases

### Phase 1: Rewrap the line and guard against recurrence

Rewrap the sentence in `lead-run.md` (and its wsflow mirror) so no line starts
with `#`. Consider a cheap check in the existing rsrc tests that flags a line
starting with `#` inside a paragraph or inline code span, if one fits the
current test layout; otherwise the rewrap alone closes this.
