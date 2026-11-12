---
title: "Claude mod band tests fail under Claude Code 2.1.323: the band root's props.width reads undefined"
---

# Band tests fail under Claude Code 2.1.323

## Background

Observed 2026-11-13. `claude plugin test agents-plugin` on develop
(f01fed037, also 6faab3fc3) runs 773 tests: 759 pass, 14 fail, all in
`agents-plugin/hooks/claude-mod/band-engine.test.tsx`. The same suite
passed 773/773 earlier the same morning, before Claude Code auto-updated
from 2.1.322 to 2.1.323 (07:36 local).

Every failure stops at the first frame assertion,
`expectFrame` line 272: `expect(d.root.props?.width).toBe(W)` receives
`undefined` (for example, expected 80). The failing cases include the
status-row frame, ws rows, the mailbox capsule colour, narrow widths, bar
caps, and key adoption. The pure row-layout helpers still pass.

Hypothesis, unconfirmed: 2.1.323 changed the element tree the test
harness hands the band (where `width` lives, or the root's shape). It is not
yet known whether the live band renders wrong in 2.1.323, or only the test
harness view changed.

## Open

- Check the live band in a 2.1.323 session: does the frame still render at
  the right width?
- Find the 2.1.323 change in the mod element and test API, and fix the test
  helper or the band code to match. Keep 2.1.322 working if the harness
  version range still includes it.
- Check whether the wsflow mirror needs the same fix (the mod is mirrored).
