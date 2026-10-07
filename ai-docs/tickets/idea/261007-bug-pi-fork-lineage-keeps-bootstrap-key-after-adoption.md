---
title: Pi fork lineage keeps the bootstrap key after revive adoption
---

# Pi fork lineage keeps the bootstrap key after revive adoption

## Background

Found in the review sweep of `bf3216981..c43ebc3cd` (Minor). After
`261007-bug-pi-session-key-reissued-on-resume`, a Pi lead that revives with a
different key adopts it: `adoptRevivedKey` (`agents-plugin-pi/src/bridge.ts`)
updates the bridge's `defaultSessionKeyRef`. The lead-bootstrap capture does
not follow:

- `agents-plugin-pi/src/index.ts` passes `registerLeadBootstrap` the plain
  `sessionKeyRef`, which is set once from the bootstrap key
  (`sessionKeyRef.current = handle.defaultSessionKeyRef.current`). The ref
  passed to the neighbouring registration two lines earlier is a getter over
  `handle.defaultSessionKeyRef` instead.
- `lead-bootstrap.ts` records `parentSessionKey: sessionKeyRef?.current` into
  the captured prompt and the `ws-pi-lead-prompt` entries.
- `fork.ts` and `ask.ts` prefer `captured.parentSessionKey ??
  bridge.defaultSessionKeyRef.current`, so a fork spawned after adoption
  names the abandoned bootstrap key as its `parentSessionKey` and in
  `WS_PI_PARENT_SESSION_KEY`.

The fork refusal guard still covers both keys through `parentSessionKeys`, so
this is a lineage defect, not an authority hole. It runs against the adoption
ticket's intent that the adopted key becomes the lead's single key.

## Direction

Likely pass `registerLeadBootstrap` the same getter over
`handle.defaultSessionKeyRef`, and test that a fork after adoption carries
the adopted key. Check whether an already-captured prompt from before the
adoption should be re-keyed or left as recorded.
