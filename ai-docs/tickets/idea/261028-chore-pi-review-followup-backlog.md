---
title: Work down the agents-plugin-pi review follow-up backlog
---
# Work down the agents-plugin-pi review follow-up backlog

## Background

Review sweeps on `develop` recorded these as `minor/review` commit follow-ups
(`ws/git.followups`), and none was ever resolved. A triage on 2026-10-28 at HEAD
`f400e5fbe` checked each against current code and classified it TEST (coverage
gap, behavior believed correct) or TIDY (duplication, naming, comment/wrap,
structure; no behavior impact); BUG and STALE entries were handled separately.
The creating commit resolves every id below, so this ticket is now the single
owner of the pi backlog and the release gate no longer lists them.

Each row keeps the original follow-up id; `git log --grep=<id>` finds the
review commit that recorded it and its AI Context. Paths are relative to `agents-plugin-pi/`. Line numbers were
re-located at triage time and will drift. `(?)` marks a call the triager was
unsure of (the lower-impact class was chosen).

Three TIDY `(?)` rows sit closest to a defect and deserve a first look:
`bargraph-headed-errant` (a mid-run user `/compact` after the advisory can queue
a second preparation message), `flashbulb-swimming-silent` (a held adapter prompt
parks until an unrelated run when its wake reservation lapses), and
`palace-importer-princess` (a slow async preflight lets the recovery timer send a
second idle prompt).

## Phases

### Phase 1: Work through the pi backlog

Re-check each row against the code at pickup; drop rows that no longer hold and
say so in the Result. Batch by file or module rather than by row. TEST rows add
tests that fail on the described regression; TIDY rows change no behavior. A
row that turns out to be a real defect is reported, not silently fixed inside a
tidy batch. Splitting this phase further is fine when the backlog is too large
for one worker pass.

#### TEST (32)

| id | current path:line | note |
|---|---|---|
| egotistic-unlinked-flight | test/lead-compaction.test.ts:609-640 (src/lead-compaction.ts:862) | extractLeadProse new-layout fallthrough with no later Child agents heading still has no test |
| system-mustang-jokester | test/goal-loop.test.ts:2401-2460 | token-threshold tests exist, none changes the context window mid-session to pin the latch |
| happily-ibuprofen-evacuee | test/background-tasks.test.ts:260-270 | fork test still calls killAllBackgroundTasks() first, so the fork-abort half is nominal |
| tray-lair-curdle | test/background-tasks.test.ts:329-335 | only the 2^31 no-fire-at-once case; step-wise re-arm to the real deadline (src/background-tasks.ts:151-160) not exercised |
| sneer-diligence-bunkhouse | src/goal-loop.ts (session_compact stale-op guard, releaseAfterCompaction ~1305) | stale-operation guard on recovered pushes untested |
| drainage-obituary-exes | src/display-summary.ts:515,683 / src/index.ts:683 | manifest default read-once, readDefaultStyle forwarding and manifest path wiring not tested (only harness-injected) |
| stock-wiring-trimming | test/push-render.test.ts:326-334,609 | (?) mostly covered now (registration at 609, task_id head at 326); only the no-task_id fallback head remains unpinned |
| running-human-unsold | src/index.ts:880-890 | keyless owner-lead bootstrap-output path untested |
| cabana-ungodly-thermal | src/display-summary-render.ts:500-520 | (?) definition-less tool rows get contentBox shell vs Pi generic renderer; unpinned |
| entail-muster-rectangle | test/display-summary-render.test.ts:675-677 | expected size computed via formatContextSize(contentBytes()) - tautological (only >4200 bytes guard is independent) |
| cytoplasm-factor-undercut | test/display-summary-render.test.ts:690-711 | push batch-item size token and subtitle dedupe unasserted |
| relatable-reliable-exponent | test/display-summary-render.test.ts:737 | adapter-message size sits at the 0.1 KB floor, does not discriminate |
| sevenfold-murky-nugget | test/mailbox-rearm.integration.test.ts:128 | fixed 1.2 s sleep for the shutdown race |
| espresso-outfield-brutishly | src/spawner.ts:3781,4001,4031 | stop-during-launch skip of awaitExitedChildStdout has no direct test |
| dynamite-destruct-tint | test/child-launch-guard.test.ts:240-262 | no-handle / child-alive negatives use ctx.spawn() only; sendToAgent resume counterpart missing |
| bullfight-gamma-version | src/spawner.ts:3640,3968-3973 | stop landing during pending preflight not tested for spawn or resume |
| scoop-catty-washable | src/local-devenv.ts:261-285 | rebootstrap rejections (plugin_version missing, launcher non-zero/timeout + cleanup) untested; test/local-devenv.test.ts has none |
| late-filth-unloving | test/session-bootstrap-guard.test.ts:271-273 | wall-clock `elapsed >= 70 && < 2000` bounds (flake risk under load) |
| scouring-suspend-jawline | src/index.ts:993,1021-1022 | orphan-notice send sites not asserted; only buildOrphanNoticeMessage (agent-sidecar.test.ts:612) |
| amplifier-passerby-smog | src/index.ts:789 | sessionId -> startBridge wiring untested at index level |
| pointing-escapist-barista | test/lead-key-resume.test.ts:273-296 | policy-child tests cover bootstrap only, not workflow_manual with an explicit session_key |
| stegosaur-impulse-negative | src/lead-compaction.ts:666-672 | descending and space-padded ranges: only "0-2" and unparsable tested (test:688-690) |
| dreamt-folk-implode | test/goal-loop.test.ts (token latch tests ~2401-2460) | no async-reader test showing baseline sets latch after read resolves |
| sandlot-deplored-emerald | src/goal-loop.ts:1544,1771,1918 | latchEpoch bumps on reset/shutdown untested |
| punisher-fantasize-calcium | test/goal-loop.test.ts:2457 | advisory>=hard now tested; partial-drop re-arm (70, 65, 71) still not |
| barge-human-goldfish | test/compaction-history.test.ts | failure test checks handler absence; renderer malformed-entry guard (src/compaction-history.ts:57) untested |
| quintuple-tidiness-famished | test/goal-loop.test.ts:3395 | still `preparations().length >= 2` |
| neon-gigabyte-italics | execute-worker-guide.md, lead-compact-guide.md | no-context-loading prose unpinned (only path references in tests) |
| ruckus-ebook-abstract | test/goal-loop.test.ts (fallback test) | compaction-entry skipping / turnPrefixMessages alongside tail not covered |
| overbite-domain-unpack | test/adapter-config.test.ts:66-77 | manifest min/max/enum not checked against resolver ranges |
| alphabet-maximize-gap | test/bridge.test.ts:188 | asserts via WS_MCP_CONFIG_MANIFESTS_ENV constant, not the literal Go name |
| take-entrap-yesterday | src/index.ts:562-563 | createWsConfigReader wiring to handle.client/defaultSessionKeyRef untested |

#### TIDY (35)

| id | current path:line | note |
|---|---|---|
| calibrate-chewing-disparate | src/goal-loop.ts:265-268, src/lead-compaction.ts:917 | (?) tokens > window -> percent >100: unreachable threshold; needs misconfig, percent side always wins when set |
| riveting-mystify-limelight | test/adapter-config.test.ts:100 | corpus duplicated by hand in Go config_manifest_test.go; drift risk only |
| wieldable-handling-handwork | src/display-summary.ts:151-154,514-516 | (?) blank style -> undefined -> falls back to last/default style; consistent with docs, not a defect |
| cheese-riptide-issuing | pi-lead-guide.md:96 | ws-background-task row sits in the child-signal table |
| tricolor-compress-unlaced | src/execute-gateway.ts:237 | literal still repeated in spawner.ts:1003, push-render.ts:147,175, display-summary.ts:395 |
| laxative-mollusk-giddiness | src/execute-gateway.ts:913, pi-lead-guide.md:51 | abort description/guide omit "kill sent; exit not confirmed" outcome (emitted at execute-gateway.ts:929) |
| eligibly-valley-encircle | src/background-tasks.ts:142-183 | (?) liveTasks removal precedes drain push; narrow settle-window race, no concrete harm |
| zoning-boxing-hardware | src/display-summary.ts:400 | PUSH_BATCH_TYPE duplicates push-protocol.ts constant (now pinned equal by test) |
| unvisited-clanking-guts | src/display-summary-render.ts:358, display-summary.ts:606, display-summary-sidecar.ts:65 | literal in 3 files vs COMPACTION_HISTORY_TYPE |
| bonanza-dramatize-take | src/adapter-config.ts:70-105 | createWsConfigKeyReader near-duplicates createWsConfigReader |
| bobbing-despair-prognosis | src/display-summary-render.ts:551 | contentText repeats customMessageText join (display-summary.ts:~410) |
| handset-overstock-stubbed | src/display-summary-render.ts:497-516 | inner try body not indented |
| throat-dangling-habitual | src/display-summary-render.ts:248 | `padX + 4` literal vs SUMMARY_FIELD_INDENT (line 118/247) |
| shorten-flinch-spotlight | src/pi-tui.ts:100-112 | doc omits `fallback:false` option |
| slander-pencil-tableful | pi-lead-guide.md (config_tune row) | tunable list does not mention pi.display_summary_model/_effort |
| unmanned-mulled-gilled | src/display-summary-render.ts:268-282 | cosmetic partial dedupe |
| reattach-sizable-trophy | src/text-width.ts:22-26 | (?) C1 controls U+0080-9F not neutralized; stripTerminalSequences runs first, terminal-dependent, no concrete trigger |
| turf-cattail-google | src/goal-loop.ts:81 | long header comment line |
| salt-pentagon-nature | src/compaction-history.ts:37-42 | filters duplicate producer strings as literals |
| unease-skiing-factual | src/index.ts:678-686 | comment about live bridge key now sits above registerDisplaySummarySession/registerCompactionHistory, not the goal-loop sessionKeyRef it documents |
| impale-conductor-battered | src/compaction-history.ts:79 | (?) possible duplicate display of kept messages on native fallback; display-only |
| clamor-overbite-onto | test/lead-compaction.test.ts:646 | fixture dir mkdtemp at describe load, never removed |
| cycling-even-wrongful | src/goal-loop.ts:1563-1570 | (?) overlapping baselines: last read to resolve wins (no ++latchEpoch in baseline); needs two events inside one read window |
| flashbulb-swimming-silent | src/goal-loop.ts:1257,1605; src/spawner.ts:1209-1215 | (?) held adapter prompt drains only on agent_start; parks if reservation lapses without a start (needs rejected prompt) |
| palace-importer-princess | src/goal-loop.ts:1281; src/spawner.ts:1232 | (?) slow async preflight beyond settle_delay lets recovery flush send a second idle prompt; needs slow input handler |
| sixteen-undusted-trash | src/goal-loop.ts:~1043 sendOwedResume | (?) ordering race only under async session_compact handlers |
| moocher-decline-swell | src/goal-loop.ts:1255-1262 | (?) hard prep at agent_end recorded as route hard and resumes; intended-ish |
| bargraph-headed-errant | src/goal-loop.ts:1788-1793 | (?) reroute sendPreparation not guarded by `preparation`: /compact mid-run after advisory queues two prep messages (duplicate guide, no data loss) |
| childish-trickle-headphone | src/goal-loop.ts buildCompactionResumeMessage; src/lead-compaction.ts Resume section | same lead-revive instruction at two sites |
| lark-pedigree-sanction | src/lead-compaction.ts renderDialogItem | (?) payloads unescaped; accepted by design in f6971f070 |
| spotting-sleeve-aggregate | src/goal-loop.ts (1931 lines) | lead-compaction wiring still inline; extraction suggestion |
| vegan-level-surrender | src/adapter-config.ts:19 | type-only import from goal-loop.ts, GoalLoopConfig naming |
| dairy-doable-parted | src/goal-loop.ts:681-686 | keptTailMessages docstring omits overflow exception (exception noted at call site 1675) |
| excretion-churn-mayflower | test/goal-loop.test.ts:1862 | title says ws-mcp config.get but body uses fileReader shim |
| canteen-rare-void | src/lead-skills.ts:39, src/bridge.ts:709-710 | comments still name retired lead-drain-ready-queue / lead-proceed.md (no such skill/rsrc at HEAD) |
