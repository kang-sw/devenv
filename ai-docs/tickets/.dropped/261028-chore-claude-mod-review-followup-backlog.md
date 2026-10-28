---
title: Work down the claude-mod hook review follow-up backlog
dropped: 2026-10-28
---
# Work down the claude-mod hook review follow-up backlog

## Background

Review sweeps on `develop` recorded these as `minor/review` commit follow-ups
(`ws/git.followups`), and none was ever resolved. A triage on 2026-10-28 at HEAD
`f400e5fbe` checked each against current code and classified it TEST (coverage
gap, behavior believed correct) or TIDY (duplication, naming, comment/wrap,
structure; no behavior impact); BUG and STALE entries were handled separately.
The creating commit resolves every id below, so this ticket is now the single
owner of the claude-mod backlog and the release gate no longer lists them.

Each row keeps the original follow-up id; `git log --grep=<id>` finds the
review commit that recorded it and its AI Context. Paths are relative to `agents-plugin/hooks/`; `mailbox-push/` was merged into `claude-mod/` by `42ab09388`, so its three rows were re-mapped there. Line numbers were
re-located at triage time and will drift. `(?)` marks a call the triager was
unsure of (the lower-impact class was chosen).

## Decisions

Open (decide before or at pickup):

- `credible-amid-patrol` - `claude-mod/captions.tsx` `DEFAULT_STYLE` embeds
  Korean examples and an "in Korean" clause. Should the shipped default be
  language-neutral (deriving the language from the user's setting) rather than
  this maintainer's language?
- `thievish-dodge-suggest` - a mixed caption group holding a failed call is
  captioned and its non-call rows hidden; a code comment calls this deliberate.
  Keep it, or surface failed calls uncaptioned?

## Phases

### Phase 1: Work through the claude-mod backlog

Re-check each row against the code at pickup; drop rows that no longer hold and
say so in the Result. Batch by file or module rather than by row. TEST rows add
tests that fail on the described regression; TIDY rows change no behavior. A
row that turns out to be a real defect is reported, not silently fixed inside a
tidy batch. Splitting this phase further is fine when the backlog is too large
for one worker pass.

#### TEST (11)

| id | current path:line | note |
|---|---|---|
| voicing-treason-existing | claude-mod/captions.tsx | still no captions.test.ts; only captions-engine.test.tsx |
| vividly-spouse-hacksaw | claude-mod/captions-engine.test.tsx | agentId/deny/off/CAPTURED_LIMIT exclusions, compacted() guards, exact PIVOT_HIGH/LOW, staged fileChange still ungrepped (fileChange itself covered at :752-761) |
| despite-agile-waking | claude-mod/compaction.test.ts:264 | corpus still hand-copied from internal/mcp/config_manifest_test.go (path moved), no drift guard |
| important-wronged-spousal | claude-mod/register.ts:~765 | turn.complete try/finally mainTurn reset; no engine test with rejecting hook beneath |
| mutable-obligate-manger | claude-mod/register.ts:~288,677 | session.end mainTurn reset untested (engine.test.ts:690 covers queued-mail record only); toolCompleted catch is defensive |
| discolor-implosion-crinkly | claude-mod/register.ts:~735-830 | lever .catch deny, measure/turn.complete catches, session.compact replay: no engine test |
| reflector-transfer-undefined | claude-mod/compaction-engine.test.ts:172 | ws/wsflow `isWs` guards; each package run covers half (gate-config dependency) |
| showman-conceal-resilient | claude-mod/engine.test.ts | no session.start re-entry-with-drain-in-flight test (only session.end, :535) |
| prison-shame-cogwheel | claude-mod/engine.test.ts:396 | identity test still thin; mailbox_watch not-connected / not-started deny branches (register.ts:~709-713) untested |
| lecturer-jackknife-alongside | claude-mod/captions.tsx:678 | `stopTicking ??= bound.every(...)`; refused period needs a host clock min >1500ms, no evidence; only an untested edge (?) |
| unexposed-easing-alarm | claude-mod/compaction.ts:~335 (runLever after compact()) | no await between compact() settling and hold(); gap is sub-microtask, no test pins turnStarted ordering (?) |

#### TIDY (17)

| id | current path:line | note |
|---|---|---|
| jingling-disorder-plant | claude-mod/register.ts:875 | ToolResult hook returns raw for isErrored (intended); ticket Drawing contract just doesn't say so (doc) |
| thievish-dodge-suggest | claude-mod/captions.tsx:307-316 | mixed group with failed call captioned (documented in comment, :303-306); undefined isCallsOnly == calls-only; deliberate, ticket 261022 :144 silent on it (policy)(?) |
| overlying-entail-untidy | claude-mod/captions.tsx:595-598,640 | stale flush requeues into the new session's `units` after start() cleared `requested`; worst case a duplicate caption request, no wrong output (?) |
| uncured-catnap-size | claude-mod/captions.tsx:93,429,658 | `settings()`/`DEFAULT_SETTINGS` not used outside module (only internal) |
| uneasily-shortlist-cornbread | claude-mod/register.ts (compactionEnabledFor, 21 uses) | caption gate reuses compaction-named predicate |
| diagnoses-sweep-galore | claude-mod/compaction.ts:68, register.ts | same rename; also `ws ...` hard-coded debug prefixes vs $.plugin.name |
| credible-amid-patrol | claude-mod/captions.tsx:85 | DEFAULT_STYLE embeds Korean examples / "in Korean" (policy) |
| sphinx-afterglow-handcart | claude-mod/compaction.ts:355-366 (start) | late advisory read clears `cached`, keeps latch; a lowered point then delivers advisory after baseline. Existing test :353 treats this as intended; only resume edge (?) |
| union-disagree-patronize | claude-mod/manual.ts:508-523 | workflow_state fallback ignores span guard for other keys; needs anchor drift + revive of another key (?) |
| polka-stranger-silo | claude-mod/manual.ts:~540-570,631 | workflow_manual(K) before setup sets `current` -> self-adoption, extra relogin and "(none) -> K" notice; needs setup slower than first call (?) |
| reformist-trading-charred | claude-mod/register.ts:183 | local storedKey still duplicates exported manual.ts:254 |
| lapped-engorge-oak | claude-mod/reconnect.ts:80,120 vs timing.ts:193, compaction.ts:104 | checkReconnectCall/ReconnectAnswer duplicate checkLeverCall/LeverAnswer |
| sternness-hammock-pediatric | claude-mod/watcher.ts:~30, register.ts:51-57 | mis-wrapped comment lines still present (reconnect.ts header looks fine now) |
| glory-yapping-semantic | claude-mod/*-engine.test.ts(x) | fixtures still copied across engine/compaction/manual/reconnect/captions engine tests |
| doorpost-relive-morphine | claude-mod/compaction.test.ts:1-19 | still imports ./timing; no timing.test.ts |
| exponent-skinning-nutlike | agents-plugin/hooks/ | no tsconfig anywhere under hooks (mailbox-push dir gone); tsc claim not reproducible (?) |
| custard-frenzy-backlog | claude-mod/register.ts:475-482 | restore reads stale STORE_PREFIX entry on same session id; no sweep (manual has prune, mailbox none); noise only (?) |


## Resolution (2026-10-28)

Dropped by user decision the same day. The canonical design lets minor review follow-ups age out of later release-gate ranges (lead-ship Release gate step 1), and lead-review now re-grades a hittable-defect Minor to Important before the verdict (5a3e14a2a), so the remaining TEST/TIDY rows are left to age out rather than kept as a standing backlog. The two open policy questions (credible-amid-patrol: Korean in the shipped caption DEFAULT_STYLE; thievish-dodge-suggest) were not decided and are dropped with it; raise them again if the caption feature is revisited. The rows stay here as a reference.
