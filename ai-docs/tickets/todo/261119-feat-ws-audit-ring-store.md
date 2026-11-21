---
title: "감사 세그먼트 링 저장소: 저장 전 마스킹과 보존 정책"
parent: 261119-epic-ws-conversation-audit
blocked-by: 261119-feat-ws-audit-policy-event-envelope
related:
  261119-feat-ws-audit-capture-harness-adapters: "producer: writes envelopes into this store; may proceed in parallel"
  261119-feat-ws-audit-query-api: "consumer: reads this store"
  261112-feat-ws-mixed-version-compat-guards: "precedent for refusing writes to newer store formats"
  261114-feat-ws-conversation-audit-event-store: "superseded source of (594) rationale, (596), (597) and the file-mode/non-blocking constraints"
---

# Audit segment ring, redaction and retention

## Background

Third child of `261119-epic-ws-conversation-audit` ((1331)=a, 2026-11-19).
It persists the envelopes child 2 captures, redacts them before they touch
disk, and bounds how long and how much is kept.

Owner decision (1960)=a (2026-11-21) supersedes the SQLite choice in
(1331)=a. `ai-docs/manuals/performance.md` `## Storage Types` classifies
audit events as Type 1 append-only logs. The superseded ticket's (594)
supplies the rationale for keeping a dedicated audit store beside the
display ring.

Verified findings (codex.large read-only research, 2026-11-19):

- The display ring is intentionally lossy and unusable as an audit store.
- Redaction is regex best-effort: `wsredact` documents that a secret in an
  unrecognized shape passes through (`agents-plugin-tool/internal/wsredact/redact.go:1-9`).

## Decisions

1. **A dedicated Type 1 audit ring ((594) rationale, (1960)=a).** The
   display ring is a capped UI view whose trimming would discard audit data.
   Full tool outputs would bloat every history page read. The audit writer
   appends envelopes to the newest segment of a separate per-session ring.
   An in-memory index supports reads after recovery from the segments.
   - Rejected: the SQLite choice in (1331)=a. Under (1960)=a, audit events
     are append-only replay and range data. Text search scans the ring index.
     SQLite is for storage types 2-4, including structured history queried
     by field over the whole store.
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
3. **Byte caps ((597), (1970)=a).** A per-session byte cap (config knob,
   default 256 MiB) drops the oldest whole segment first, because one runaway
   tool output could fill the disk within any time window. (597)'s time rule
   is settled in Decision 6 ((1733)=d). The per-store total byte cap is a
   separate config knob. It evicts whole sessions' audit rings in
   least-recently-used order. Its default is open in (2010). Segment size is
   an implementation choice.
4. **Compatibility tier T0, with its own schema version.** A new store with no
   older reader. The on-disk ring format carries its schema version. A daemon
   refuses to open a newer version for writing, in line with
   `261112-feat-ws-mixed-version-compat-guards`.
5. **Memory-first ring discipline ((1960)=a).** Follow
   `ai-docs/manuals/performance.md`. Recover the index from retained segments,
   keep the current append handle open across writes, and serve reads from
   memory. Rotation opens the next segment. Retention removes whole segments
   or a whole session's audit ring, according to Decisions 3, 6 and 8.
6. **Retention is a config knob, default 7 days after the session was last
   seen ((1733)=d, owner 2026-11-19).** The default is (597)'s approved
   behaviour, so the existing session prune semantics apply unchanged. An
   owner who audits older work lengthens the knob without a code change.
   Time retention ((1970)=a) removes a session's whole audit ring
   (Decision 8). Both byte caps apply independently (Decision 3).
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
   The feature's user documentation states this residual risk and explains
   how the byte caps, time retention and session deletion remove data. A
   shorter retention knob is therefore the owner's mitigation. The
   documentation's location is chosen at implementation.
   - Rejected: b, a plus a size-only placeholder for calls that read known
     secret locations. A denylist misses as much as the regex does while
     hiding legitimate audit content.
   - Rejected: c, a plus encrypting audit data at rest with an OS-keychain
     key. It is the key-management surface (596) already rejected.
   - Rejected: d, a plus an owner action to purge one event or one session's
     audit data (the lead recommended a+d; the owner chose a only). No purge
     route, tool or UI control is added anywhere in the audit epic.
8. **One audit ring per session, deleted with the session ((1735)=a,
   (1960)=a).** Every approved read route is per session (`/s/<id>/audit/...`,
   (599)), so each lead and `ws/agent.*` child ((1731)=b) gets its own
   segment ring and index. Session deletion, time retention and store-wide
   eviction remove all its audit segments and index entries. Store-wide
   eviction removes audit data only. Reads and search remain per session.
   - Rejected: b, one shared audit database with a session column. It pays
     off only for cross-session search, which no approved contract needs, and
     lets one runaway session bloat the others.
   - Rejected: c, an audit table inside a daemon database shared with other
     SQLite-migrated stores. It couples audit retention and deletion to
     unrelated stores.
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
   - Rejected: d, previews only, as the display ring does today. It drops the
     comparison and defeats the audit's purpose.

## Constraints

- Implementation conventions (AGENTS.md): `agents-plugin-tool/` ->
  `ai-docs/manuals/shipped-surface-boundary.md`; `agents-plugin-tool/internal/`
  -> `ai-docs/manuals/release-compatibility.md`.
- Follow `ai-docs/manuals/performance.md` for Type 1 storage, memory-first
  interactive paths and measured before/after file ops.
- Config keys (Decisions 3, 6, 9): the per-session byte cap, per-store total
  byte cap, retention period and per-body cap are new config keys. They are
  additive, T0 (`ai-docs/manuals/release-compatibility.md`). An older binary
  that does not know them keeps running on its built-in behaviour. A new key is not a
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
- Store files use mode 0600 and directories 0700, like the display ring (carried).
- Release-gate file-touch rule (parent epic): append not rewrite, no
  heartbeat writes, no per-event O(N) rereads.

## Open Decision Queue

(2010) [confirmed] [policy] Owner answer 2026-11-21: a (4 GiB). What is the default total byte cap across the audit
store's session rings ((1970)=a)?

- a. 4 GiB, equivalent to 16 sessions at the default 256 MiB per-session cap.
- b. 1 GiB, equivalent to 4 sessions at the default per-session cap.
- c. 8 GiB, equivalent to 32 sessions at the default per-session cap.

> recommendation: a. It keeps more recent sessions than the 1 GiB option
> with half the maximum disk use of the 8 GiB option. Owners can tune the cap
> to local disk capacity and audit volume.

## Phases

### Phase 1: Store, writer and redaction

After the envelope child lands, implement the per-session segment ring
(Decision 8) with its schema version, recovery, in-memory index and held-open
append handle. Implement the non-blocking writer behind the envelope child's
sink interface, redaction with match reporting (Decision 2) applied before every
write, and the per-body cap with the stored-truncation marker (Decision 9)
applied after it. Register the per-body cap, retention and both byte-cap config
keys (Constraints, "Config keys").

Verification: redaction tests (rule name and length kept, original absent
from persisted segments), including a secret straddling the per-body cut.
Refuse writes to a newer ring format, recover retained events into the
in-memory index, and verify independent session rings, append-handle reuse,
and file and directory modes. Verify that an oversized body keeps its head,
tail and marker and an under-cap body stays whole. A failing or slow writer
must drop and log without delaying the producer. Measure file ops per appended
event before/after per `performance.md`.

### Phase 2: Retention and byte caps

Depends on Phase 1. Implement the retention knob (default 7 days after the
session was last seen, Decision 6) and the per-session byte cap (Decision 3)
dropping the oldest whole segment first. Implement least-recently-used
whole-session audit eviction under the per-store total cap (Decision 3).
Resolve the total cap default in (2010). Session deletion removes the
session's ring (Decision 8). There is no purge action (Decision 7).

Verification: the per-session cap drops the oldest whole segments and keeps
the newest. The per-store cap removes whole audit rings in least-recently-used
order. Measure on-disk bytes after both kinds of eviction and check that reads
see only retained events. Expire audit rings for sessions past the configured
idle age and verify that lengthening the period keeps them. Deleting a session
removes its whole audit ring. Verify the config keys resolve and the `ws` and `wsflow`
manifests stay key-identical with only `default` differing.
