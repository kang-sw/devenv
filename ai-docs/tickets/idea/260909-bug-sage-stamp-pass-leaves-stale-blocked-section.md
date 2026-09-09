---
title: "sage_stamp pass after block leaves the stale Blocked section in the ticket body"
---

## Summary

`tickets.sage_stamp` appends a `## Blocked (<date>)` section when a verdict is
`block`, but a later stamp whose verdict resolves to `pass` only rewrites the
frontmatter posture: the section stays in the body. A `ready/` ticket then
carries `sage-review-*: completed` beside a heading that says it is blocked,
and a worker that reads the whole file (the worker protocol requires it) has
to guess which is current.

Observed 2026-09-09 during the refoundation epic's promotion batch: four
children stamped `block` in round 1, fixed, stamped `pass` in round 2, and all
four kept the round-1 `## Blocked` section. Worked around by renaming the
heading by hand to `## Sage Review Round 1 (<date>)` and re-stamping so the
body digest matched.

## Expected

On a pass-resolving stamp, `appendOrReplaceBlockedSection`'s counterpart
either removes the section or retitles it as history. Retitling is the better
default: the round-1 tables carry the finding-to-resolution record the commit
message otherwise has to repeat.

Touchpoint: `agents-plugin-tool/internal/wsdoc/tickets_sage.go` (the
`verdict == "block"` branch and the pass path just below it);
`appendOrReplaceBlockedSection` already knows how to find the section.
