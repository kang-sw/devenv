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
   key-management surface. What else secret handling needs is settled in
   Decision 7 ((1734)=a): nothing more. Redaction runs on the full body
   before any per-body cap (Decision 9), so a secret straddling the cut cannot
   be split into an unredacted fragment.
3. **Byte cap ((597), carried in part).** A per-session byte cap (config
   knob, default 256 MiB) drops the oldest data first, because one runaway
   tool output could fill the disk within any time window. (597)'s time rule
   is settled in Decision 6 ((1733)=d). Its drop unit (whole segments) is
   superseded by the SQLite layout. The byte cap is a config key like the
   knobs of Decisions 6 and 9 (Constraints, "Config keys").
4. **Compatibility tier T0, with its own schema version.** A new store with no
   older reader. The schema version lives in the database (SQLite
   `user_version`). A daemon refuses to open a newer version for writing, in
   line with `261112-feat-ws-mixed-version-compat-guards`.
5. **SQLite precedent.** Follow the existing `modernc.org/sqlite`
   configuration in `agents-plugin-tool/internal/wsstore/store.go` (WAL,
   `SetMaxOpenConns(1)`, `busy_timeout`, foreign keys, `SQLITE_BUSY`/
   `SQLITE_LOCKED` backoff retry), as the SQLite epic does. Handle lifecycle
   follows Decision 8.
6. **Retention is a config knob, default 7 days after the session was last
   seen ((1733)=d, owner 2026-11-19).** The default is (597)'s approved
   behaviour, so the existing session prune semantics apply unchanged. An
   owner who audits older work lengthens the knob without a code change.
   Retention removes a session's whole audit database file (Decision 8). The
   byte cap (Decision 3) still bounds size independently.
   - Rejected: a, a fixed 7 days with no knob. It gives an owner who audits
     older work no recourse short of a code change.
   - Rejected: b, a fixed age per event (for example 30 days) independent of
     session activity. It changes (597)'s approved semantics and leaves a live
     session's older events pruned out from under it.
   - Rejected: c, no time limit with only the byte cap and an explicit
     session delete removing data. It keeps a long-lived store of
     best-effort-redacted data indefinitely (Decision 7).
   - A knob shorter than the session's own retention can leave a live session
     with no audit data. That is a valid state, not an error (the query API
     child serves it as empty).
7. **Secrets: best-effort redaction as it is, residual risk documented
   ((1734)=a, owner 2026-11-19).** (596) stands unchanged: redact before
   persist, keep rule name and original length only. `wsredact` is regex
   best-effort, so a secret in an unrecognized shape is stored in plain text.
   The feature's user documentation states this residual risk, and that
   retention (Decision 6) and deleting the session are the only ways data
   leaves the store. A shorter retention knob is therefore the owner's
   mitigation. The documentation's location is chosen at implementation.
   - Rejected: b, a plus a size-only placeholder for calls that read known
     secret locations. A denylist misses as much as the regex does while
     hiding legitimate audit content.
   - Rejected: c, a plus encrypting the database at rest with an OS-keychain
     key. It is the key-management surface (596) already rejected.
   - Rejected: d, a plus an owner action to purge one event or one session's
     audit data (the lead recommended a+d; the owner chose a only). No purge
     route, tool or UI control is added anywhere in the audit epic.
8. **One SQLite file per session, deleted with the session ((1735)=a, owner
   2026-11-19).** Every approved read route is per session (`/s/<id>/audit/...`,
   (599)), so each session, lead or `ws/agent.*` child (the envelope child's
   (1731)=b), gets its own audit database file.
   - Handle lifecycle follows the SQLite epic's handle rule, owner
     **(1347)=c**: the handle is held open and closed only after about 30
     minutes idle. It is not "lazy open plus a short idle close". Reopen
     after an idle close is lazy and costs one open. (The SQLite epic's text
     on `develop` e3837e033 still says "opened lazily and closed when idle"
     for the whiteboard and does not yet record (1347)=c. When that epic
     records the rule, its wording governs this one.)
   - Deleting a session deletes its audit file (the file's `-wal`/`-shm`
     included) and retention (Decision 6) does the same. Cross-session search
     is not provided.
   - The byte cap (Decision 3) applies within a session's file. Deleting the
     oldest rows must keep the file's on-disk size bounded, not only the live
     row bytes, so Phase 2 measures the file size after a cap drop.
   - Rejected: b, one shared audit database with a session column. It pays
     off only for cross-session search, which no approved contract needs, and
     lets one runaway session bloat the others.
   - Rejected: c, an audit table inside a daemon database shared with other
     SQLite-migrated stores. Same reason, and it couples audit retention and
     delete to unrelated stores.
   - The SQLite epic's open gates (Windows AV benchmark, direct-binary CLI
     access policy) are not settled here (Constraints).
9. **Tool-result bodies: a per-body cap keeping head and tail ((1737)=b,
   owner 2026-11-19).** A per-body cap (config knob) bounds each stored `in`,
   `out` and `raw`. A body over the cap is stored as its head and tail with
   an explicit stored-truncation marker (the envelope's `stored_trunc`). The
   read API's range follow-up works within what is stored. This keeps the
   seen-versus-stored comparison the mockup approved while one runaway output
   can no longer evict a session's history.
   - Order: redact the full body, then apply the cap (Decision 2). Redaction
     `offset` values refer to the redacted body before the cap. A redaction
     inside the omitted middle stays listed with its rule and length but has
     nothing to render. The exact offset semantics are fixed in Phase 1.
   - The default cap, and the head and tail split, are set at ready promotion
     with measured evidence. A generous default is the intent.
   - Search covers stored text only. The omitted middle is not searchable.
   - Rejected: a, the full body bounded only by the per-session byte cap. One
     huge output can evict most of a session's history.
   - Rejected: c, only what the model saw (`seen` bytes) plus the full size.
     It drops the comparison with the part below the model's cut.
   - Rejected: d, previews only, as the ring does today. It drops the
     comparison and defeats the audit's purpose.

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
- Config keys (Decisions 3, 6, 9): the byte cap, the retention period and the
  per-body cap are new config keys. They are additive, T0
  (`ai-docs/manuals/release-compatibility.md`): an older binary that does not
  know them keeps running on its built-in behaviour. A new key is not a
  manifest field change. The manifest parser refuses unknown fields and
  `default_scope` values outside `session|project|global` (scenario 11, T3),
  so the keys use only existing fields and scopes. Wherever a key is declared
  in a config manifest, the `ws` and `wsflow` manifests must stay
  key-identical with only `default` differing, including the native-pi
  first-party and dashboard manifest copies
  (`TestShippedClaudeManifestsDifferOnlyInDefaults`;
  `ai-docs/manuals/wsflow-mirroring.md`). Key names are chosen at
  implementation under the existing config registry naming.
- Writing must not block capture. Append off the hot path; on a write error,
  log and drop the event rather than stall the session (carried).
- Store files use mode 0600 and directories 0700, like the ring (carried).
- Release-gate file-touch rule (parent epic): append not rewrite, no
  heartbeat writes, no per-event O(N) rereads.

## Phases

### Phase 1: Store, writer and redaction

After the envelope child has landed: implement the per-session database
layout (Decision 8) with its schema version and the held-open handle
lifecycle, the non-blocking writer behind the envelope child's sink
interface, redaction with match reporting (Decision 2) applied before every
write, and the per-body cap with the stored-truncation marker (Decision 9)
applied after it. Register the per-body cap, retention and byte-cap config
keys (Constraints, "Config keys").

Verification: redaction tests (rule name and length kept; the original
absent from the database file's bytes, WAL included); a newer `user_version`
is refused for writing; a secret straddling the per-body cut boundary is
redacted, not split; a body over the cap is stored as head plus tail with the
marker and an under-cap body is stored whole; one file per session, with a
handle that stays open across a short idle and closes only after the long
idle (a test clock, not a 30-minute wait); file and directory modes; a
failing or slow database drops and logs without delaying the producer; measured file ops
per appended event before/after, per `performance.md`.

### Phase 2: Retention and byte cap

Depends on Phase 1. Implement the retention knob (default 7 days after the
session was last seen, Decision 6) and the per-session byte cap (Decision 3)
dropping the oldest data first. There is no purge action (Decision 7).

Verification: the byte cap drops the oldest data first and keeps the newest;
retention removes exactly the sessions past the configured age and no
others, and a lengthened knob keeps them; the file's on-disk size stays
bounded after a cap drop; deleting a session removes its audit file and its
`-wal`/`-shm`; the new config keys resolve, and the `ws` and `wsflow`
manifests stay key-identical with only `default` differing.
