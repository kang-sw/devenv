---
title: "데몬 메타데이터 묶음을 SQLite로 전환"
parent: 261119-epic-ws-daemon-sqlite-storage
related:
  261119-refactor-ws-whiteboard-sqlite-board-store: "SQLite pilot and measurement precedent"
  261118-feat-ws-display-transcript-memory-first: "Phase 3 record delta work transferred by (1326)"
  261120-refactor-ws-agent-workers-under-holders: "temporary applied-sequence write exception (1862)"
  261117-research-ws-daemon-file-touch-map: "historical file-touch and crash-recovery evidence"
---

# Move daemon metadata stores to SQLite

## Background

The owner selected the daemon-only metadata bundle as the next SQLite epic
child (board (1953)=a, 2026-11-21). The candidate set is Scope item 4 of
`261119-epic-ws-daemon-sqlite-storage`. The child absorbs the conversation
record delta work and must eliminate whole-record writes made solely to
record an applied holder sequence. The source shows that some candidates are
shared across processes or depend on file-change detection. Their scope and
cutover requirements are settled below.

Source citations are pinned to `e29019758`. Go paths are relative to
`agents-plugin-tool/internal/` unless they start with `cmd/`, which is relative
to `agents-plugin-tool/`. `<dash>` is `<cache>/dashboard`
(`wsdashboard/endpoint.go:37,61-70`, `wsstate/paths.go:91-103`). `<session>` is
`<dash>/sessions/<reply-id>` (`wsdashboard/registry.go:30-46,257-261`).
`<config>` is the directory of global `config.json`, by default `~/.ws` and
overridable with `WS_CONFIG_HOME` (`wsconfig/global.go:11-29`).

### Current store inventory

The paths and cadence below are **VERIFIED** from their read and write sites.

| Store | Readers | Writers and write frequency |
|---|---|---|
| `<session>/agent.json` (the runner's `Record`, called `record.json` in the transferred phase) | Runner startup enumerates session directories and restores open and closed records. Handoff reads prior records. MCP reads the record for tracker support admission, playbook terminology, hosted parent resolution, Pi child rows and board nudges. Dashboard lifecycle and Pi revival read a projection, and the page watches the file. `wsagent/record.go:288-332`, `wsagent/runner_startup.go:68-89`, `wsagent/handoff.go:146-173`, `mcp/tracker_support.go:49-64`, `mcp/playbook_terminology.go:74-104`, `mcp/hosted_key.go:84-99`, `mcp/pi_live.go:65-89`, `mcp/whiteboard_nudges.go:388-402`, `wsdashboard/registry.go:386-425,470-498`, `wsdashboard/page_reader.go:230-254,304-312`. | Runner writes on creation, child creation, takeover/handoff, recovery changes and conversation mutations. Each holder line that adds a ring entry can trigger a full replacement, with another replacement on a dirty heartbeat or graceful detach. `wsagent/runner_create.go:217-249`, `wsagent/children.go:380-390`, `wsagent/handoff.go:146-173,272-287`, `wsagent/runner_startup.go:190-202`, `wsagent/runner_conv.go:653-667,812-839`, `wsagent/runner_process.go:797-816`, `wsagent/runner_events.go:827-842`. |
| `<session>/permission.json` | Cached projection and provenance checks, fresh read at launch/admission, and metadata revalidation on hosted lead `Runner.Get`. Absence allows legacy derivation. An invalid existing selection fails closed, including a dangling symlink. `wsagent/permission_store.go:49-140,148-179`, `wsagent/permission.go:125-191`, `wsagent/runner_operations.go:15-31`. | Explicit lead creation or spawn, authorized permission change, and a mapped native-mode change when it updates an explicit common selection. The writer rereads and preserves unknown JSON members and the presence of `delegated_authority`. There is no periodic write. `wsagent/runner_create.go:217-237`, `wsagent/permission.go:329-356,375-405`, `wsagent/permission_store.go:182-239`. |
| `<dash>/support.json` | `NewSupport` loads last selection and store-to-conversation mappings during daemon construction. Choices and current support-session lookups use memory. `cmd/ws-mcp/dashboard.go:112-119`, `wsagent/support.go:82-91,157-182,350-384,450-486`. | Opening support writes the mapping. Restart can delete and then add a mapping in two writes. Store unregister removes its mapping. Selection writes the last choice, with an extra mapping write on a harness replacement. `wsagent/support.go:197-212,387-414,489-528,560-581,634-657`. |
| `<dash>/pi-probes-v1/<fingerprint>-{models,version}.json` | Go computes the fingerprint from the Pi executable/package identity and, for models, the agent directory and credential/config stamps. Cache reads occur on the version probe and after Support's model memory-cache miss. Models retain the original observation with a five-minute TTL. The MCP version probe is once per process, and the direct server refuses this Pi route before probing when it has no runner. `wsagent/pi_probe_cache.go:101-139,157-232`, `wsagent/pi.go:94-116,280-300`, `wsagent/support.go:40-41,282-332`, `mcp/pi_tools.go:77-98,129-143,521-532`. | Success after a cache miss or staleness only. An unrecognized future schema is protected during publication. The Pi subprocess returns listing/version output. Go reads and writes the recovery cache. `wsagent/pi.go:94-116,280-300`, `wsagent/pi_probe_cache.go:235-261`, `cmd/ws-mcp/dashboard.go:354`. |
| `<dash>/rail-layout.json` | Startup reads the document. The rail reads it when the file stamp changes, and the stream stamp revalidates at the foreign-files boundary. `wsdashboard/rail_layout.go:285-320,344-394,419-431`, `wsdashboard/rail.go:37-50,235-248`, `wsdashboard/foreign_schedule.go:23-26,81-93`. | A changed `POST /rail/layout` operation (move, attach, star, import, etc.) or a worktree removal that changes saved order. A mutation reads the current file first. `wsdashboard/rail_layout_route.go:18-42,104-111`, `wsdashboard/rail_layout.go:442-484`, `wsdashboard/rail_layout_worktrees.go:65-115`. |
| `<dash>/plan-usage.json` and `plan-usage-agy.json` | Startup loads each latest projection. `GET /rail/usage` reads their in-memory entries, can ingest Claude observations, and can ask for a refresh. `wsdashboard/daemon_start.go:135`, `wsdashboard/daemon_http.go:65-66`, `wsdashboard/plan_usage.go:20,114-133,397-460`, `wsdashboard/plan_usage_agy.go:22-23,85-144`. | Changed Claude observations and Codex/Pi usage query results update the first file. Codex/Pi queries are gated at five minutes ordinarily and five seconds on forced refresh. Successful agy results update the second file when its encoding changes. Activity or forced refresh can schedule a probe, with a default ten-minute debounce. `wsdashboard/plan_usage.go:179-188,266-281`, `wsdashboard/plan_usage_probe.go:23-55,71-125`, `wsdashboard/plan_usage_agy.go:22-28,138-144,189-209,241-263`, `wsdashboard/daemon_runtime.go:16-22,57-69`. |
| `<dash>/plan-usage-history.json`, `plan-usage-codex-history.json` and `plan-usage-agy-history.json` | The history response and nonempty append batches stat the file and reread on a stamp miss. Codex checks for retained five-hour samples before reading its projected history. `wsdashboard/plan_usage_history.go:17-20,119-164,265-272,408-460`, `wsdashboard/plan_usage_agy.go:22-23,553-564`, `wsdashboard/plan_usage.go:447-457`. | A changed retained sample batch rewrites the relevant history file. Shared history receives Claude and eligible Codex/Pi samples. Codex five-hour and agy histories have separate files. Identical or immediately pruned samples cause no write. `wsdashboard/plan_usage.go:273-281`, `wsdashboard/plan_usage_probe.go:109-123`, `wsdashboard/plan_usage_agy.go:254-263`, `wsdashboard/plan_usage_history.go:265-332`. |
| `<dash>/memos/<sha256-of-workspace>.json` | GET and POST `/rail/memos` and one read per displayed workspace per rail render. The file key is the hash of the workspace string. `wsdashboard/daemon_http.go:65`, `wsdashboard/memos.go:23-51,118-151`, `wsdashboard/rail.go:37-51,214-225`. | Each successful add, edit or delete replaces that workspace's memo document. The writer retains unknown document/item members and rejects a future schema. `wsdashboard/memos.go:67-84,153-197`. |
| `<session>/label.json` | Dashboard index reads the label after a successful lane read. `wsdashboard/index.go:574-596`, `wsdashboard/label_route.go:73-89`. | Each accepted session-label POST replaces the label or removes the file when cleared. Hiring can also write the label before publishing a hired lead. `wsdashboard/label_route.go:91-105,131-152`, `wsdashboard/session_hiring.go:42-49`, `wsagent/runner_create.go:243-249`. |
| `<session>/ack-check.json` | Eligible answer checks read it and reread under `inboxMu` before updating. Interrupt reads it under the same mutex. Change pokes coalesce while a one-second timer is armed. `wsdashboard/ack_check.go:57-64,113-143,219-272,275-301,304-342`, `wsdashboard/daemon_runtime.go:35-54`. | Interrupts write the interrupt timestamp. A check that passes the commit guard writes the checked turn before it attempts to enqueue a nudge. `wsdashboard/inbox_routes.go:735-743`, `wsdashboard/ack_check.go:134-143,279-297`. |
| `<session>/hiring.json` | Dashboard hiring routes, lead eligibility and rail attach check it. The runner reads hire request identity during a retry search. `wsdashboard/session_hire_routes.go:162-175`, `wsdashboard/session_hire.go:108-136`, `wsdashboard/rail_layout_route.go:91-102`, `wsagent/hire.go:57-81`. | The trusted hiring-enabled POST writes on acceptance. Hired lead creation writes the initial marker after its label and before the conversation record. `wsdashboard/session_hire_routes.go:154-197`, `wsdashboard/session_hiring.go:19-49`, `wsagent/runner_create.go:243-249`. |
| `<config>/dashboard/workspaces.json` | Startup loads the list. The rail and memo path checks read memory. Mutations and sweeps reload the file to incorporate changes from another process. `cmd/ws-mcp/dashboard.go:61,132-134`, `wsdashboard/workspaces.go:23-30,50-105,124-139,179-249`, `wsdashboard/rail.go:137`, `wsdashboard/memos.go:88-92`. | A new workspace, forget or sweep that folds a stored linked worktree into its root rewrites the list. Preview and no-op sweeps read it. Batch admission preflights it and can add more than one workspace. `wsdashboard/workspaces.go:179-261`, `wsdashboard/workspace_routes.go:204,227,249,310-320,365-383`, `wsdashboard/daemon_start.go:175-186`. |
| `<dash>/tracker-hooks/<store-id>.json` | The hook runner loads it on each active store scan, validating the registration nonce. Scans run at startup, on nudge or configuration change, and on a one-minute backstop. `cmd/ws-mcp/dashboard.go:255-266`, `wstrackerhook/state.go:15-16,44-68`, `wstrackerhook/runner.go:24-28,177-195,280-330`. | A successful activity read results in one whole-file replacement for that store, including no-event scans. The cursor, processed IDs and due marks are saved after queueing. Hook commands receive events on stdin, and the separate `tracker-hooks.json` is input configuration. `wstrackerhook/runner.go:39-46,210-247,323-369,449-478`, `wstrackerhook/state.go:72-95`, `wstrackerhook/config.go:16`. |
| `<config>/tracker/<store-id>/ui-seen.json` (specifically `StoreEntry.Dir`) | The daemon's tracker view and projection read seen marks. The first access loads that registration's file and subsequent reads return memory keyed by store ID, directory and nonce. `wstrackerhost/registry.go:32-40,43-64`, `wstrackerhost/host_register.go:105`, `wstrackerhost/seen.go:36-56,78-101`, `cmd/ws-mcp/tracker_host.go:211-215`, `cmd/ws-mcp/tracker_projection.go:24-36`. | Each accepted seen POST makes `MarkSeen` recheck the store registration, union badge/read IDs and replace the file, even if the union adds nothing. Only the dashboard route calls the writer in current product code. `wsdashboard/tracker_routes.go:332-345`, `cmd/ws-mcp/tracker_host.go:531-535`, `wstrackerhost/seen.go:104-129`, `wstrackerhost/registry.go:178-191,250-260`. |

### Process and ownership boundaries

- **VERIFIED: direct Go MCP reads `agent.json`.** Raw `ws-mcp serve --stdio`
  starts an independent server. Raw `ws-mcp call` constructs one without
  `c.host`. Tracker support admission uses the record's key, root, profile
  and open state. Playbook rendering and tool alias admission also use the
  matching record. Ordinary dashboard registration can stat the record
  before unregister and read it before pruning directories, where its
  lifecycle can protect a terminal-owned Pi session file.
  `cmd/ws-mcp/main.go:19-55,58-74`, `cmd/ws-mcp/hosted_cli.go:74-91,162-163,529-542`,
  `mcp/server_dispatch.go:145-154`, `mcp/server_inventory.go:140-165`,
  `mcp/tracker_support.go:49-64`, `mcp/playbook_render.go:107-115`,
  `mcp/tool_aliases.go:102-114`, `mcp/dashboard_sessions.go:370-409,460-472`,
  `wsdashboard/registry.go:386-425,448-498`.
- **VERIFIED: an external non-Go fixture uses two file formats.**
  `scripts/rail-headless.mjs:135-149,167-180,232-243,579-582,979-982`
  seeds `label.json` in its scratch registry and removes/reads the scratch
  `rail-layout.json`. This is a test consumer and a cutover verification
  dependency. **INFERRED:** its sandboxed fixture contract gives no evidence of
  a production dependency on either JSON file.
- **VERIFIED: three stores explicitly account for foreign file changes.**
  Permission revalidation tests the file identity and metadata, rail layout
  refresh detects a foreign write, and history rereads a newer version or a
  repaired file. Workspace mutations also reload external edits. These paths
  establish current behavior. **INFERRED:** a supported production editor
  has yet to be identified for permission, rail layout or plan-usage history.
  `wsagent/permission_store.go:70-98`,
  `wsdashboard/rail_layout.go:322-327,354-379,446-459`,
  `wsdashboard/plan_usage_history.go:119-127,265-272`,
  `wsdashboard/workspaces.go:50-52,134-139`.
- **VERIFIED:** the daemon lock is per dashboard state directory, while the
  workspace list and tracker root are per config directory.
  **INFERRED:** separate `WS_CACHE_HOME` values with the same `WS_CONFIG_HOME`
  allow independent dashboard daemons to share `workspaces.json` and
  `ui-seen.json`. Workspace reload behavior explicitly anticipates another
  process. Seen currently assumes one dashboard writer, so its in-memory
  cache has no foreign seen-mark invalidation. Moving either into a
  cache-local DB could fork state. `wsdashboard/daemon_start.go:21-44`,
  `wsstate/paths.go:91-103`, `wsdashboard/workspaces.go:16-30,50-52`,
  `wstrackerhost/registry.go:32-40`, `wstrackerhost/seen.go:36-49,104-128`,
  `ai-docs/manuals/ws-mcp.md:1480-1487`.
- **INFERRED from the tracked source search:** no other production launcher,
  packaged Pi/Claude extension, Python holder or external Go caller was
  found opening the listed metadata files. In the adjacent **VERIFIED**
  contracts the launcher opens `endpoint.json`, Pi and Claude read the
  registration path supplied by the runner, and the Python holder writes
  its descriptor and exit files. A workspace or tracker hook can run user
  code, so repository search cannot classify private consumers.
  `agents-plugin/bin/ws-mcp-launcher.py:963,1149`,
  `wsagent/runner_process.go:373-387`, `agents-plugin-pi/src/hosted-session.ts:95-105`,
  `agents-plugin/hooks/claude-mod/hosted.ts:93-105`, `wsholder/holder.py:13-23,108-110`,
  `wstrackerhook/runner.go:39-46,449-478`.

### Before: file operations per action (derived from source)

Counts below are **VERIFIED** as API-level counts from the cited success
paths, with stated branch conditions. Count one for a read, stat, mkdir,
create, chmod, write, replace or remove. Exclude close, retries, work outside
the named stores and the internals of a compound call such as `ReadFile`.
The wsagent replacement is five calls (`MkdirAll`, create, chmod, write,
replace) and the dashboard private-file replacement is four (create, write,
chmod, replace). The tracker seen writer omits chmod and mkdir.
`wsagent/record.go:335-365`, `wsdashboard/endpoint.go:97-122`,
`wstrackerhost/registry.go:133-158`. Windows can issue more calls on sharing
violations (`wsagent/replace_file_windows.go:15-60`). These are source counts,
with measured syscall counts and latency still due.

| Action and conditions | Metadata calls | Source |
|---|---:|---|
| Held lead or worker finishes a line that advances the sequence and touched the final ring | 5 for the whole record per line. Clean heartbeat 0. Dirty heartbeat 5 for the whole record, on the default 30 s runner tick. | `wsagent/record.go:525-543`, `wsagent/runner_events.go:17-29`, `wsagent/runner_process.go:797-816`, `wsagent/runner_conv.go:536,653-659,812-829`, `wsagent/runner_startup.go:35-37`, `wslive/snapshot.go:85-89` |
| Direct support admission with a matching candidate key | 1 whole-record read per admission invocation | `mcp/tracker_support.go:49-64,86-97`, `wsagent/record.go:294-321` |
| Hosted lead state poll or explicit common-permission change to an existing lead selection | 1 stat for a warm stable poll, 3 calls when a loaded existing file revalidates and rereads. Stable absent poll 2 (stat + lstat). The change uses 8 on the selection file (authorization stat + read, writer read + 5), plus any record update or later launch checks. A worker's parent authorization reads are additional. | `wsagent/runner_operations.go:23-29`, `wsagent/permission_store.go:76-125,201-238`, `wsagent/permission.go:128-130,294-350`, `wsagent/record.go:335-365` |
| Support store read, opening send, or New replacing an existing conversation | 0 in `support.json` for read or send to an existing conversation, 5 to set a mapping, 10 to delete and replace a mapping | `wsagent/support.go:197-212,411-414,474-528` |
| Pi probe cache hit or successful cold miss with valid identity and publication | 1 cache read on hit. Miss 9 (lookup read, header read, mkdir, chmod, 5-call replacement). Executable/package/credential stamps and the actual probe are extra. | `wsagent/pi_probe_cache.go:101-139,157-217,220-261`, `wsagent/record.go:335-365` |
| Changed rail layout mutation, no-op mutation, or one rail render showing W workspaces | 7 on a change (stat + read + 4 + stat), 2 on no-op. Render 1 layout stat + W memo reads, plus 1 layout read on change. | `wsdashboard/rail_layout.go:285-286,314-319,354-379,446-484`, `wsdashboard/rail.go:37-51`, `wsdashboard/memos.go:28-35`, `wsdashboard/endpoint.go:99-122` |
| Usage observation that changes latest usage, and each changed history batch it produces | Latest 4 per changed file. History 6 on a warm stamp (stat + 4 + stat), 7 when an existing history file must be reread. Warm history-only read 1 stat for shared history, 1 for displayed agy history, and 1 or 2 for Codex five-hour history (the second when samples are present). A changed existing file adds a read. Empty append batch 0. | `wsdashboard/plan_usage.go:107-112,179-188,273-281`, `wsdashboard/plan_usage_history.go:132-162,265-332,408-460`, `wsdashboard/plan_usage_agy.go:138-144,254-263,553-564` |
| Memo GET or accepted add/edit/delete | GET 1. Mutation 6 (read + mkdir + 4). | `wsdashboard/memos.go:28-35,67-84,145-197`, `wsdashboard/endpoint.go:99-122` |
| Session label set or clear | Set 4, clear 1 remove. A following successful index lane read adds 1 label read. | `wsdashboard/label_route.go:75-105,142-150`, `wsdashboard/index.go:574-589`, `wsdashboard/endpoint.go:99-122` |
| Answer check reaches and passes its commit guard, or interrupt updates the marker | Check 6 (initial and guarded read + 4), interrupt 5 (read + 4). | `wsdashboard/ack_check.go:115-143,247-272,279-297`, `wsdashboard/endpoint.go:99-122` |
| Accepted hiring toggle or hired lead with nonempty label | Toggle 5 (read + 4), hired lead 8 for label + hiring writes, before the separate agent-record write. | `wsdashboard/session_hire_routes.go:169-197`, `wsdashboard/session_hiring.go:21-49`, `wsagent/runner_create.go:243-249`, `wsdashboard/endpoint.go:99-122` |
| Workspace add/forget/sweep that changes the list, or warm rail access to the list | Change 6 (reload read + mkdir + 4), no-op forget or sweep 1, warm rail accessor 0. Additional path and Git probes are outside the count. | `wsdashboard/workspaces.go:77-107,108-131,179-261`, `wsdashboard/endpoint.go:99-122` |
| Active tracker hook scan for one store with a successful activity read | 5 even if it finds nothing to queue (read + mkdir + create + write + replace) | `wstrackerhook/runner.go:314-369`, `wstrackerhook/state.go:52-95` |
| Tracker seen POST on a warm seen cache, or a warm seen read | POST 3 for marks plus 1 registry stat and, on a registry memo miss, 1 registry read. A cold marks access adds 1 seen-file read. Warm seen read 0. | `wstrackerhost/seen.go:36-49,78-129`, `wstrackerhost/registry.go:107-109,115-117,138-158,178-191,250-260`, `wsstate/read_cache.go:38-53` |

The older profile in `261117-research-ws-daemon-file-touch-map`, ranked touch
sources and Q2 (lines 190-205 and 280-290), gives context for record rewrites,
permission polling, rail/memo reads and answer checks. Its rates describe
that snapshot of the code and machine.

## Decisions

- **Bundle next and scoped to daemon-only metadata** (board (1953)=a).
  Apply the parent epic's five-store scope and exclusions ((1325)=a).
  Git-tracked state, git transport, non-Go production file contracts,
  child-owned inputs and raw blobs stay files. `wsmailbox` remains on its
  existing memory-first segment ring. `registration.json`, Python holder
  files, Pi `session.jsonl` and `system_prompt.md` are among the file contracts.
- **Conversation record deltas are part of this child** ((1326)=a, the
  transferred Phase 3 in `261118-feat-ws-display-transcript-memory-first`).
  **Remove the whole-record holder cursor write** ((1862)=a,
  `261120-refactor-ws-agent-workers-under-holders`, Decisions). It applies to
  ring-writing final lines and dirty heartbeats for held leads and workers.
  The inherited cost was approved for the holders change as a temporary
  performance exception. **Rejected there:** adding a separate cursor store
  in the holders ticket. This child settles the replacement recovery mechanism.
- **SQLite starting point** ((1325)=a, epic Cross-Child Decisions). Use the
  `modernc.org/sqlite` precedent, WAL, `SetMaxOpenConns(1)`, `busy_timeout`,
  foreign keys and bounded `SQLITE_BUSY`/`SQLITE_LOCKED` retry. Measure the
  chosen configuration on the target platform. `wsstore/store.go:145-162,209-225`,
  `wsstore/retry.go:22-48,61-72`.
- **Whiteboard precedent is board-specific** ((1325)=a, refined by (1347)=c).
  The whiteboard child owns one DB per board, lazy open, a small cache and
  close after a long idle (about 30 minutes). **Rejected there:** a shared
  board database and a board-only append journal. The embed snapshot is a
  separate epic child. These decisions provide the structural precedent for
  this draft. Metadata DB topology is settled below; handle lifetime remains
  subject to measurement.

- **(1980) Agent records:** Move `agent.json` authority into SQLite in this
  bundle. First route direct Go record readers, including tracker auth,
  session unregister and registration pruning, through the daemon. Retain
  only legacy file behavior required by the selected compatibility tier.
  Rejected: leave the shared file authoritative and only remove holder
  whole-file rewrites.
- **(1981) Workspaces:** Leave `workspaces.json` outside the bundle: its
  config root is shared across processes and it has no cursor-driven rewrite.
  Rejected: include it and add a shared-ownership contract.
- **(1982) Tracker seen:** Leave `ui-seen.json` outside the bundle: the tracker
  config root can be shared across daemons. Rejected: include seen marks and
  add a shared-ownership contract.
- **(1983) Permission selection:** Edits go only through the daemon's
  authorized entry point; read the old file only for compatibility and
  import. Preserve fail-closed behavior and origin validation. Rejected:
  continue accepting direct live file edits.
- **(1984) Rail layout:** Dashboard actions are the only live write path;
  update fixtures to test dashboard actions and legacy import separately.
  Preserve ordering, generation and wake behavior. Rejected: continue
  accepting live direct file edits.
- **(1985) Usage history repair:** Preserve schema and error protection on
  legacy import and document an offline SQLite recovery path. Rejected: keep
  the file as the live out-of-process recovery source.
- **(1986) Direct callers:** Route affected `ws-mcp call` and `serve --stdio`
  operations through authenticated daemon requests; specify behavior when no
  daemon is running. These direct entry paths remain compatibility paths;
  the daemon handles the work. Prove predecessor drain, writer shutdown and
  successor startup during replacement. Rejected: multi-process SQLite WAL
  access; refuse direct calls as a T3 boundary.
- **(1987) Compatibility:** Target T0–T2 per store and return any T3 boundary
  to the owner as a new decision. Rejected: settling T3 without owner approval.
- **(1988) Topology:** Use one shared daemon state DB for cache-rooted stores;
  revisit only if measurements show contention. Rejected: per-store DBs with
  multiplied handles and checkpoints.
- **(1989) Import:** Use transactional, repeatable legacy import. Choose the
  discovery point, completion marker and authority handoff; keep invalid or
  future data and unknown fields available for recovery. Rejected: request-path
  fallback and continuous dual writes.
- **(1990) Legacy files:** Keep legacy files through this child, following the
  whiteboard rollback precedent (1816)=a. Rejected: remove them in this child.
- **(1991) Conversation state:** Use reply-ID keyed state and bounded recovery
  deltas; decide snapshot/compaction and legacy unknown-field representation.
  Rejected: rewrite the whole record on every mutation.
- **(1992) Applied sequence:** Use a delta keyed by conversation and holder
  identity. Prove replay with partial ring append, cursor write failure, holder
  change and process death between stores. Rejected: persist a whole-record
  image or delay sequence persistence by time alone.
- **(1993) Permissions:** Use reply-ID keyed explicit permission records with
  presence-sensitive provenance and preservation of unknown members. Keep
  invalid and absent legacy inputs distinguishable. Rejected: infer selection
  from conversation settings.
- **(1994) Support:** Separate last selection from store-to-conversation keyed
  mappings; preserve replacement and unregister recovery. Rejected: rewrite
  one support document for a single mapping change.
- **(1995) Pi probes:** Key probe records by fingerprint and kind; preserve
  success-only admission, freshness and invalidation. Decide whether to import
  legacy cache hits or re-probe as part of (1989). Rejected: merge both probe
  kinds into runner or support state.
- **(1996) Rail storage:** Use workspace-scoped layout changes and an in-memory
  generation; preserve ordering, legacy extra fields and consumer wake.
  Rejected: a monolithic layout rewrite or DB-file polling.
- **(1997) Latest usage:** Key projections by source/group; preserve the wire
  projection, fallback and debounce behavior. Rejected: rewrite the shared
  current usage document.
- **(1998) Usage samples:** Use keyed samples with existing history identity
  and retention; prove shared, Codex five-hour and agy projections, including
  unknown legacy fields. Rejected: serialize the entire history on append.
- **(1999) Memos:** Use keyed records with stable order and preservation of
  legacy document/item fields; retain or resolve the workspace hash on import.
  Rejected: rewrite every memo for a one-item edit.
- **(2000) Labels:** Key labels by reply ID and delete on clear; preserve index
  publication and newly hired lead behavior. Rejected: persist an empty value.
- **(2001) Answer checks:** Use a guarded reply-ID keyed marker; preserve the
  interrupt/check race and trace marker update before inbox nudge. Rejected:
  an unguarded batched cursor advance.
- **(2002) Hiring:** Key metadata by reply ID with request identity available
  to retry; decide recovery after partial label, hiring and record publication.
  Rejected: embed hiring flags in registration.
- **(2004) Tracker hooks:** Scope cursors by registration nonce and use keyed
  processed/due updates; preserve queue/receipt order and catch-up policy.
  Rejected: replace all hook state every scan.
- **(2006) Durability:** Define requirements per write class and evaluate
  `PRAGMA synchronous` against recovery and measured cost, especially for
  holder cursors and consumed input. Rejected: one immediate flush rule for
  every metadata update.
- **(2007) Checkpoints:** Measure WAL checkpoint cadence outside interactive
  actions; set handle lifetime, checkpoint method and idle/shutdown drain from
  topology and Windows evidence. Rejected: assume default auto-checkpoint is
  cheaper.
- **(2008) Staging:** Use the whiteboard precedent: an unwired store/import
  phase, an authority switch, and a measured result. Rejected: switch authority
  without the staged import and measurement.
- **(2009) Baseline:** Capture the pinned baseline before wiring authority and
  compare on the target Windows AhnLab V3 machine before release. Rejected:
  gate the switch on a WAL prototype; the wired comparison supplies the needed
  evidence for the chosen topology.

## Constraints

- Follow `ai-docs/manuals/performance.md:12-40,63-73` for memory-first reads,
  deltas, file/handle lifetime, interactive I/O and Windows before/after
  measurement. Rule 8 governs any proposed exception. Count the store reads
  and writes as well as SQLite open, WAL/SHM, locks, sync and checkpoint work.
- **Compatibility tier unresolved.** Grade each affected store and direct
  call under `ai-docs/manuals/release-compatibility.md` before `ready`. Trace
  new open of old files, the old binary's actual read/write or refusal after
  cutover, and re-upgrade reconstruction. A T2 claim needs surviving data
  and a deterministic new-reader recovery trace. T3 requires a minor release,
  explicit owner approval and an effective old-side fence before damaging
  writes. T4 cannot ship. Retaining files as a rollback snapshot alone gives
  no T2 recovery of the updates after import.
- **VERIFIED existing contracts:** the holder reattaches using `HolderSeq`
  together with holder identity, for leads and workers. Ring apply precedes
  line completion, and graceful detach finishes an in-flight line. Preserve
  replay ordering and deduplication across the record, ring and snapshot
  recovery boundaries. Preserve the durable copies of consumed input,
  including the queued turn around `proc.Send`, mailbox claims, steers and
  pending reports. Preserve protection of terminal-owned files during handoff,
  existing downgrade-fenced worker recovery, and owner-only treatment of the
  record and permission data, including SQLite side files when applicable. `wsagent/runner_process.go:208-254,286-299,480-496,797-816`,
  `wsagent/record.go:324-357,525-555`, `wsagent/runner_events.go:827-847`,
  `wsagent/runner_conv.go:318-365,1209-1230,1284-1313`,
  `wsagent/children.go:654-679,744-775`, `wsdashboard/registry.go:448-467`.
- Route permission edits through the authorized daemon entry point and rail
  layout writes through dashboard actions. Their legacy files are compatibility
  and import inputs, not live edit paths. Preserve usage history import
  schema/error protection and document offline SQLite recovery.
- Preserve the current permission error/omission distinction, unknown fields,
  and presence-sensitive delegation provenance. Preserve hook catch-up, due
  and retry semantics, and seen-mark identity across unregister/re-register.
  Dashboard views and routes retain existing label clearing, workspace
  placement, usage retention/dedupe, memo, notify and hiring behavior.
  `wsagent/permission_store.go:119-161,201-239`, `wsagent/record.go:187-239`,
  `wstrackerhook/runner.go:303-369`, `wstrackerhost/seen.go:36-56,104-128`,
  `wsdashboard/plan_usage_history.go:17-20,216-250,265-332` (14 days and
  5,000 samples per harness/group/kind), `wstrackerhook/runner.go:447-480`
  (a failed hook run is logged).
- Keep `workspaces.json` and tracker `ui-seen.json` outside this bundle.
  Keep the scope of neighboring stores as set by the epic. Session-key
  metadata, wslive snapshots and rings, inbox, whiteboard, tracker synced
  data and tracker host registry/sync files have their own owners. Adjacent
  `workspace-admission.json`, `hire-proposals.json` and `visibility.json`
  remain outside the listed bundle. Preserve their integration points.
  Follow `ai-docs/manuals/shipped-surface-boundary.md` for any new diagnostics
  or other shipped text.

## Route Facts

These are draft facts to refresh at promotion. **VERIFIED** denotes a current
code route. **INFERRED** denotes implementation or verification scope.

| Fact | Value | Evidence |
|---|---|---|
| scope.span | multi-file | VERIFIED. Inventory spans `wsagent`, `wsdashboard`, `wstrackerhook`, `wstrackerhost`, `mcp` and `cmd/ws-mcp`. |
| scope.surface | cross-module | VERIFIED. Record read paths extend to direct MCP, dashboard ownership and registration pruning. See process and ownership boundaries. |
| scope.new_public_symbol | open | INFERRED. The store API and authenticated daemon routing require implementation design under (1980), (1986) and (1988). |
| scope.new_type_contract | yes | INFERRED. SQLite recovery shapes and downgrade behavior require implementation design under the settled decisions. |
| scope.test_surface | multi-package | INFERRED. Startup and mixed-version recovery, raw CLI, holder replay, route behavior and the rail scratch fixture cross package boundaries. |
| complexity.reuse_points | confirmed | VERIFIED. `wsstore/store.go:145-162,209-230` and `wsstore/retry.go:22-48` provide SQLite setup, and legacy readers are listed above. |
| complexity.side_effect_risk / risk.correctness | high | INFERRED. A stale authority read can alter admission or prune a live directory. Cursor/recovery ordering can drop or replay held output. |
| risk.fit / risk.test | high | INFERRED. Shared roots and direct callers constrain cutover. Performance needs comparable Windows AV traces. |
| risk.security_or_contract | high | INFERRED. Record-based tracker admission, permission provenance, and stored MCP token/holder data require explicit preservation. `mcp/tracker_support.go:41-64`, `wsagent/permission_store.go:148-161,217-237`, `wsagent/runner_process.go:247-255,489-496`. |

## Phases

Sequence under (2008). Resolve the remaining implementation details and grade
compatibility under (1987) before the ready plan.

### Phase 1: Build the store and legacy import without wiring callers

- Implement the shared store and repeatable import for eligible metadata. Use
  fixtures from the listed old formats and keep active callers on their
  existing paths during this phase.
- Verify old-data import, restart and retry after interruption, unknown
  fields and invalid/future inputs, and registration or holder ID isolation.
  Capture a comparable legacy baseline and record the directional
  compatibility evidence for the authority switch.

### Phase 2: Switch authority and remove the holder sequence rewrites

- Depends on Phase 1. Wire the agreed reader, writer and daemon lifecycle
  paths. Route direct Go readers through the daemon before agent record cutover,
  including tracker auth, session unregister and registration pruning. The
  holder's sequence-only writes for a final ring line and a dirty heartbeat
  cease replacing the whole record.
- Verify hard-kill/rejoin and graceful replacement for held leads and
  workers, including interrupted ring append and pending consumed input.
  Exercise tracker authorization, terminal ownership/pruning, permission
  provenance, dashboard refresh/notifications and the hook registration
  boundaries. Exercise authenticated direct calls and stdio with the daemon
  available and unavailable, and replacement drain. Test authorized permission
  edits, fail-closed origin validation, dashboard-only layout actions, and
  legacy imports separately. Verify schema/error protection and the documented
  offline SQLite recovery path. Verify the old/new cutover contract on the old
  binary as well as the new one.

### Phase 3: Record the measured result

- Depends on Phase 2. Compare the same actions and dataset on the Windows
  heavy-AV baseline. Trace cold open/import and warm actions separately.
  Include held final ring lines and dirty/clean heartbeats, permission polls,
  representative UI reads/mutations and hook scans for the stores included
  in the bundle; workspace and tracker seen files remain excluded.
- Record syscall counts and action-to-visible-effect latency, including WAL,
  SHM, sync, checkpoint, replacement and shutdown bursts, against the prior
  paths. Verify the two cursor-driven whole-file writes are absent in the
  traced cases. Record any requested performance exception and its measured
  cost for the owner.
