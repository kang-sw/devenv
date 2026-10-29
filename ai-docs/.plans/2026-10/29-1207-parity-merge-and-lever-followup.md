# Finish the parity merge, then make the compaction lever always available

Context: ticket `261029-feat-claude-mod-package-parity-knob-defaults` (now in
`ai-docs/tickets/.done/`) landed on `impl/develop/baggy-wrath-chant`. The lead
started `ws/git.merge` of that branch into `develop` in this checkout
(`/home/swkang/devenv`); it stopped on conflicts. Read the ticket for its
decisions (D1-D15) before changing the mod.

## Step 1: complete the merge

The merge is in progress on `develop` with conflicts only in
`agents-plugin/hooks/claude-mod/band-engine.test.tsx` and its byte-identical
wsflow mirror `agents-plugin-wsflow/hooks/claude-mod/band-engine.test.tsx`.

- Develop's side (13da9ce56, status band bar label rework) owns the bar
  assertions: keep its `barRuns` expectations and the split bar tests.
- The branch side owns test names and world setup: features are now selected by
  knob, not package name, so keep its renamed tests and world options
  (`wsProject: true, manual: true` etc.).
- Resolve the ws copy, copy it byte-identically over the wsflow copy, run the
  verification below, and commit the merge (`git commit` with the prepared
  merge message, keeping an `## AI Context` explaining the resolution).

## Step 2: follow-up commit (user-approved)

1. `request_compaction` (the compaction lever) is registered always, inside
   session.start's awaited path, in every package; no knob read gates it. The
   lever and the compaction execution it needs (turn-end compaction, the
   resume prompt, the compacted handling) run regardless of the knob.
2. The knob narrows to the advisory nudge only: the usage-measured advisory
   and its reminders. Rename `claude.compaction_enabled` to
   `claude.compaction_advisory_enabled` (unreleased key, no migration) in both
   packages' `config-manifest.json`, keeping per-package defaults (ws true,
   wsflow false) and the D9 drift guard green; reword its description to say it
   gates only the advisory and reminders, and that the lever is always
   available. Update every reference (mod, tests, docs, ticket-free mentions in
   manuals/README).
3. This removes the stale-lever problem (a later session.start reading the knob
   off left the lever registered with no handler); make sure no path leaves a
   registered lever whose calls go unanswered.
4. Remove the now-unreachable `restoreSeq` guard in the mailbox setup
   (`register.ts`), since `mailbox_watch` registers only after server
   resolution; keep behavior otherwise identical.
5. `register.ts` compaction setup log line spells `ws compaction:`; use
   `${$.plugin.name}` like the reconnect line (the mirrored mod must not spell a
   package name).

Out of scope: the manual knob read retry after a failed first read (open with
the user), `mailbox_watch` registration timing (kept as is), band code.

Mirror every changed claude-mod file byte-identically into
`agents-plugin-wsflow/hooks/claude-mod/` per `ai-docs/manuals/wsflow-mirroring.md`.

## Verification (both steps)

- `claude plugin test agents-plugin` and `claude plugin test agents-plugin-wsflow`
- `claude plugin validate` on both packages
- `go test ./...` in `agents-plugin-tool`
- `python3 -m unittest discover agents-plugin/tests` and `agents-plugin-wsflow/tests`

## Return boundary

Two commits on `develop` (the merge commit, then the follow-up). Do not push.
Delete the merged branch `impl/develop/baggy-wrath-chant` only after the merge
commit exists (`git branch -d`). Stop and report if a conflict needs a
judgment beyond the rule in step 1, or the follow-up needs more than the scope
above.
