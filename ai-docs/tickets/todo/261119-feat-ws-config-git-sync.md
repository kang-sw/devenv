---
title: "설정 git 동기화: lead-tune 설정을 사용자 origin으로 동기화하는 synced 레이어"
related:
  261119-refactor-ws-config-drop-candidate-cleanup: "owner (1334)=a: the 28 DROP candidates leave this ticket and are re-verified and removed there; here they stay local and unsynced"
  260924-bug-ws-config-project-scope-is-machine-wide: "the `project` scope is one machine-wide file above `global`; its values shadow the synced layer on read (see Decision 10)"
  261116-feat-ws-dashboard-config-settings-ui: "dashboard settings surface; the sync-now button and sync status (Phase 4) sit in the same settings area and must not pre-empt its open questions"
  261117-research-ws-daemon-file-touch-map: "config resolution already reads several config files per call; the synced layer must not add a file touch per resolution"
  261112-feat-ws-mixed-version-compat-guards: "tier vocabulary and the remaining mixed-version guards"
  260930-bug-wsconfig-writer-replaces-symlinked-config: "the shared config writer the first-enable migration rewrites `~/.ws/config.json` through"
---

# Config git sync: a synced settings layer pushed to the user's own origin

## Background

Settings tuned through `config.tune` (lead-tune) live in machine-local files,
so a user who works on several machines re-tunes the same personal preferences
on each one. This ticket adds a settings layer that is synced through git to a
remote the user owns, so portable preferences follow the user across machines
while machine-bound and secret values stay local.

A per-key triage of every user-settable config element (lead-commissioned
read-only triage, 2026-11-19, against `develop`) classified 68 elements or key
families: **SYNC 23, SYNC+LOCAL-OVERRIDE 11, LOCAL 6, DROP-CANDIDATE 28**.
The full per-key table is copied in `## Appendix A`. Its evidence column cites
`develop` line numbers at triage time; re-check before editing.

Code facts at drafting time (`develop`, 2026-11-19; line numbers drift):

- Read precedence is `session > project > repo > global > builtin`
  (`agents-plugin-tool/internal/wsconfig/resolver.go:76-131`). Global-only keys
  skip the session/project/repo overlays and read only global, then builtin
  (`resolver.go:150-162`, `GlobalOnly` at `wsconfig/scope.go:287`).
- `global` is `~/.ws/config.json` (or `$WS_CONFIG_HOME/config.json`)
  (`wsconfig/global.go:17-29`). `project` is `CacheRoot/config.json`
  (`wsconfig/config.go:592-598`), which carries no project identity, so it is
  one machine-wide file (`260924-bug-ws-config-project-scope-is-machine-wide`).
- The default write scope is `DefaultScope(key)`: `global` for
  `preference.<slot>`, the registered scope otherwise, and `project` for an
  undeclared key (`wsconfig/scope.go:305-314`). The triage's structural caveat:
  `compaction.*`, `prompt.*`, `claude.compaction_*`, and `pi.compaction_*`
  default to `project`, so without a routing change their tunes keep landing in
  a machine-local file.
- The global file is also read outside `Resolver.Get`: compaction resolution
  (`wsconfig/compaction.go:94`), the scalar listing (`wsconfig/scalars.go:52`),
  structured overrides (`wsconfig/structured.go:89-93`), the agent tier overlay
  (`LoadAgentTierConfig`, `wsconfig/config.go:173-186`), the scoped show
  (`wsconfig/scoped_show.go:37`), and raw `GlobalOverride` callers
  (`cmd/ws-mcp/mailbox_sweep.go:157`, `cmd/ws-mcp/proxy.go:401`). Manifest
  keys (`claude.*`, `dashboard.*`, `pi.*`) resolve through `Resolver.Get`,
  the compaction path, or `resolveHarnessBucketed` inside `resolveConfigGet`
  (`internal/mcp/config_manifest.go:513-570`).
- Every project/global write goes through `updateConfigFile`, which decodes
  into the fixed `Config` struct and re-marshals it whole
  (`wsconfig/config.go:783-805`). An unknown top-level field in
  `~/.ws/config.json` is therefore dropped by any writer; a new entry inside the
  `overrides` map (`map[string]string`, `config.go:40`) survives.
- Config files are read through a stat-revalidated cache on every resolution
  (`wsconfig/config.go:105-121`).
- Precedent for a git-synced bare repo: the tracker ledger keeps
  `~/.ws/tracker/<id>/repo.git` and writes it through git plumbing
  (`internal/wsledger/store.go:45,189`, `internal/wsledger/gitio.go`,
  `internal/wsledger/remote.go`), with fetch and push under a network lock.
- The go module has no in-process git library (`agents-plugin-tool/go.mod`).
- `TrustedWrite` gates dashboard writes with execution authority
  (`internal/wsdashboard/access.go:211`).

## Decisions

Owner answers, 2026-11-19:

1. **(1327)=c — storage and precedence.** The synced layer is a dedicated bare
   repo at `~/.ws/config-sync/repo.git`, synced to the user's own origin. The
   synced layer sits **below** the machine-local `~/.ws/config.json`: a
   machine-local value wins over a synced one.
2. **(1328)=b — write routing.** Keys classified SYNC write to the synced
   layer by default. LOCAL keys and secret/path keys always write local.
   Secret/path keys that stay local: `agent.profile.<name>`,
   `agent.git_search_roots`, `worktree_pool`.
3. **(1332)=a — sync timing and merge.** Push on every tune, with a pull and a
   per-key merge right before it; pull on daemon start; and a "sync now"
   button on the dashboard. The merge is per-key last-writer-wins.
4. **(1333)=a — first enable.** On first enable, the local values of SYNC keys
   move into the synced layer and are deleted locally, with a backup kept. When
   the repo already holds a value for a key, the repo value wins. The file scope
   is narrowed by Decision 10 (global file only, class SYNC only).
5. **(1334)=a — DROP candidates.** The 28 DROP candidates are not handled in
   this ticket. Here they stay local (not synced); their re-verification and
   removal belong to `261119-refactor-ws-config-drop-candidate-cleanup`.

Owner answers on the open queue, 2026-11-19 (items 1701-1718):

6. **(1701)=ok — compatibility tier T0/T1 (owner 2026-11-19).**
   - Synced layer files under `~/.ws/config-sync/`: an older binary never opens
     them (no reader exists on `develop`), so T0.
   - First-enable migration deletes SYNC-key values from `~/.ws/config.json`:
     an older binary then resolves those keys to builtin defaults
     (`resolver.go:117-130`) and writes nothing, so T1 (display/behavior falls
     back to defaults; the values remain in the synced repo and in the backup).
     An older binary's own `config.tune` writes the machine-local file, which
     wins on the new binary per Decision 1, so no data is lost.
   - Any new machine-local sync setting is stored as an `overrides` map entry,
     never as a new top-level field, because every writer re-marshals the fixed
     struct (`config.go:783-805`) and would drop a top-level field (scenario 10
     hazard); map entries survive (`config.go:40`), keeping this T0.
   - The new `scope: "sync"` value on `config.tune`/`config.unset` (Decision 9)
     is T0 for legacy calls; a new caller passing it to an older server is
     refused by the argument precheck and needs capability gating
     (`release-compatibility.md` Tools and Protocol Mapping).
7. **(1702)=b — SYNC+LOCAL-OVERRIDE write target (owner 2026-11-19).** An
   unscoped `config.tune` of a SYNC+LOCAL-OVERRIDE key (11 keys, Appendix A)
   writes machine-local at the key's registered default scope; an explicit
   `scope: "sync"` (Decision 9) publishes the value. Rejected: writing to the
   synced layer by default like SYNC - a headless-host toggle silently
   propagating to every workstation is worse than a missed propagation, which
   one explicit tune repairs.
8. **(1703)=a — status of Appendix A (owner 2026-11-19).** Appendix A is the
   binding per-key list for this ticket, held in one Go classification table; a
   key or family absent from it is LOCAL until classified. Rejected: Appendix A
   as advisory with the owner confirming the SYNC list key by key before
   Phase 1 - the table is already per-key and evidenced, and LOCAL-by-default
   for unclassified keys keeps new keys (and possible secrets) off the remote
   until someone decides.
9. **(1704)=a — explicit write target (owner 2026-11-19).** `config.tune` and
   `config.unset` gain a `sync` scope value. `scope: "global"` keeps writing
   `~/.ws/config.json` (a deliberate machine-local override); `sync` on a LOCAL
   or secret/path key is refused with an error naming the key's class. Needs
   capability gating per Decision 6. Rejected: no new scope, with unscoped SYNC
   writes going to the synced layer and every explicit scope writing
   machine-local - both directions stay expressible with `sync`, and refusing
   `sync` for LOCAL keys makes the secret boundary an error instead of a silent
   redirect.
10. **(1705)=b — SYNC-key values in the machine-wide `project` file (owner
    2026-11-19).** The first-enable migration moves SYNC values from `global`
    (`~/.ws/config.json`) only. SYNC-key values found in the machine-wide
    `project` file are left untouched and reported to the user as shadowing the
    synced value; owner reason: values set in the project layer may be there
    for a reason. The committed `repo` scope is untouched. Only class SYNC
    values migrate: a machine-local value of a SYNC+LOCAL-OVERRIDE key is a
    deliberate local override by Decision 7 and stays put. Rejected: migrating
    both machine-local files (`project` and `global`) - it would move values
    the user set in the project layer deliberately.
11. **(1706)=b — where the git work runs (owner 2026-11-19).** `config.tune`
    updates the in-memory synced layer and its local snapshot and returns; a
    single-flight async daemon worker then pulls, merges, commits, and pushes.
    A tune that arrives while a sync runs schedules one follow-up sync, so
    every tune is pushed but bursts coalesce. Sync failures surface as status
    (Decision 17), not as a failed tune. Rejected: synchronous, with
    `config.tune` returning after the push succeeds or fails - it keeps several
    git spawns and a network round trip on the tool call.
12. **(1707)=ok — git transport (owner 2026-11-19).** Spawn the system `git`
    through the existing `wsgit` runner and reuse `wsledger`'s bare-repo
    plumbing pattern (`hash-object`/`commit-tree`/`readBlob`, network lock).
    Rejected: an in-process git library - the module has none (`go.mod`), and
    system git reuses the user's credential helpers and SSH setup for their own
    origin.
13. **(1708)=ok — performance Rule 8 exception (owner 2026-11-19).** Phase 2
    measures file operations per tune and tune-to-return latency, plus
    daemon-start cost, on the Windows baseline before and after, records them
    in the Result, and waits for owner approval of the git spawns before the
    phase lands. The measurement covers the Decision 11 worker's git spawns
    as well as the daemon-start pull.
14. **(1709)=ok — read path (owner 2026-11-19).** The resolution path never
    reads the bare repo. After each pull/merge or synced write, the daemon
    writes a materialized snapshot (`~/.ws/config-sync/config.json`, the same
    `Config` shape) through the existing locked writer; every reader loads it
    through the existing stat-revalidated cache as one more layer below
    `global`: `Resolver.Get`, `getGlobalOnly`, compaction, scalars, the agent
    tier overlay (`LoadAgentTierConfig`, because `agents.tier` can be synced
    explicitly per Decisions 7 and 9), and scoped show. Structured readers and
    raw `GlobalOverride` callers stay unchanged because no structured-override
    or `GlobalOverride` key is SYNC or SYNC+LOCAL-OVERRIDE in Appendix A as
    drafted; Phase 1 re-verifies this. Rejected: reading the bare repo on
    resolution - it would add a file touch or git spawn per `config.get`
    (Constraints).
15. **(1710)=ok — repo content and LWW clock (owner 2026-11-19).** One JSON
    document in the repo maps each key to `{value, updated_at, origin}`
    (`origin` an opaque random per-machine id, never a hostname or user name);
    an unset writes a tombstone so deletions propagate; merge keeps the newer
    `updated_at` per key, tie-broken by `origin`; tombstones older than a fixed
    horizon are compacted.
16. **(1711)=a — enabling sync and the remote (owner 2026-11-19).** Two LOCAL
    keys in the machine-local `overrides` map (e.g. `config_sync.remote`,
    `config_sync.enabled`), set through `config.tune` and shown in the
    dashboard; setting the remote plus enabling triggers the first-enable
    migration. The remote URL and enabled flag are per machine and never
    synced. Rejected: a dedicated tool or CLI subcommand that configures the
    bare repo's `origin` directly and keeps no config key - the keys give one
    settings surface, visible in `config.list`, and survive older writers.
17. **(1712)=ok — failure handling (owner 2026-11-19).** An offline remote, an
    auth failure, or a lost push race keeps the local commit and snapshot,
    retries on the next tune, daemon start, and sync-now, and records a
    last-sync status (time, outcome, error text without credentials) that the
    dashboard shows; a non-fast-forward push re-runs fetch + per-key merge
    before retrying once.
18. **(1713)=ok — only the daemon runs git (owner 2026-11-19).** A non-daemon
    process that writes a synced key updates the snapshot under the config lock
    and marks it dirty; the daemon pushes dirty state at its next start, tune,
    or sync-now.
19. **(1714)=a — dashboard sync-now placement (owner 2026-11-19).** A minimal
    sync-now button with last-sync status in the existing settings popover,
    behind a daemon route gated by `TrustedWrite`; the full settings sheet
    adopts it later. Rejected: waiting for the settings sheet and shipping the
    button inside it - (1332)=a asks for the button in this ticket, and a
    gated route plus small control does not pre-empt the sheet's design
    (`261116-feat-ws-dashboard-config-settings-ui` Q1, Q8 stay open).
20. **(1715)=ok — classification table (owner 2026-11-19).** Classification
    lives in one Go table (key or family -> SYNC / SYNC+LOCAL-OVERRIDE / LOCAL /
    DROP-CANDIDATE, plus a secret/path flag); no config-manifest field or
    `default_scope` value is added (Constraints). Adapter-manifest keys
    (`claude.*`, `dashboard.*`, `pi.*`) are classified in the same table by key
    name.
21. **(1716)=ok — first-enable backup (owner 2026-11-19).** Before deleting any
    local value, write one copy of each machine-local config file the migration
    will change to `~/.ws/config-sync/backup/<UTC timestamp>-<file>.json`. Per
    Decision 10 the migration changes only `~/.ws/config.json`, so only that
    file is backed up; the `project` file is not modified and not backed up.
    The migration is idempotent and records completion in the machine-local
    `overrides` map so it runs once per machine.
22. **(1717)=ok — commit-time guard (owner 2026-11-19).** Before every commit
    to the synced repo, the writer verifies that every key in the document is
    SYNC or SYNC+LOCAL-OVERRIDE class (SYNC+LOCAL-OVERRIDE keys reach the repo
    only through an explicit `scope: "sync"` per Decisions 7 and 9) and aborts
    the commit otherwise, so a classification bug cannot push a LOCAL or
    secret/path value.
23. **(1718)=ok — scope label on read (owner 2026-11-19).** `config.get` and
    `config.list` report a value from the synced layer with a distinct scope
    label (e.g. `sync`); older callers only display the string (T0/T1 per
    Decision 6).

## Constraints

- **Compatibility tier.** This change touches a shared store's on-disk shape
  (`~/.ws/config.json` contents after migration, a new synced layer) and
  a tool argument (the `sync` scope value, Decision 9). The tier is settled in
  Decision 6 (T0/T1; the `sync` scope needs capability gating)
  (`ai-docs/manuals/release-compatibility.md`, AGENTS.md
  `### Compatibility Tiers`).
- **No config-manifest field change.** The manifest parser refuses unknown
  fields (`internal/mcp/config_manifest.go:179`) and refuses a
  `default_scope` outside `session|project|global`
  (`config_manifest.go:272-281`); a new field or scope value there is a T3 read
  boundary (`release-compatibility.md` scenario 11). Per-key sync
  classification therefore lives in Go code, not in manifest fields. If a new
  manifest *key* is ever added, ws and wsflow manifests must stay
  key-identical with only `default` differing
  (`TestShippedClaudeManifestsDifferOnlyInDefaults`,
  `internal/mcp/config_manifest_parity_test.go:51`;
  `ai-docs/manuals/wsflow-mirroring.md`), including the native-pi
  first-party and dashboard manifest copies.
- **Windows file touches and process spawns**
  (`ai-docs/manuals/performance.md`). The tune path gains git work: Rule 5
  (interactive paths do not wait on file I/O), Rule 6 (spawn a process only
  when no in-process route exists), and Rule 4 (no per-call open/stat) apply.
  Any git spawn on the tune path or at daemon start is a Rule 8 exception that
  states its measured cost (file operations per tune and tune latency on the
  Windows baseline) and waits for owner approval; see Decisions 11 and 13. The
  resolution path must not add a file touch or spawn per `config.get`.
- **No secret leaves the machine.** LOCAL and secret/path keys never enter the
  synced repo, on any write path and in the first-enable migration.
- **Old config files keep working.** A `~/.ws/config.json` written by an older
  version, including values of SYNC keys, stays readable; after enable, a
  machine-local value still wins (Decision 1).
- **Repo is public.** Tests and fixtures use placeholder remotes and
  isolated homes; no real remote URL, credential, or user path is committed.

## Phases

### Phase 1: Synced layer and classification (no git)

Uses Decisions 6-10, 14, 20, 22, and 23.

- Add the classification table (Appendix A, binding per Decision 8) with the
  secret/path flag on `agent.profile.<name>`, `agent.git_search_roots`, and
  `worktree_pool`; keys absent from it are LOCAL.
- Add the synced layer below `global` on every read path that reads the global
  file for a syncable key (SYNC or SYNC+LOCAL-OVERRIDE), including global-only
  keys and the agent tier overlay (Decision 14); label its values `sync` in
  `config.get`/`config.list` (Decision 23).
- Route writes per Decisions 2, 7, and 9: unscoped SYNC writes go to the synced
  layer; unscoped SYNC+LOCAL-OVERRIDE writes go machine-local; `scope: "sync"`
  writes the synced layer for SYNC and SYNC+LOCAL-OVERRIDE keys and is refused
  for LOCAL and secret/path keys with an error naming the class; LOCAL and
  secret/path keys always write local; DROP candidates stay local
  (Decision 5). Capability-gate the `sync` scope (Decision 6).
- Add the commit-time guard in the writer (Decision 22), allowing SYNC and
  SYNC+LOCAL-OVERRIDE keys and refusing LOCAL and secret/path keys.
- Deferred: git, migration, dashboard.
- Verification: resolution-order tests (machine-local beats synced beats
  builtin, global-only keys and the agent tier overlay included,
  project/session still above); routing tests per class, including the
  SYNC+LOCAL-OVERRIDE unscoped-local and explicit-`sync` cases and refusal of
  `sync` for LOCAL and secret/path keys; the guard refuses a LOCAL or
  secret/path key and accepts SYNC and SYNC+LOCAL-OVERRIDE keys; an
  older-format `~/.ws/config.json` fixture still resolves unchanged.

### Phase 2: Git transport

Depends on Phase 1.

- Initialise or clone `~/.ws/config-sync/repo.git` against the user's
  configured remote (per Decision 16).
- A single-flight async daemon worker does pull + per-key LWW merge + commit +
  push after every synced tune (Decisions 3, 11, and 15), with one coalesced
  follow-up when a tune arrives mid-sync; the tune returns without waiting.
  Pull at daemon start; failure handling and last-sync status per Decision 17;
  only the daemon runs git (Decision 18).
- Verification: two isolated homes sharing a local bare remote converge per
  key under concurrent tunes, offline retry, and a lost push race; no LOCAL or
  secret/path key ever appears in the repo (Decision 22); a tune returns
  before the push completes and a burst of tunes coalesces; Windows
  measurements per Decision 13 cover the worker's git spawns and daemon start.

### Phase 3: First-enable migration

Depends on Phase 2 (the repo-wins rule needs the remote state).

- On first enable, move machine-local SYNC values into the synced layer and
  delete them locally, keeping a backup; where the repo already holds a value,
  the repo value wins (Decision 4).
- Migrate only class SYNC values from `~/.ws/config.json` (Decision 10); leave
  SYNC-key values in the machine-wide `project` file untouched and report them
  to the user as shadowing the synced value, quoting the owner's reason (values
  set in the project layer may be there for a reason).
- Back up only the file the migration changes, `~/.ws/config.json`
  (Decision 21).
- Verification: first machine (empty repo) publishes its values; second
  machine with conflicting local values adopts the repo values; backup exists
  and restores byte-identically and no backup of the `project` file is made; a
  `project`-file SYNC value is not modified and is reported as shadowing; a
  re-run is a no-op; secret/path, LOCAL, and SYNC+LOCAL-OVERRIDE local values
  are untouched.

### Phase 4: Dashboard sync-now

Depends on Phase 2.

- A sync-now control and last-sync status on the dashboard (placement per
  Decision 19), behind a `TrustedWrite`-gated daemon route.
- Verification: route refuses untrusted requests; a click runs one sync and
  updates the status; localized strings pass the dashboard i18n checks.

## Appendix A: Per-key classification (triage, 2026-11-19)

Copied from the triage table. "Current default write scope" is today's
behavior, not the post-ticket routing. Evidence lines are `develop` at triage
time.

| key | what it controls | current default write scope | verdict | one-line reason | evidence file:line |
|---|---|---|---|---|---|
| `compaction.lead_advisory` (`[.<claude\|codex\|pi\|agy>]`) | Hosted lead advisory compaction threshold (percent and/or token count) | `project` | SYNC | Portable context-pressure threshold; per-harness suffix handles host differences | `agents-plugin-tool/internal/mcp/config_registry.go:249`, `agents-plugin-tool/internal/wsconfig/compaction.go:13,66-108`, `agents-plugin-tool/internal/wsconfig/scope.go:312` |
| `compaction.lead_hard` (`[.<claude\|codex\|pi\|agy>]`) | Hosted lead hard compaction trigger threshold | `project` | SYNC | Portable context ceiling before native compaction queues; per-harness suffix handles host differences | `agents-plugin-tool/internal/mcp/config_registry.go:250`, `agents-plugin-tool/internal/wsconfig/compaction.go:13,66-108`, `agents-plugin-tool/internal/wsconfig/scope.go:312` |
| `compaction.worker_advisory` (`[.<claude\|codex\|pi\|agy>]`) | Hosted worker advisory compaction threshold | `project` | SYNC | Portable worker context-pressure threshold independent of OS or machine | `agents-plugin-tool/internal/mcp/config_registry.go:251`, `agents-plugin-tool/internal/wsconfig/compaction.go:14,66-108`, `agents-plugin-tool/internal/wsconfig/scope.go:312` |
| `compaction.worker_hard` (`[.<claude\|codex\|pi\|agy>]`) | Hosted worker hard compaction trigger threshold | `project` | SYNC | Portable worker context ceiling before native compaction queues | `agents-plugin-tool/internal/mcp/config_registry.go:252`, `agents-plugin-tool/internal/wsconfig/compaction.go:14,66-108`, `agents-plugin-tool/internal/wsconfig/scope.go:312` |
| `hosted.plugin_package` | Selects embedded `ws` vs `wsflow` plugin package for daemon-launched processes | `global` (global-only) | SYNC+LOCAL-OVERRIDE | Portable mode preference (`ws` vs `wsflow`), though restricted/headless hosts may differ | `agents-plugin-tool/internal/wsconfig/scope.go:34,203`, `agents-plugin-tool/internal/mcp/config_registry.go:254`, `agents-plugin-tool/internal/wsagent/hosted_plugins.go:20-31` |
| `workflow.prefer_subagent` | Enables default delegation of eligible work through `lead-delegate` | `global` (global-only) | SYNC | Machine-independent personal delegation posture | `agents-plugin-tool/internal/wsconfig/scope.go:38,204`, `agents-plugin-tool/internal/mcp/config_registry.go:267` |
| `workflow.whiteboard_enabled` (`[.<claude\|codex\|pi\|all>]`) | Makes session whiteboard the lead's primary user channel per harness | `global` | SYNC+LOCAL-OVERRIDE | Portable UI habit per harness, but headless/SSH hosts commonly disable browser-first channel | `agents-plugin-tool/internal/wsconfig/scope.go:47,209-212`, `agents-plugin-tool/internal/mcp/config_registry.go:280`, `agents-plugin-tool/internal/mcp/server_config.go:346` |
| `workflow.tracker_enabled` (`[.<claude\|codex\|pi\|all>]`) | Gates tracker instructions in workflow manual per harness | `global` | SYNC | Machine-independent personal workflow habit per harness | `agents-plugin-tool/internal/wsconfig/scope.go:58,215-218`, `agents-plugin-tool/internal/mcp/config_registry.go:308` |
| `workflow.profile` (`[.<claude\|codex\|pi\|all>]`) | Selects `direct` vs `orchestrate` lead playbook profile per harness | `global` | SYNC | Portable orchestration preference already bucketed per harness | `agents-plugin-tool/internal/wsconfig/scope.go:67,221-224`, `agents-plugin-tool/internal/mcp/config_registry.go:336` |
| `bootstrap_alarm` | Gates `AGENTS.md` template-version staleness warning | `global` (global-only) | DROP-CANDIDATE | Rarely tuned from default `on`; syncing adds negligible value | `agents-plugin-tool/internal/wsconfig/scope.go:130,226`, `agents-plugin-tool/internal/mcp/config_registry.go:364` |
| `agent.git_search_roots` | Extra absolute directory paths that `git.search` may read | `global` (global-only) | LOCAL | Stores absolute paths, OS path-list separators (`:` vs `;`), and local read-security scope | `agents-plugin-tool/internal/wsconfig/scope.go:139,227`, `agents-plugin-tool/internal/mcp/config_registry.go:379` |
| `agent.tier_aliases` | User-defined `name=<harness>.<tier>` aliases for `agent.spawn` | `global` (global-only) | SYNC+LOCAL-OVERRIDE | Alias vocabulary is portable, but target harnesses depend on locally installed/authed CLIs | `agents-plugin-tool/internal/wsconfig/scope.go:145,228`, `agents-plugin-tool/internal/mcp/config_registry.go:393` |
| `agent.child_waiting_days` | Days before daemon deletes idle waiting child agents (default `15`) | `global` (global-only) | DROP-CANDIDATE | Internal daemon retention TTL rarely tuned from 15 days | `agents-plugin-tool/internal/wsconfig/scope.go:151,229`, `agents-plugin-tool/internal/mcp/config_registry.go:411` |
| `sage_review` | Completeness-stage review gate (`off\|ask\|auto`) at `ready/` promotion | `project` | SYNC | Machine-independent personal review-rigor baseline (repos/projects still override locally) | `agents-plugin-tool/internal/wsconfig/scope.go:74,232`, `agents-plugin-tool/internal/mcp/config_registry.go:430` |
| `sage_review_design` | Design-stage review gate (`off\|ask\|auto`) at `ready/` promotion | `project` | SYNC | Machine-independent personal opt-in for agent-heavy workflows (repos/projects override locally) | `agents-plugin-tool/internal/wsconfig/scope.go:81,233`, `agents-plugin-tool/internal/mcp/config_registry.go:447` |
| `ticket_review_auto_ack` | Allows `ticket-handler` to auto-confirm autonomous/technical queue items | `project` | SYNC | Machine-independent personal ticket-settlement autonomy preference | `agents-plugin-tool/internal/wsconfig/scope.go:90,236`, `agents-plugin-tool/internal/mcp/config_registry.go:464` |
| `review_phase` | Worker per-phase independent code review level (`off\|lite\|full`) | `project` | SYNC | Machine-independent review-depth baseline across machines | `agents-plugin-tool/internal/wsconfig/scope.go:99,240`, `agents-plugin-tool/internal/mcp/config_registry.go:481` |
| `workflow.lang` | User's preferred conversation and summary language | `global` | SYNC | Pure user preference; completely machine-independent and desired everywhere | `agents-plugin-tool/internal/wsconfig/scope.go:115,244`, `agents-plugin-tool/internal/mcp/config_registry.go:502` |
| `agents.tier` (`agents.model_aliases.<tier>.<harness>`, legacy `agents.tiers`) | Maps `small\|medium\|large\|xlarge` per harness to `{backend, model, effort}` | `project` | SYNC+LOCAL-OVERRIDE | Portable tier-to-model preferences, but machines commonly differ in installed CLIs and auth | `agents-plugin-tool/internal/mcp/config_registry.go:521-560`, `agents-plugin-tool/internal/wsconfig/config.go:50-53,173-186,286-292` |
| `preference.<slot>` | Named standing user preferences rendered into `lead-workflow-manual` | `global` | SYNC | Core cross-machine user preference text; already defaults to `global` | `agents-plugin-tool/internal/wsconfig/scope.go:306-308`, `agents-plugin-tool/internal/wsconfig/scalars.go:66-87`, `agents-plugin-tool/internal/mcp/config_preferences.go:9-60` |
| `prompt.UserPreferenceSection.<claude\|codex\|pi\|all>` | Base/legacy standing user-preference prompt section per harness | `project` | SYNC | Core per-harness user-preference prompt text (note current default write scope is `project`) | `agents-plugin-tool/internal/mcp/config_registry.go:570-597`, `agents-plugin-tool/internal/wsconfig/scope.go:312`, `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md:36` |
| `prompt.TicketHandlerPersistConfirmation.<claude\|codex\|pi\|all>` | Internal override splice for `ticket-handler` confirmation-record rule | `project` | DROP-CANDIDATE | Internal playbook splice driven by `ticket_review_auto_ack`, not a user-preference prompt | `agents-plugin-tool/internal/mcp/config_registry.go:570-597`, `agents-plugin/rsrc/ticket-handler/ticket-handler.md:37`, `agents-plugin-tool/internal/mcp/ticket_review_text_test.go:86-118` |
| `prompt.TicketHandlerReviewAutoAck.<claude\|codex\|pi\|all>` | Internal override splice for `ticket-handler` auto-confirm rules | `project` | DROP-CANDIDATE | Internal playbook splice driven by `ticket_review_auto_ack`, not a user-preference prompt | `agents-plugin-tool/internal/mcp/config_registry.go:570-597`, `agents-plugin/rsrc/ticket-handler/ticket-handler.md:77`, `agents-plugin-tool/internal/mcp/ticket_review_text_test.go:86-118` |
| `prompt.TicketQueueWhiteboard.<claude\|codex\|pi\|all>` | Internal override splice for `lead-ticket` whiteboard queue rule | `project` | DROP-CANDIDATE | Internal playbook splice driven by `workflow.whiteboard_enabled`, not a user-preference prompt | `agents-plugin-tool/internal/mcp/config_registry.go:570-597`, `agents-plugin/rsrc/lead-ticket/lead-ticket.md:99`, `agents-plugin-tool/internal/mcp/ticket_review_text_test.go:86-118` |
| `worktree.profile.<name>` (`structured_overrides`) | Worktree pool partition, sparse-checkout, preserve patterns, and lifecycle hooks | `project` | DROP-CANDIDATE | Repo/project construct (belongs in `.ws-workflow/config.json`); rarely tuned globally and hooks are OS/tool-bound | `agents-plugin-tool/internal/mcp/worktree_profile.go:22,288-307`, `agents-plugin-tool/internal/mcp/config_structured.go:116-119`, `agents-plugin-tool/internal/mcp/server_config.go:575` |
| `agent.profile.<name>` (`structured_overrides`) | Named headless agent profile (`backend`, `tools`, `ws_tools`, `mcp_servers`, `plugins`, `instructions`, `lead`) | `project` | LOCAL | Can hold secrets (`mcp_servers` env/headers), absolute paths (`plugins`, `instructions`), and host-specific CLIs | `agents-plugin-tool/internal/mcp/agent_profile.go:13-32`, `agents-plugin-tool/internal/mcp/config_structured.go:116-119`, `agents-plugin-tool/internal/wsagent/profile.go:80-129,332-357` |
| `worktree.nudge_stale_days` | Inactivity days before `worktree.acquire` nudges about a pooled worktree (default `7`) | `project` | DROP-CANDIDATE | Local pool cleanup threshold rarely tuned from default 7 days | `agents-plugin-tool/internal/wsconfig/scope.go:189,249`, `agents-plugin-tool/internal/mcp/worktree_nudge.go:22-30` |
| `worktree.nudge_preserved_gb` | Preserved ignored-artifact GB threshold for detached worktree nudge (default `1`) | `project` | DROP-CANDIDATE | Local disk-usage threshold rarely tuned from default 1 GB | `agents-plugin-tool/internal/wsconfig/scope.go:190,250`, `agents-plugin-tool/internal/mcp/worktree_nudge.go:31-39` |
| `git.merge_size_soft_lines` | Soft line-count limit for `git.merge` advisory file-size warning (default `1000`) | `project` | DROP-CANDIDATE | Per-repo codebase convention (`.ws-workflow/config.json`), rarely tuned globally | `agents-plugin-tool/internal/wsconfig/scope.go:197,251`, `agents-plugin-tool/internal/mcp/git_merge_size.go:19-27` |
| `git.merge_size_hard_lines` | Hard line-count limit for `git.merge` advisory file-size warning (default `2000`) | `project` | DROP-CANDIDATE | Per-repo codebase convention (`.ws-workflow/config.json`), rarely tuned globally | `agents-plugin-tool/internal/wsconfig/scope.go:198,252`, `agents-plugin-tool/internal/mcp/git_merge_size.go:28-36` |
| `git.merge_size_extensions` | Space-separated file extensions checked by `git.merge` size warning | `project` | DROP-CANDIDATE | Per-repo language/extension set, rarely tuned globally | `agents-plugin-tool/internal/wsconfig/scope.go:199,253`, `agents-plugin-tool/internal/mcp/git_merge_size.go:37-44` |
| `workflow.skeptical_posture` | Prepends evidence-posture block to `workflow_manual` (unlisted in `config.tune`) | `global` (global-only) | DROP-CANDIDATE | Has no `config.tune` writer and defaults `on`; rarely tuned | `agents-plugin-tool/internal/wsconfig/scope.go:123,225`, `agents-plugin-tool/internal/mcp/workflow_manual.go:369-370` |
| `worktree_pool` | Directory path or `$(GitRoot)` template for pooled Git worktrees (no `config.tune` writer) | `project` | LOCAL | Filesystem path / OS layout choice (may store an absolute path) | `agents-plugin-tool/internal/wsconfig/scope.go:182,248`, `agents-plugin-tool/internal/mcp/worktree_tools.go:190,219-230` |
| `ticket-assignee-aware` | Opt-in project gate for ticket assignee tracking (no `config.tune` writer) | `project` | DROP-CANDIDATE | Committed per-repo team decision in `.ws-workflow/config.json`, not a user-global setting | `agents-plugin-tool/internal/wsconfig/scope.go:165,257`, `agents-plugin-tool/internal/mcp/server_tickets.go:14-26` |
| `sage_review_design_tier` | Registered `scope.go` constant for design reviewer tier (unused/orphaned) | `project` | DROP-CANDIDATE | Dead constant in `scope.go`; never read or written by `config.tune` or runtime | `agents-plugin-tool/internal/wsconfig/scope.go:104,241` |
| `sage_review_completeness_tier` | Registered `scope.go` constant for completeness reviewer tier (unused/orphaned) | `project` | DROP-CANDIDATE | Dead constant in `scope.go`; never read or written by `config.tune` or runtime | `agents-plugin-tool/internal/wsconfig/scope.go:108,242` |
| `claude.compaction_advisory_enabled` | Enables Claude Code lead compaction advisory and reminder turns | `project` | SYNC | Portable Claude workflow behavior (also differs between `ws` and `wsflow` defaults) | `agents-plugin/config-manifest.json:6-10`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `claude.compaction_advisory_percent` | Context-usage threshold for Claude Code lead compaction advisory (and hosted fallback) | `project` | SYNC | Portable compaction threshold desired consistently across machines | `agents-plugin/config-manifest.json:12-17`, `agents-plugin-tool/internal/wsagent/catalog.go:392`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `claude.manual_injection_enabled` | Enables Claude Code lead session key minting and workflow manual injection | `project` | DROP-CANDIDATE | Core plugin mechanism defaulting `true`; rarely turned off | `agents-plugin/config-manifest.json:19-23`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `claude.mailbox_drain` | Selects `direct` vs `notice` mailbox delivery in Claude Code | `project` | SYNC | Machine-independent UX preference (`direct` vs `notice`) | `agents-plugin/config-manifest.json:25-33`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `claude.display_summary_enabled` | Enables second-model tool-row captions in Claude Code terminal | `project` | SYNC+LOCAL-OVERRIDE | Portable UX toggle, but extra token cost/privacy rules differ across work vs personal machines | `agents-plugin/config-manifest.json:35-39`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `claude.status_band_enabled` | Draws framed status band above Claude Code prompt | `project` | SYNC | Pure terminal UI preference, independent of machine | `agents-plugin/config-manifest.json:41-45`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `claude.dashboard_enabled` | Hands Claude Code live activity to dashboard and accepts dashboard input | `project` | SYNC+LOCAL-OVERRIDE | Portable default, but headless/restricted hosts commonly turn dashboard feed off | `agents-plugin/config-manifest.json:47-51`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `claude.display_summary_model` | Model alias or full model ID for Claude Code tool-row captions | `global` | SYNC+LOCAL-OVERRIDE | `haiku` alias is portable, while custom/provider-specific model IDs can vary by host auth | `agents-plugin/config-manifest.json:53-58` |
| `claude.display_summary_style` | Prompt style guidance for Claude Code tool-row captions | `global` | SYNC | Pure prompt style text preference; completely machine-independent | `agents-plugin/config-manifest.json:60-65` |
| `claude.display_summary_context_tokens` | Context window size in tokens for `claude.display_summary_model` | `project` | DROP-CANDIDATE | Derived from the summary model (`200000`); rarely tuned independently | `agents-plugin/config-manifest.json:67-72`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `claude.mcp_watchdog_enabled` | Enables periodic `/mcp reconnect` watchdog when Claude Code idles | `project` | DROP-CANDIDATE | Host/network reconnect workaround rarely tuned from package default | `agents-plugin/config-manifest.json:74-78`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `claude.mcp_watchdog_interval_minutes` | Polling interval in minutes for Claude Code MCP watchdog | `project` | DROP-CANDIDATE | Internal timer constant rarely tuned from 10 minutes | `agents-plugin/config-manifest.json:80-85`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `dashboard.enabled` | Lists lead sessions on local dashboard and opens browser | `project` | SYNC+LOCAL-OVERRIDE | Desired on workstations, commonly disabled locally on headless/SSH machines | `agents-plugin/dashboard-config-manifest.json:6-10`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `dashboard.port` | TCP loopback port for dashboard daemon (default `29417`) | `project` | LOCAL | Host-specific port allocation and local port conflict avoidance | `agents-plugin/dashboard-config-manifest.json:12-18`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `dashboard.lan_enabled` | Exposes dashboard daemon on LAN interfaces with access token | `global` (global-only) | LOCAL | Network exposure and host security boundary; unsafe to replicate across machines | `agents-plugin/dashboard-config-manifest.json:20-25`, `agents-plugin-tool/internal/mcp/dashboard_config.go:21,32` |
| `dashboard.mailbox_undelivered_minutes` | Minutes before daemon sends undelivered-mail notice (default `3`) | `global` (global-only) | DROP-CANDIDATE | Internal daemon sweep threshold rarely tuned from 3 minutes | `agents-plugin/dashboard-config-manifest.json:27-33`, `agents-plugin-tool/internal/mcp/dashboard_config.go:23,33` |
| `dashboard.pi_spawn_enabled` | Exposes `pi.*` subagent tools when `pi >= 1.0` is installed | `project` | LOCAL | Directly gated on whether `pi >= 1.0` CLI is installed and usable on this machine | `agents-plugin/dashboard-config-manifest.json:35-39`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `pi.agent_wait_animation` | Animates Pi live-agent widget while child waits | `global` | DROP-CANDIDATE | Minor visual polish rarely tuned from default `true` | `agents-plugin/native-pi/first-party/config-manifest.json:6-11` |
| `pi.agent_widget_rows` | Tool-batch rows per agent in expanded Pi `/agents` widget | `global` | DROP-CANDIDATE | Minor UI row cap (`3`, auto-shrinks on short terminals); rarely tuned | `agents-plugin/native-pi/first-party/config-manifest.json:13-20` |
| `pi.agent_widget_batch_summary_enabled` | Summarizes child tool batches in Pi live-agent widget | `global` | SYNC+LOCAL-OVERRIDE | Portable UX preference, but extra quota cost/privacy posture commonly differs per machine | `agents-plugin/native-pi/first-party/config-manifest.json:22-27` |
| `pi.dashboard_enabled` | Hands Pi session activity to ws dashboard and accepts dashboard input | `global` | SYNC+LOCAL-OVERRIDE | Portable default, commonly overridden off on headless/isolated machines | `agents-plugin/native-pi/first-party/config-manifest.json:29-34` |
| `pi.runaway_threshold` | Consecutive Pi goal-loop reminders with no tool call before force-stop | `project` | DROP-CANDIDATE | Internal safety loop counter rarely tuned from 10 | `agents-plugin/native-pi/first-party/config-manifest.json:36-41`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `pi.settle_delay_ms` | Goal-loop settle wait / wake retry delay in milliseconds | `project` | DROP-CANDIDATE | Internal timing constant rarely tuned from 5000ms | `agents-plugin/native-pi/first-party/config-manifest.json:43-48`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `pi.compaction_advisory_percent` | Pi lead compaction advisory threshold (and hosted Pi fallback) | `project` | SYNC | Portable compaction threshold desired consistently across machines | `agents-plugin/native-pi/first-party/config-manifest.json:50-55`, `agents-plugin-tool/internal/wsagent/catalog.go:565` |
| `pi.compaction_hard_percent` | Pi lead hard compaction steer threshold (and hosted Pi fallback) | `project` | SYNC | Portable hard compaction threshold across machines | `agents-plugin/native-pi/first-party/config-manifest.json:57-62`, `agents-plugin-tool/internal/wsagent/catalog.go:565` |
| `pi.context_window_override` | Overrides model context window size for Pi percent calculation | `project` | DROP-CANDIDATE | Rarely tuned fallback (`0` uses model's own window) | `agents-plugin/native-pi/first-party/config-manifest.json:64-69`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `pi.compaction_dialog_budget_bytes` | Byte budget for dialog section of Pi lead compaction summary | `project` | DROP-CANDIDATE | Internal summary byte budget rarely tuned from 40960 | `agents-plugin/native-pi/first-party/config-manifest.json:71-76`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `pi.compaction_reread_budget_bytes` | Byte budget for inlined required re-reads in Pi lead compaction summary | `project` | DROP-CANDIDATE | Internal summary byte budget rarely tuned from 40960 | `agents-plugin/native-pi/first-party/config-manifest.json:78-83`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `pi.child_retention_ttl_days` | Days before Pi prunes stored child homes at lead startup | `global` | DROP-CANDIDATE | Local disk-cleanup TTL rarely tuned from 30 days | `agents-plugin/native-pi/first-party/config-manifest.json:85-91` |
| `pi.display_summary_model` | `provider/model-id` for Pi terminal collapsed-row summaries | `global` | SYNC+LOCAL-OVERRIDE | Portable choice when providers match, but provider auth/availability differs per machine | `agents-plugin/native-pi/first-party/config-manifest.json:93-98` |
| `pi.display_summary_effort` | Thinking level for `pi.display_summary_model` | `global` | SYNC | Machine-independent cost/detail preference; auto-clamped to the active model | `agents-plugin/native-pi/first-party/config-manifest.json:100-114` |
| `pi.display_summary_style` | Prompt style guidance for Pi display summaries | `global` | SYNC | Pure prompt/style text preference; completely machine-independent | `agents-plugin/native-pi/first-party/config-manifest.json:116-121` |
