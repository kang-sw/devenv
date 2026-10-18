---
title: Revive key adoption and mailbox auto-bind in the Claude mod
related:
  261018-feat-claude-mod-ws-manual-injection: the merged feature this extends; its minted conversation key is what a revive replaces and what the mailbox push binds to
  261017-feat-claude-mailbox-active-push-mod: the mailbox push this binds automatically; its bind-through-the-tool decision is narrowed here for sessions whose key the mod owns
  261017-bug-claude-mailbox-mod-post-drain-robustness: the bind, supersede, and session.end ordering rules the automatic bind reuses
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 6b98cd8a9a9dbc83
sage-review-completeness-reviewed: 6b98cd8a9a9dbc83
---

# Revive key adoption and mailbox auto-bind in the Claude mod

## Background

The Claude mod now mints one lead session key per conversation in a ws
project and puts the keyed workflow manual in the system prompt
(261018-feat-claude-mod-ws-manual-injection). Two gaps remain against pi:

- **Revive does not switch the key.** When the lead calls `workflow_manual`
  with another key (for example `ws:lead-revive` with a preserved key), the
  mod passes the result through whole and keeps its minted key: the `$.store`
  entry still names the minted key, so a later `--resume` re-logs in with it,
  and the next snapshot refresh (compaction) re-injects a section that names
  the minted key while the lead works under the revived one. pi adopts the
  revived key (`agents-plugin-pi/src/bridge.ts` `revivedKeyToAdopt`,
  `reloginLeadKey`, `adoptRevivedKey`): it re-logs in with it, makes it the
  default key, records it for later resumes, appends a notice, and re-arms the
  mailbox waiter through `onDefaultKeyAdopted`.
- **Mailbox push is a manual lever.** The lead binds the push by calling the
  mod's `mailbox_watch` tool with its key. Where the mod owns the key, it can
  bind the push itself, as pi arms its waiter for the lead.

The user asked for both together (2026-10-18), since pi ties the mailbox
re-arm to key adoption.

## Decisions

- **Adopt a revived key, as pi does.** A main-thread `workflow_manual` call
  (no `agentId`) whose `session_key` is an explicit key, not the
  fresh-bootstrap sentinel (`obsidian-latch`), and different from the
  conversation's current key, triggers adoption after the call returns: the
  mod re-logs in with that key through `ferrule` (`relogin_session_key`, root
  from `$.session.root()`, the existing `relogin` helper in `manual.ts`). On
  success the key becomes the conversation's key, the `$.store` key entry is
  rewritten with it (so `--resume` re-logs in with it), the pinned adoption
  notice is appended to the call's result, and the mailbox push re-binds to
  it. The conversation's current key switches at re-login success, tracked
  apart from the cached section's key, so a second call with the adopted key
  while the rebuild runs is not adopted again. A refused re-login adopts nothing and leaves the result as it was; a
  refused or errored `workflow_manual` result triggers no re-login at all.
  Adoption runs only under the manual feature's gate (ws package and
  `ai-docs/WORKFLOW.md`), and also when the conversation has no key yet (a
  failed mint), so a revive still gives the conversation a key. Rejected:
  leaving the pass-through as is (the stale-key section and resume above).
  (user, 2026-10-18)
- **The section follows the adopted key at the next refresh.** Adoption
  rebuilds the cached section with the adopted key in the background by
  re-running the mint's setup against the rewritten store entry (a re-login,
  so the rebuilt cache reads as reused and the flag stays false until a
  landing; a failed rebuild recovers at the next first compose through the
  same lazy start), so the first compose after the
  next refreshing reset injects a section naming the adopted key; the
  existing first-compose wait covers a rebuild still running. A refreshing
  reset is a manual `/compact` or a landed compaction request (the lever),
  as the manual feature already defines (`landingRefreshes`); native auto
  compaction composes nothing (the manual feature's live dogfood: across six
  auto compactions the requests kept the snapshot), so after one the section
  still names the previous key until the next `/compact` or compaction
  request. That gap is accepted: the compaction summary carries the key the
  lead works under, and a revive with it is a no-op adoption. Rejected: a
  re-snapshot right away (no host trigger short of compaction or `/clear`, per
  the research). (user, 2026-10-18; refreshing-reset scope settled under the
  user's 2026-10-18 directive after design review)
- **The snapshot flag goes false at adoption.** The current snapshot still
  carries the section for the previous key, so the state-only rewrite stays
  off until the next refreshing reset sets the flag from the rebuilt section;
  meanwhile the adopted key's `workflow_manual` calls return in full. Will
  clear the flag over keeping it - the existing span guard would already pass
  most calls through, but an explicit false keeps the rule "the flag is true
  only when the snapshot carries the cached section" literal, and a false
  flag only costs a full result. (technical, user directive 2026-10-18)
- **The sentinel is never adopted**, as on pi: a `workflow_manual` call with
  `obsidian-latch` mints a separate key in ws-mcp and the mod does not follow
  it. Rewriting sentinel calls to the conversation key (pi fills omitted and
  sentinel keys) needs the session-key omission the user dropped from scope;
  a lead that bootstraps despite the injected key line keeps working under its
  own key, with the push bound to the conversation key, as a known limitation.
  (technical, user directive 2026-10-18)
- **Mailbox push binds automatically where the mod owns the key.** Under the
  manual feature's gate, every key the conversation gets - minted, re-logged
  in on resume or hot reload, minted after `/clear`, or adopted - is bound to
  the mailbox push through the existing bind path (`arm` in `register.ts`,
  with its supersede and slug-lookup rules), driven by a key listener the
  manual feature calls once per key change (pi's `onDefaultKeyAdopted`
  shape). The listener is synchronous and only records the latest key; the
  bind runs from it through `arm` once the watcher exists, and a key that
  arrives before `mailboxSetup` created the watcher is bound when it does
  (latest key wins, through `armSeq`); the `session.end` mailbox step clears
  a recorded key that was never bound. The automatic bind writes no
  `mailbox-watch:` store entry; the
  restore after a hot reload comes from the manual feature's re-login
  instead. Accepted cost: every ws-project session runs one mailbox wait.
  (user, 2026-10-18)
- **No `mailbox_watch` tool where the mod owns the key.** Under the manual
  feature's gate the mod does not register `mailbox_watch`, so `mailboxSetup`
  awaits the gate (a filesystem check) before it registers. The gate is one
  promise evaluated once per module instance and shared by both features; a
  rejected check counts as closed for both (the lever registers, nothing is
  minted), so a session never holds both the lever and the automatic bind.
  Will share one gate over the manual feature's current re-check at a later
  compose - two evaluations could disagree and leave both mechanisms on.
  (technical, user directive 2026-10-18) Where the gate is
  closed (the wsflow package, a project without `ai-docs/WORKFLOW.md`) the
  lever registers and behaves exactly as today. A gate-open session whose mint
  fails has no key, no section, and no lever; the lead takes the playbook's
  background-wait fallback. Rejected: removing the lever everywhere (wsflow
  and non-ws projects mint no key, so they would lose push); keeping the lever
  registered as an idempotent no-op in ws projects (the user chose to retire
  the manual lever there). (user, 2026-10-18)
- **The section says the push is bound, and the Claude mailbox playbook
  reads it.** When the push is available (the existing `python3` probe
  passed; the manual setup awaits the probe outcome as an edge, inside the
  first compose's bounded wait), the section carries the pinned mailbox line
  after the key line; the
  shipped `agents-plugin/rsrc/lead-use-mailbox/arm-the-wait.claude.md` gains a
  first branch telling a lead whose system prompt says so to launch nothing,
  ahead of the existing tool branch and background-wait fallback. Without
  that branch a ws-project lead, finding no tool, would launch a second,
  background wait. Will key the playbook on the system prompt line over
  keying it on the tool's absence - an absent tool also means "no push here"
  (desktop without `$.process`, a failed mint), where the fallback wait is
  right. (technical, user directive 2026-10-18)
- **Lifecycle ordering is unchanged.** `session.end` stops the watcher and
  resets the manual state in their existing isolated steps; a key change that
  lands after a session end (a rebuild or adoption still in flight) binds
  nothing, through the manual generation counter and the watcher's `armSeq`.
  Adoption also bumps the manual generation, and the setup re-checks the
  generation before its store write and before calling the key listener, so a
  mint or re-login still running when the lead revives never overwrites the
  adopted key in the store or binds the push back to the old key.
  (technical, user directive 2026-10-18; 1ed6398be ordering)

## Constraints

- Known limitation: a session can run a second, shell mailbox wait beside
  the push - a gate-open session whose mint failed (the lead took the
  background-wait fallback) and that gets a key later, or a `--resume` of a
  conversation recorded before this change (its replayed snapshot has no
  mailbox line). Harmless: the mod drains, and the shell wait wakes to
  nothing. (technical, user directive 2026-10-18)
- Out of scope: session-key omission and sentinel rewriting; changing the
  `mailbox_watch` lever where the gate is closed; adoption in the wsflow
  package (the manual feature is ws-only). (user, 2026-10-18)
- The 261018 manual-injection, 261018 reconnect, 261017 compaction, and 261017
  mailbox Decisions stay in force except where this ticket's Decisions narrow
  them (the bind-through-the-tool rule, for gate-open sessions only).
- The wsflow `claude-mod/` copy stays byte-identical; the features stay inert
  there.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/hooks/)

## Pinned Prose

Asserted verbatim by tests. `{key}`, `{previous}`, `{current}` are session
keys; `{previous}` reads `(none)` when the conversation had no key.

Section mailbox line, after the key line and a blank line, only when the push
is available:

```text
Mailbox push is bound to this key automatically: arriving mail starts your next turn even when you are idle, so launch no mailbox wait. If a revive switches your key, the push follows it.
```

Adoption notice, appended to the adopting `workflow_manual` result after a
blank line:

```text
[ws] Lead session key changed: {previous} -> {current}. Keep passing {current} as session_key; this conversation resumes with it, and the system prompt names it after the next /compact or compaction request.
```

New first branch of `arm-the-wait.claude.md` (the existing branches follow,
renumbered; the final "Either wake path" branch is reworded to cover the
automatic push as well, following `skill-authoring.md`):

```text
1. If your system prompt says mailbox push is bound to your session key
   automatically, launch nothing from this section: arriving mail already
   starts your next turn.
```

## Prior Decisions

- 261018-feat-claude-mod-ws-manual-injection (2026-10-18, 839c014d): "Known gap accepted for a follow-up feat: no revive key adoption (pi adoptRevivedKey parity) and no mailbox auto-bind; the user chose to merge first and ticket both together." — bearing: supports
- 261018-feat-claude-mod-ws-manual-injection (2026-10-18, 1ffd64e1): "A refused re-login mints a new key but keeps reused=true: the snapshot is still the replayed one, so the flag stays false." — bearing: constrains
- 261018-feat-claude-mod-ws-manual-injection (2026-10-18, 23cc9f5c): "The span guard binds the cut to the minted key, which only the injected section reveals" (resumed/forked conversation replays a prompt without the section) — bearing: constrains
- 261018-feat-claude-mod-ws-manual-injection (2026-10-18, 1ffd64e1): "The key binds to $.session.root() (ws-mcp canonicalizes to the worktree toplevel), not $.session.repo().root" — bearing: supports
- 261017-feat-claude-mailbox-active-push-mod (2026-10-17, Decisions): "Explicit bind. The agent binds by calling a mod-registered tool with its own session key; the binding is not inferred from other ws tool calls." — bearing: constrains
- 261017-feat-claude-mailbox-active-push-mod (2026-10-17, Decisions): "Tool availability. The mod registers its bind tool only where $.process is available, so the Claude arm-the-wait 'if available' branch falls back to the shell wait elsewhere." — bearing: constrains
- 261017-bug-claude-mailbox-mod-post-drain-robustness (2026-10-17, 6f2527cb): "One arm sequence counter serves overlapping binds, the restore's 'no newer bind' check, and stop-cancels-pending-bind; a lock was rejected per the ticket." — bearing: constrains
- 261017-bug-claude-mailbox-mod-post-drain-robustness (2026-10-18, 00cc0a25): "session.end keeps the watcher reference, so stopWatcher's armSeq bump is the only cancel for an in-flight bind" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/claude-mod/manual.ts, agents-plugin/hooks/claude-mod/register.ts, agents-plugin/hooks/hooks.json, agents-plugin/rsrc/lead-use-mailbox/arm-the-wait.claude.md, ai-docs/manuals/wsflow-mirroring.md, agents-plugin-wsflow/hooks/claude-mod/ copy, plus the claude-mod tests |
| scope.surface | public-interface | model-facing: the mailbox_watch tool is no longer registered when the gate is open, the section gains a pinned mailbox line, workflow_manual results gain a pinned notice, and a shipped playbook branch is added |
| scope.new_public_symbol | no | no exported symbol is named; a key listener edge is added to the internal Manual interface in manual.ts |
| scope.new_type_contract | yes | the key listener signature on the Manual interface (manual.ts) called once per key change, with previous reading none when absent |
| scope.test_surface | existing | agents-plugin/hooks/claude-mod/manual.test.ts, manual-engine.test.ts, engine.test.ts, watcher.test.ts extended (engine tests named in Verification) |
| complexity.reuse_points | confirmed | relogin helper (manual.ts#L321-L330), arm with armSeq supersede (register.ts#L287-L291), mailboxSetup gate/probe (register.ts#L320-L330) all read |
| complexity.side_effect_risk | high | every ws-project session starts one mailbox wait and the lever disappears there; the bind runs across async setup, adoption, rebuild, and session.end |
| risk.correctness | high | ordering between manual generation, armSeq, store rewrite, and session.end across in-flight rebuilds and adoptions |
| risk.fit | moderate | follows pi adoptRevivedKey and the existing arm path, but mailboxSetup must now await the manual gate, which is independent today |
| risk.test | moderate | engine harness covers the paths, but race and ordering cases need mutation-style tests |
| risk.security_or_contract | moderate | a model-supplied key triggers a ferrule re-login (ws-mcp accepts only a parent-less lead key bound to root); pinned prose and the lever contract change |

## Phases

### Phase 1: Key adoption and mailbox auto-bind

In `agents-plugin/hooks/claude-mod/`: adoption and the key listener in
`manual.ts`; the gate-dependent lever registration, the automatic bind, and
the section's mailbox line wiring in `register.ts` (the section needs the
`python3` probe outcome, so the first compose's existing bounded wait also
covers it). Update the `register.ts` header comment, the ws `hooks.json`
description, and the Claude Mod paragraph of
`ai-docs/manuals/wsflow-mirroring.md` for the changed mailbox behavior. Add
the pinned branch to `arm-the-wait.claude.md` following `skill-authoring.md`
and mirror it as `wsflow-mirroring.md` directs. Resync the wsflow
`claude-mod/` copy byte-identically.

Verification:

- `claude plugin validate` and `claude plugin test` pass for both packages,
  with engine tests for: adoption on a main-thread revive with another key
  (re-login, store entry rewritten, notice verbatim, push re-bound); no
  adoption for a subagent call, the sentinel, the current key, or a refused
  re-login or an errored call; adoption when no key was minted; an adoption
  during an in-flight mint or re-login keeping the adopted key in the store
  and the push; a key produced before the watcher exists bound once it does;
  one shared gate, with a rejected check registering the lever and minting
  nothing; the rebuilt section naming the
  adopted key at the first compose after the next landing, and the flag false
  until then; the mailbox line present only when the push is available;
  automatic bind at mint, at re-login on resume and hot reload, and at the
  lazy mint after `/clear`; no bind after a session end lands mid-setup; no
  `mailbox_watch` tool when the gate is open, and the lever unchanged when it
  is closed (wsflow, no `ai-docs/WORKFLOW.md`); and the existing mailbox,
  compaction, reconnect, and manual tests passing (adjusting only those that
  assumed the lever in a gate-open session).
- `python3 -m unittest discover` passes in `agents-plugin/tests` and
  `agents-plugin-wsflow/tests`, and `go test -race ./internal/wsrsrc/` passes
  under `agents-plugin-tool/` (hooks mirror and rsrc drift).
- Live dogfood in this repository (opus, interactive): the section carries
  the mailbox line; mail sent to the minted key from another session starts a
  turn with no `mailbox_watch` call; `ws:lead-revive` with another valid lead
  key appends the notice, mail to the new key starts a turn, and after a
  `/compact` or compaction request the section names the new key; `--resume` of that session
  re-logs in with the adopted key.

### Result (6cfcd48f3) - 2026-10-18

Landed in 88187affe (feature) and 6cfcd48f3 (lite-review fixes).

- `manual.ts`: `revivedKeyToAdopt`, `adoptionNotice`, `MAILBOX_LINE`,
  `sectionText(key, manual, push)`; adoption after the state-only/dedupe
  rewrite (re-login, conversation key switch, key listener, entry rewrite,
  flag false, rebuild); the generation counter now also bumps at adoption and
  the setup re-checks it before its entry write and before the listener.
  The adoption's rebuild is pending before the entry rewrite and re-logs in
  with the adopted key itself, never minting.
- `ManualEdges` gains `keyChanged(current, previous)` (synchronous) and
  `pushAvailable()`; the ticket's "key listener on the Manual interface" is
  an edge bound at start (cosmetic).
- `register.ts`: one `wsGate` promise per module instance shared by both
  features (a rejected check reads as closed); `mailboxSetup` awaits it after
  creating the watcher and registers no `mailbox_watch`, no store entry and
  no restore when it is open; `autoKey` records the latest key and binds
  through `arm` once a watcher exists; `session.end` clears it.
- `arm-the-wait.claude.md` gains the pinned first branch (last branch
  reworded); rsrc manifest, wsflow rsrc and pi rsrc mirrors regenerated; the
  Go arm-wait composition test pins the new branch and its order. Header
  comment, ws `hooks.json`, and the `wsflow-mirroring.md` Claude Mod paragraph
  updated; wsflow `claude-mod/` resynced byte-identically.

Verification: `claude plugin validate` passes for both packages;
`claude plugin test` 226 pass / 0 fail in `agents-plugin` and
`agents-plugin-wsflow`; `python3 -m unittest discover` OK in
`agents-plugin/tests` (78) and `agents-plugin-wsflow/tests` (15);
`go test -race -count=1 ./internal/wsrsrc/` ok. A mutation removing the
`session.end` `autoKey` clear fails the new engine test.

Decisions: the existing "gate asked again by the first compose" unit test now
pins "closed for good" (ticket decision); two manual-engine tests that passed
another known key through now use a key ws-mcp refuses; engine tests holding
a live wait child carry a 20 s timeout (about a second of real time per
settle in the kit). Not done: the live dogfood (interactive opus session) is
left to the lead/user.

## Sage Review Round 1 (2026-10-18)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | The adoption notice promises a system-prompt refresh that native auto compaction does not deliver | important | missing |
| 2 | An adoption can race with the initial setup | minor | autonomous |
| 3 | Key listener and section line need the mailbox setup's readiness | minor | autonomous |
| 4 | No rule for whether the lever registers when the gate check rejects | minor | autonomous |
| 5 | Some sessions can end up running two mailbox waits | minor | autonomous |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|
| 1 | Key listener signature left to the implementer | minor |
| 2 | Adoption when the triggering workflow_manual call itself errored | minor |
| 3 | Phase 1 bundles several sub-deliverables | minor |
