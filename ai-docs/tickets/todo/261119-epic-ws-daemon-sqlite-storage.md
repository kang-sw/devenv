---
title: "데몬 단독 저장소 SQLite 전환"
sage-review-design: required
---

# Daemon-only stores migration to SQLite

## Scope

Owner decision (1960)=a (2026-11-21) limits this epic to storage types 2-4
as defined in `ai-docs/manuals/performance.md` `## Storage Types`. Type 1
transcript and audit event logs use segment rings. The audit storage work
belongs to `261119-epic-ws-conversation-audit`.

Migrate the daemon-only, non-git-tracked runtime stores identified in the
2026-11-19 storage audit to SQLite (following the existing `modernc.org/sqlite`
WAL precedent in `agents-plugin-tool/internal/wsstore/store.go`, `6539b0c01`),
covering five candidate store groups where the owner expects dramatic
file-touch and complexity gains:

1. **`wslive` (`snapshot.json`, `model-evidence/`):** `<cache>/dashboard/sessions/<reply-id>/`
   ephemeral recovery cache. Daemon-only writer in 0.50+ with no external
   readers. Note: the transcript ring (`ring/`) is already RAM plus append-only
   segments (`07843a2f2`, `261118-feat-ws-display-transcript-memory-first` Phase
   2), so the remaining SQLite migration gain is `snapshot.json` and
   `model-evidence/` writes (formerly 4 file ops + `ReadDir(up to 2,000)` +
   `Remove` on `AppendRing` and 6 ops + `fsync` + `Link` + `Remove` +
   `pruneObserved` per model-evidence observation; 19/65 storage fix/perf
   commits, e.g. `aa9da6ac2`, `15c0409b1`, `bd3f4308c`, `2f3b2674b`).
2. **`wsinbox` (`inbox/`, `drained/`, `answer-pending/`):** `<cache>/dashboard/sessions/<reply-id>/inbox/`
   queue and 2PC crash journal. Daemon-only writer in 0.50+ (except direct CLI
   bypass); currently 4 ops per normal `Send`, ~11–14 ops + 2 `fsync` + 3
   `syncInboxDir` per `SendJournaled`, and `ReadDir` + per-entry `Rename` +
   Windows `NtSetInformationFile` workarounds on `Drain`/`Steer`/`Cancel` (10/37
   storage fix/perf commits, e.g. `f44bddbaa`, `4bfc94ce8`, `2f3b2674b`).
   (`attachments/` raw image blobs remain files.)
3. **`wswhiteboard` (`ai-docs/whiteboards/<reply-id>/`):** Repo-rooted but
   git-excluded (`.git/info/exclude`) 2PC WAL folder (`meta.json`,
   `committed.json`, `pending.json`, `.lock`, `tN/`, `panels/`). Daemon-only
   writer in 0.50+; currently ~25–50+ file ops + multiple `fsync`/dir-syncs per
   `whiteboard.update` (18/32 storage fix/perf commits, e.g. `6937612f9`,
   `45d559d09`, `872978ce4`, `f3e8bea7c`). (Derived `index.html`/`index.stamp.js`
   remain files if `file://` links remain; retiring the separate per-update
   embed HTML snapshot `ai-docs/whiteboards/<id>-<hash>.html` is also in scope
   as its own child.)
4. **Daemon-only metadata bundle:** Strictly daemon-internal JSON snapshots
   with zero external readers even outside the launcher:
   - `wsagent`: `agent.json` (also known as `record.json`; absorbs Phase 3
     `record.json` delta-append-log work moved from
     `261118-feat-ws-display-transcript-memory-first` by owner decision
     `(1326)=a`, 2026-11-19), `permission.json`, `support.json`, `pi-probes-v1/`
     (5 ops/write + poll `stat` on `permission.json`; 27/68 storage fix/perf
     commits, e.g. `c47ac38d6`, `539c6c778`, `6979a79e6`, `253ea7b41`; child
     inputs `pi/session.jsonl` and `system_prompt.md` stay files).
   - `wsdashboard`: `rail-layout.json`, `plan-usage*.json`, `memos/`,
     `label.json`, `ack-check.json`, `hiring.json`, `workspaces.json` (4–5
     ops/update; 11/25 storage fix/perf commits, e.g. `da771b684`, `95aa846d1`,
     `b9c3611a3`).
   - `wstrackerhook`: `<store-id>.json` cursor/processed/due state (4–5
     ops/batch; 2/4 storage commits, `edf30f571`, `3a04d584d`).
   - `wstrackerhost`: `ui-seen.json` (4–5 ops on `MarkSeen`; strictly
     daemon-only).
5. **MCP session key store (`mcp/session_layout.go`, `sessions/<key>/`):**
   `<cache>/sessions/<key>/{key,agenda,todos,whiteboard}.json` + `.lock` and
   legacy `keys/`. Daemon-only writer in 0.50+; currently ~7–9 ops per
   `agenda.set`/`todo.add`, `Stat`+`Chtimes` on reads, and 2x `ReadDir` across
   7,800+ observed session directories on `listKeys` (34/108 storage fix/perf
   commits, e.g. `9ee5b99e5`, `cae01a2c7`, `1f4b3ff45`, `fdf968c02`).

## Non-Scope

- **Excluded candidate — `wsmailbox`:** Already migrated to a memory-first
  segmented JSONL ring (`e973b72b4`, `ai-docs/manuals/performance.md:42-61`)
  achieving 1 append `write()` / 0 reads / 0 `fsync` per send with shutdown
  legacy export; converting it to SQLite WAL has low/negative payoff.
- **Must stay files:**
  - **Git-tracked stores:** `ai-docs/.review-ledger.md` (`wsreview` canary
    merge-conflict semantics), `ai-docs/ws-notes/<hex>.json` (`wsnote` repo
    layer), `.ws/config.json` (`wsconfig` repo layer), `ai-docs/tickets/**`
    (`wsdoc`).
  - **Git-transport / remote-synced stores:** `wsledger`
    (`~/.ws/tracker/<id>/repo.git` signed refs/commits), `wsindex`
    (`refs/ws/tickets-index`, `refs/ws/tickets-pending`).
  - **Files read or written by non-Go processes:**
    - `dashboard/{endpoint.json, dashboard.lock, launcher-start.lock, last-port, last-running-executable.json, localhost-secret, lan-token}`
      (read by `ws-mcp-launcher.py` and `desktop-electron`).
    - `dashboard/holders/<id>/{descriptor.json, exit.json, holder.log}`
      (written by detached Python `wsholder/holder.py`).
    - `dashboard/sessions/<reply-id>/registration.json` (read directly on disk
      by Pi `hosted-session.ts` and Claude mod `claude-mod/hosted.ts`,
      `register.ts`).
    - `~/.ws/config.json` (human-editable global config).
  - **Raw blobs and child-owned files:** `inbox/attachments/*`, `wstrackerhost`
    `attachments/*`, `wsagent` `pi/session.jsonl` & `system_prompt.md`,
    `execjob` `stdout`/`stderr`/`combined` logs, `dashboard/rollback/` staged
    binaries, and `ai-docs/whiteboards/<id>/index.html` (if `file://` opening
    remains).
- **Other stores outside this epic's five candidates:** `wsnote` non-repo
  layers (`machine`, `worktree`, `clone`), `wsstate` `project.json` /
  `worktree.json` / throttles, and `wstrackerhost` `stores.json` /
  `profiles.json` / `host-sync.json` are not included in the five confirmed
  targets.

## Child Tickets

Child execution order is **not yet decided** (see `## Open questions`):

- Planned: `wslive` (`snapshot.json`, `model-evidence/`) -> SQLite. Session transcripts use their Type 1 segment ring ((1960)=a).
- Planned: `wsinbox` (`inbox/`, `drained/`, `answer-pending/`; `attachments/` kept as blob files) -> SQLite
- Planned: `wswhiteboard` (2PC WAL folder -> one SQLite DB per board/session, lazily opened and idle-closed)
- Planned: `wswhiteboard` embed HTML snapshot retirement (stop rendering `ai-docs/whiteboards/<id>-<hash>.html` per update; updates shipped whiteboard playbook embed rule and `whiteboard.update` `embed:` result field; mechanism open)
- Planned: Daemon-only metadata bundle (`wsagent`, including absorbed `record.json` delta log from `261118-feat-ws-display-transcript-memory-first` Phase 3 per `(1326)=a`, `wsdashboard` misc UI state, `wstrackerhook` state, `wstrackerhost` `ui-seen.json`) -> SQLite
  - Must also remove the held-conversation applied-sequence rewrite: today each final ring line and each dirty heartbeat replaces the whole `agent.json` for held leads and, since `261120-refactor-ws-agent-workers-under-holders`, held workers (owner-approved performance exception `(1862)=a`, 2026-11-20).
- Planned: MCP session key store (`sessions/<key>/` + legacy `keys/` migration) -> SQLite
- Planned: Numbers screen (`261120-feat-ws-dashboard-numbers-screen`), with ~30-day harness/model statistics after the SQLite board store lands on `develop`

## Cross-Child Decisions

Confirmed by the owner on 2026-11-19 (21:52 and 21:56, board `(1325)=a`):

- **Five-store scope and exclusions:** Target all five candidate stores listed
  in `## Scope` for expected dramatic file-touch and complexity reductions;
  exclude `wsmailbox` (already memory-first JSONL ring, `e973b72b4`) and keep
  all git-tracked, git-transport, non-Go-accessed, and raw-blob files as files.
- **Whiteboard storage shape (`(1325)=a`, 2026-11-19 21:56):** One SQLite file
  per board (per session), opened lazily and closed when idle, with a small
  `cache_size`.
  - Refined by `(1347)=c` (owner, 2026-11-19 23:10): the handle stays open and
    closes only after a long idle (about 30 minutes), then reopens on next
    access; recorded in `261119-refactor-ws-whiteboard-sqlite-board-store`.
  - *Rejected:* One shared DB with a `board_id` column (boards share nothing; a
    per-board file keeps zip export/load and delete trivial); memory state +
    per-board append journal like `wsmailbox` (would leave the whiteboard on a
    different engine from the rest of the epic, and history queries would need
    the whole board in memory).
- **Whiteboard embed HTML snapshot retirement (owner allowance, 2026-11-19 21:56):**
  Retiring the per-update embed HTML snapshot rendering
  (`ai-docs/whiteboards/<id>-<hash>.html` written for hosts that embed the
  board inline; owner note: "참고로 임베드 html 렌더링에도 tact time이 꽤
  소요되는 거 같은데 이 경로 이제 버려도 되긴 합니다.") is in scope as its own
  child of this epic. Because it touches shipped whiteboard playbook text
  (embed rule) and the `whiteboard.update` result field `embed:`, that child
  must state its compatibility tier per
  `ai-docs/manuals/release-compatibility.md` and follow
  `ai-docs/manuals/shipped-surface-boundary.md`. The child's mechanism is not
  decided here.
- **SQLite implementation precedent:** Build on the existing
  `modernc.org/sqlite` configuration in
  `agents-plugin-tool/internal/wsstore/store.go` (WAL mode,
  `SetMaxOpenConns(1)`, `busy_timeout`, foreign keys, and `SQLITE_BUSY` /
  `SQLITE_LOCKED` backoff retry per `6539b0c01`), subject to Windows AV
  measurement and per-store open questions below.
- **Cross-cutting manual obligations:** Each child ticket must comply with
  `ai-docs/manuals/performance.md` (memory-first interactive paths, measured
  before/after file ops), `ai-docs/manuals/release-compatibility.md` (source-backed
  compatibility tier settled before ready), and
  `ai-docs/manuals/shipped-surface-boundary.md`.

## Open questions

The following architectural and sequencing choices are intentionally open and
must not be assumed by child tickets without owner confirmation:

- **Child ordering:** Which candidate store runs first as the pilot and the
  sequence across the children.
- **Per-store compatibility tier:** For each candidate store (and the embed
  snapshot retirement child), whether downgrade compatibility stays `<= T2` (via
  startup import + shutdown/dual-write export to legacy files) or accepts a
  `T3` minor boundary.
- **Windows AV benchmark (AhnLab V3) gate:** Using a corporate Windows PC
  running AhnLab V3 as the first gate to compare SQLite WAL (`-wal`/`-shm`,
  checkpoint `fsync`, `PRAGMA synchronous`) syscall counts and latency against
  current file stores / memory-first JSONL (`ai-docs/manuals/performance.md:65-73`).
- **Direct-binary CLI access policy:** How direct `ws-mcp` CLI/serve
  invocations bypassing `ws-mcp-launcher.py` (`cmd/ws-mcp/main.go`) are handled
  when the daemon holds a SQLite store (concurrent WAL access vs. explicit
  refusal, plus `daemon-start --replace` handoff behavior on Windows).

## Completion Criteria

- Done: All five candidate-store child tickets and the in-scope embed-snapshot
  retirement child are in `.done/` (or explicitly deferred/dropped by the
  owner), compatibility tiers and Windows AV measurements are verified per
  child, and the owner closes this epic.
- Dropped: The owner drops the SQLite migration direction.
- Deferred: Stores outside the five confirmed candidates (`wsnote` non-repo
  layers, `wsstate` throttles/project metadata, `wstrackerhost` config/sync
  files) and `wsmailbox` remain outside this epic unless separately ticketed.
