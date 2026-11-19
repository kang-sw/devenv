---
title: "감사 SQLite 저장소: 저장 전 마스킹과 보존 기간"
parent: 261119-epic-ws-conversation-audit
blocked-by: 261119-feat-ws-audit-policy-event-envelope
related:
  261119-epic-ws-daemon-sqlite-storage: "dependency: SQLite precedent, cross-child obligations and open gates this store must not settle on its own"
  261119-feat-ws-audit-capture-harness-adapters: "producer: writes envelopes into this store; may proceed in parallel"
  261119-feat-ws-audit-query-api: "consumer: reads this store"
  261112-feat-ws-mixed-version-compat-guards: "planned newer-schema write refusal for sqlite stores (user_version)"
  261114-feat-ws-conversation-audit-event-store: "superseded source of (594) rationale, (596), (597) and the file-mode/non-blocking constraints"
---

# Audit SQLite store, redaction and retention

## Background

Third child of `261119-epic-ws-conversation-audit` ((1331)=a, 2026-11-19).
It persists the envelopes child 2 captures, redacts them before they touch
disk, and bounds how long and how much is kept.

(1331)=a moved audit storage onto SQLite, depending on
`261119-epic-ws-daemon-sqlite-storage`. That replaces the superseded
ticket's per-session JSONL segments plus sidecar index ((594)), which
overlapped both the display ring's append-only segments and the SQLite epic
(codex.large read-only research, 2026-11-19).

Verified findings (same research):

- The display ring is intentionally lossy and unusable as an audit store.
- Redaction is regex best-effort: `wsredact` documents that a secret in an
  unrecognized shape passes through (`agents-plugin-tool/internal/wsredact/redact.go:1-9`).

## Decisions

1. **A separate audit store beside the ring ((594) rationale, carried).** The
   ring is a capped UI view whose trimming would discard audit data. Full
   tool outputs would bloat every history page read. The store is SQLite per
   (1331)=a. The JSONL-segment/sidecar-index form of (594) is superseded.
2. **Redact before persist ((596), carried).** Before writing, `wsredact`
   runs over `in`, `out` and `raw`. A redaction keeps the rule name and the
   original length, never the original (`redactions: [{field, offset, rule,
   length}]`). This extends `wsredact` with a match-reporting entry point and
   keeps the `Secrets(s) string` signature for existing callers. Rejected:
   storing unredacted data in a separate secure store, which is a new
   key-management surface. What else secret handling needs is (1344).
3. **Byte cap ((597), carried in part).** A per-session byte cap (config
   knob, default 256 MiB) drops the oldest data first, because one runaway
   tool output could fill the disk within any time window. (597)'s time rule
   (the 7-day session prune) is re-asked as (1343). Its drop unit (whole
   segments) is superseded by the SQLite layout.
4. **Compatibility tier T0, with its own schema version.** A new store with no
   older reader. The schema version lives in the database (SQLite
   `user_version`). A daemon refuses to open a newer version for writing, in
   line with `261112-feat-ws-mixed-version-compat-guards`.
5. **SQLite precedent.** Follow the existing `modernc.org/sqlite`
   configuration in `agents-plugin-tool/internal/wsstore/store.go` (WAL,
   `SetMaxOpenConns(1)`, `busy_timeout`, foreign keys, `SQLITE_BUSY`/
   `SQLITE_LOCKED` backoff retry), as the SQLite epic does.

## Open Decision Queue

Last ID: (1347)

(1343) [open] [policy] How long audit data is kept.
- Context: (597) (2026-11-14) tied the audit data to the session directory, so the existing prune removed it 7 days after the session was last seen; the byte cap (Decision 3) bounds size independently. With SQLite the store no longer sits inside the session directory by default (see (1345)).
- Alternative a: 7 days after the session was last seen, as (597) approved.
- Alternative b: a fixed age per event (for example 30 days), independent of session activity.
- Alternative c: no time limit; only the byte cap and an explicit session delete remove data.
- Alternative d: a config knob, with one of a-c as the default.
> Recommend d with a as the default: it keeps the approved behaviour and the existing prune semantics, and lets an owner who audits older work lengthen it without a code change.

(1344) [open] [policy] How secrets are handled beyond best-effort redaction.
- Context: (596) redacts before persist and keeps only rule name and length, but `wsredact` is regex best-effort, so a secret in an unrecognized shape is persisted in plain text in a long-lived store. (596) rejected an unredacted store behind separate key management.
- Alternative a: (596) as is; document the residual risk.
- Alternative b: a, plus never storing bodies of calls that read known secret locations (environment dumps, credential files), recorded as a size-only placeholder.
- Alternative c: a, plus encrypting the audit database at rest with an OS-keychain key (the key-management surface (596) rejected).
- Alternative d: a, plus an owner action to purge one event or one session's audit data.
> Recommend a plus d: purge gives a remedy when a secret slips through without the key management (596) rejected, and b's denylist misses as much as the regex does while hiding legitimate audit content.

(1345) [open] [policy] Database layout: one SQLite file per session, or one shared audit database.
- Context: every approved read route is per session (`/s/<id>/audit/...`, (599)). The SQLite epic chose one file per board for the whiteboard ((1325)=a: boards share nothing, export and delete stay trivial), while `wsstore` uses one shared database. The SQLite epic's direct-binary CLI access policy and Windows AV gate are still open there.
- Alternative a: one SQLite file per session, opened lazily and closed when idle, deleted with the session.
- Alternative b: one shared audit database with a session column and indexes.
- Alternative c: an audit table inside a daemon database shared with other SQLite-migrated stores.
> Recommend a: it matches (599)'s per-session routes and the whiteboard precedent, makes retention and delete a file removal, and keeps one runaway session from bloating the others; b and c only pay off for cross-session search, which no approved contract needs.

(1347) [open] [policy] How much of a tool-result body is stored.
- Context: the approved read API serves a body past 8 MiB as a truncated head with a range follow-up ((599)), which assumes the full body is stored. The approved view marks where the model's view ended and shows what lies below it (`seen`), which needs more than the model saw. The per-session byte cap is 256 MiB by default, so one huge output can evict most of a session's history.
- Alternative a: the full body, bounded only by the per-session byte cap.
- Alternative b: a per-body cap (config knob) keeping head and tail with an explicit stored-truncation marker; the range follow-up works within what is stored.
- Alternative c: only what the model saw (`seen` bytes), plus the full size.
- Alternative d: previews only, as the ring does today.
> Recommend b: a generous per-body cap keeps the seen-vs-stored comparison the mockup approved while one runaway output can no longer evict a session's history; c and d drop the comparison.

## Constraints

- Implementation conventions (AGENTS.md): `agents-plugin-tool/` ->
  `ai-docs/manuals/shipped-surface-boundary.md`; `agents-plugin-tool/internal/`
  -> `ai-docs/manuals/release-compatibility.md`.
- SQLite epic obligations: comply with `ai-docs/manuals/performance.md`
  (memory-first interactive paths, measured before/after file ops),
  `ai-docs/manuals/release-compatibility.md` and
  `ai-docs/manuals/shipped-surface-boundary.md`. Do not settle anything
  `261119-epic-ws-daemon-sqlite-storage` leaves open (Windows AV gate,
  direct-binary CLI access policy) on this ticket's own authority. When that
  epic cuts the child this store most directly builds on, add it as
  `blocked-by:`.
- Writing must not block capture. Append off the hot path; on a write error,
  log and drop the event rather than stall the session (carried).
- Store files use mode 0600 and directories 0700, like the ring (carried).
- Release-gate file-touch rule (parent epic): append not rewrite, no
  heartbeat writes, no per-event O(N) rereads.

## Phases

### Phase 1: Store, writer and redaction

After (1344), (1345) and (1347) are answered and the envelope child has
landed: implement the settled database layout with its schema version, the
non-blocking writer behind the envelope child's sink interface, and
redaction with match reporting (Decision 2) applied before every write,
including the settled body handling.

Verification: redaction tests (rule name and length kept; the original
absent from the database file's bytes, WAL included); a newer `user_version`
is refused for writing; file and directory modes; a failing or slow
database drops and logs without delaying the producer; measured file ops
per appended event before/after, per `performance.md`.

### Phase 2: Retention and byte cap

Depends on Phase 1 and (1343). Implement the settled retention rule and the
per-session byte cap (Decision 3) dropping the oldest data first, plus any
purge action (1344) settles.

Verification: the byte cap drops the oldest data first and keeps the newest;
retention removes exactly what the settled rule says; deleting a session
removes its audit data under the settled layout.
