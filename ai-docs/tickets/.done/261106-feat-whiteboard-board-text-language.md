---
title: Whiteboard board text follows the conversation language
completed: 2026-11-06
---

# Whiteboard board text follows the conversation language

## Background

Dogfood surprise (2026-11-06, devenv lead session with `workflow.lang` =
Korean): on its first whiteboard renders the lead wrote item titles, options,
rationale, and the Now fragment in English, and the user had to ask for
Korean. The lead conflated the board with tickets and commits, which this
repository keeps in English.

The `whiteboard` playbook (`agents-plugin/rsrc/whiteboard/whiteboard.md`) says
nothing about the language of board text, and the manual's `### Whiteboard`
section does not either. The board is the lead's primary user-facing channel
while the knob is on, so the conversation language (`workflow.lang` when set,
otherwise the user's language) is the expected default; every lead that also
writes English tickets can make the same mistake.

Open for design: where the rule lives (the playbook's Calls/Payload text, the
manual section, or the `workflow.lang` line the manual already carries), and
whether fixed tokens (IDs, class tags, `ok`/`no` answers, option letters) are
named as staying untranslated.


## Resolution (2026-11-06)

Resolved by 8f2d513d1: one line in the whiteboard playbook's Payload section (mirrored to wsflow and Pi) says board text follows the conversation language.
