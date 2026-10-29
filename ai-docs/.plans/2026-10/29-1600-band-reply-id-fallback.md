# Status band: show the reply id when the key owns no named inbox

## Goal (user-approved, reverses one status-band decision)

`261028-feat-claude-mod-status-band` decided "the mailbox capsule shows the
key's own named-inbox address and is absent when the key owns none". The user
now wants the capsule to fall back to the key's reply id: in practice they hand
a session's `id:<...>` reply id to another session to start a link, and most
sessions own no named inbox, so the capsule never shows.

`mailbox_lookup_peers` `format: "json"` returns `self.address` only when the
key owns a named inbox, and `self.reply_id` (e.g. `id:gnarlyladylikeupchuckcrusher`)
otherwise.

## Settled behavior

1. The band's mailbox capsule shows `self.address` when present (unchanged
   look), else `self.reply_id`, drawn dimmer than a named address so the two
   are distinguishable; absent only when neither is present or the read fails.
2. Only the band's address read changes. `selfAddress()` as used by the
   mailbox watcher's slug resolution (register.ts, the `WS_MAILBOX` fallback
   path) keeps returning the named address only; do not let a reply id become
   a watcher slug.
3. Refresh timing, read failures and key switches behave as today.

## Scope

- `agents-plugin/hooks/claude-mod/register.ts` (the band `address` edge, and a
  small helper beside `selfAddress` if that is the cleanest shape) and
  `band.tsx` (capsule rendering and the `BandData` field/doc comment).
- Tests in the claude-mod suite: named address shown as before; reply id
  fallback shown dimmed; neither -> capsule absent; watcher slug still ignores
  the reply id.
- Update the band header comments that state the named-inbox-only rule.
- Mirror every changed claude-mod file byte-identically into
  `agents-plugin-wsflow/hooks/claude-mod/` per
  `ai-docs/manuals/wsflow-mirroring.md`.
- Do not edit the done ticket; record the reversal in the commit's
  `## AI Context`.

## Verification

- `claude plugin test agents-plugin` and `claude plugin test agents-plugin-wsflow`
- `diff -r agents-plugin/hooks/claude-mod agents-plugin-wsflow/hooks/claude-mod` empty

## Return boundary

Commit on this worktree's branch `delegate/band-reply-id-fallback`. Do not
merge or push. Stop and report if the change needs anything beyond this scope.
