# Release sweep repair: e280db38b..3732c199d findings

Context: the release-gate sweep over
`e280db38b0a95219aca3bfdfc225fbd90f435675..3732c199d1d5a0aa2578c2bccfc0c484eeca9aa6`
returned NEEDS FIX. The full findings, with file/line evidence, are in
`/home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/9b28e4a5-01-release-sweep-e280db38b-3732c199d.md`.
Read it first, then the done ticket
`ai-docs/tickets/.done/261029-feat-claude-mod-package-parity-knob-defaults.md`
(decisions D1-D15) and the commits 3e6b62c86 and 3732c199d with their
`## AI Context`.

## Scope (user-approved)

Fix I1-I4 and M1-M7 from the findings file. M8 and M9 are deferred follow-ups;
do not touch them.

Settled decisions:

1. I1: a late manual gate open (`manual.opened()` after `retryManual`) never
   replaces a key the model already holds. When the mailbox watcher carries a
   model-bound key, the late setup adopts that key (re-login with it, as the
   adoption rebuild path does) instead of minting; it mints and auto-binds
   only when no key is bound. Watcher, band rows, and the store entry all
   stay on the adopted key.
2. I2: an older session.start's mailbox setup never assigns or binds a
   watcher after a newer start began (re-check `startSeq` before the
   assignment, or stop the previous watcher on assignment - your choice).
3. I3 (also the user's earlier item (34)): no claude-mod string spells the
   package name. Every `ws `-prefixed label, prompt prefix, transcript
   notice, and debug log prefix (timing.ts `LABEL`, compaction.ts notices,
   register.ts/band.tsx/captions.tsx/manual.ts log prefixes) derives from the
   plugin name (`$.plugin.name`, or a label parameter passed in where a
   module has no `$`). Keep the ws rendering byte-identical where the plugin
   name is `ws`.
4. I4 and M4/M6/M7: add the tests the findings file lists; each new guard
   test must fail when its guard is removed (state in the commit which
   mutations you checked).
5. M1/M2/M3: the doc and manifest description fixes as described. M5:
   isolate the revive-nudge Python tests from an enclosing repo (for example
   `GIT_CEILING_DIRECTORIES` on the subprocess).

Mirror every changed claude-mod file and `compact-revive-nudge.py`
byte-identically into `agents-plugin-wsflow/` per
`ai-docs/manuals/wsflow-mirroring.md`. The two `config-manifest.json` files
keep identical text apart from `default`. Shipped wsflow text never names ws.

## Verification

- `claude plugin test agents-plugin` and `claude plugin test agents-plugin-wsflow`
- `claude plugin validate` on both packages
- `go test ./...` in `agents-plugin-tool` (run it in the background if it
  outlasts the foreground limit)
- `python3 -m unittest discover agents-plugin/tests` and `agents-plugin-wsflow/tests`
- `diff -r agents-plugin/hooks/claude-mod agents-plugin-wsflow/hooks/claude-mod` is empty

## Return boundary

Commit on `develop` in `/home/swkang/devenv` (one commit, or a few logical
ones). Do not push. Stop and report before changing anything beyond the
findings above, or if I1 needs a contract change beyond "adopt the bound key,
mint only when none".
