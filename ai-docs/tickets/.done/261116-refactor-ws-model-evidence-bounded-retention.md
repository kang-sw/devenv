---
title: Bound model-evidence retention to first, change points and the most recent N
related:
  261115-feat-ws-dashboard-observed-session-model: introduced the model-evidence directory and its Phase 1 Edition (7ad4a0627) records the current prune
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: dab7c9899d7d02ef
sage-review-design-reviewed: dab7c9899d7d02ef
completed: 2026-11-16
---

# Bound model-evidence retention to first, change points and the most recent N

## Background

### Compatibility surface (stated first)

`model-evidence/` is a shared on-disk store under each dashboard session
directory (`<cache>/dashboard/sessions/<reply-id>/model-evidence/*.json`), so
any retention change is a cross-version surface under AGENTS.md
`### Compatibility Tiers`. VERIFIED against source HEAD `1b9d6fee60606cbac120aa868176f599a1083344`; Go paths abbreviated below are relative to `agents-plugin-tool/`:

- Writers, all through `wslive.AppendObserved`
  (`agents-plugin-tool/internal/wslive/observed.go#L255-L278`), which publishes
  write-once files with first-write-wins identity and prunes after a successful
  new-file write (`agents-plugin-tool/internal/wslive/observed.go#L280-L325`):
  - the MCP live layer: `internal/mcp/live_snapshot.go:594`;
  - hosted Pi live: `internal/mcp/pi_live.go:132`;
  - the agent runner's record writer: `internal/wsagent/record.go:476`.
- Readers:
  - `wslive.ReadObserved` (`observed.go:314`), which tolerates missing files
    and folds records through `SeedObserved` (`agents-plugin-tool/internal/wslive/observed.go#L148-L166`) into
    `summarizeObserved` (`agents-plugin-tool/internal/wslive/observed.go#L190-L214`);
  - `ObservedCache.Load` (`agents-plugin-tool/internal/wslive/observed_cache.go#L99-L107`, used by
    `agents-plugin-tool/internal/wsdashboard/snapshot_cache.go#L132-L144`); it now extends chronological additions incrementally and canonically rebuilds retained records on deletions or late arrivals (`observed_cache.go#L214-L231`, `#L258-L262`);
  - the MCP snapshot seed (`internal/mcp/live_snapshot.go:221`) and runner seed (`agents-plugin-tool/internal/wsagent/record.go#L359-L364`);
  - the session fingerprint in `internal/wsdashboard/agent_sessions.go:224`,
    which lists names only.
- `summarizeObserved` yields per agent `Current` (the latest record) and
  `History` (the first record plus every model change). It is unchanged since
  4dc1e7c35, which added the directory (`git log -- agents-plugin-tool/internal/wslive/observed.go`; `git show 4dc1e7c35:agents-plugin-tool/internal/wslive/observed.go`). Evidence-capable binaries from 4dc1e7c35 until 7ad4a0627 do not prune; 7ad4a0627 and this HEAD do (`agents-plugin-tool/internal/wslive/observed_prune.go#L12-L39`).

### What exists today (pre-change source baseline)

The age/threshold behavior below describes the verified tree, not the newly
settled target: MAIN's 18:44:31 disposition removes both age guards and the
threshold, retaining the throttle and accepting the late-arrival limitation.

7ad4a0627 (merged in 34ae81150) added `pruneObserved`
(`internal/wslive/observed_prune.go:51`). The prune runs only once the
directory holds more than `observedPruneLimit = 2256` records
(`observed_prune.go:30`). Per agent it keeps the first record, every model
change and the latest record. It deletes another record only when its `At`
and its file mtime are both more than `observedPruneAge = 1h` older than that
agent's newest record (`observed_prune.go:33`, `:139-145`). It prunes at most
once per directory per process every 10 min (`observed_prune.go:39`).

### Measured growth (2026-11-16, this machine, read-only)

- 8 session evidence directories hold 10,307 files in total. Each file uses
  one ~4 KB block on disk (13.6 MB for 3,403 files).
- The largest, MAIN's lead session, holds 3,430 records over a 13.6 h span,
  from 128 agents: the lead's 679 records plus 2,751 records from 127
  children. Children live a median of 2 min and at most 30 min.
- Under the current rule, **0** of those 3,430 records can be pruned, although
  only 256 are a first, a change point or a latest record:
  - **Children (2,751 records):** every child repeat lies within 1 h of that
    child's own newest record, permanently. A short-lived agent's repeats are
    therefore never pruned.
  - **Lead:** 576 lead repeats have an `At` older than 1 h but a file mtime
    under 1 h old, so the mtime guard holds them.
  - **Burst (INFERRED cause):** 557 files landed in one 10-minute bucket about
    35 min before measurement. A replay re-adding records that an earlier
    prune deleted would match this, the churn case 7ad4a0627's tests cover.
- Net effect: the reported directory stays above the limit; the current rule supplies no fixed record-count bound, since all records within each agent's own 1-hour window survive, as do older records with protected mtimes (`agents-plugin-tool/internal/wslive/observed_prune.go#L134-L149`).

Historical measurement figures above are reported evidence, not independently
reproduced by this fact pass: no exact directory identity, raw listing or
measurement script is attached. The replay explanation remains INFERRED;
fresh mtimes alone do not establish the writer or replay cause.

## Decisions

- Retention keeps, per agent, the first record, every model-change point, and
  the most recent N records; everything else is deletable. (Settled in the
  owner discussion 2026-11-15; the five implementation choices below were
  settled by MAIN at 18:35:33 under the owner's autonomy grant, not as new owner answers.)
- **MAIN decision under (1170)=a — ODQ-1:** N is the fixed constant 8; add no config knob.
- **MAIN decision under (1170)=a — ODQ-2:** N is per agent; each agent's first, every change point and latest are always retained.
- **MAIN decision under (1170)=a — ODQ-3:** Keep the append-time trigger and existing 10-minute per-directory per-process throttle; drop the 2256-directory threshold.
- **MAIN decision under (1170)=a — ODQ-4 / R2, settled 18:44:31:** Drop the 1-hour At-and-mtime guard entirely; there is no At-only fallback. Keep N=8 per agent plus first, all change points and latest, the 10-minute throttle, and no 2256 threshold. The count bound applies immediately after an eligible completed prune pass, not between passes or at session end; throttling permits interpass growth.
- **MAIN decision under (1170)=a — ODQ-5 / R1, settled 18:44:31:** T0 is the disposition: cross-version readers fold the same retained files identically. No-re-dating is not absolute. Preserve Current/History across a static prune, in-order appends, and replays of pruned same-model records whose model matches their neighbours (the actual churn case). A different-model record arriving late with At inside an already-pruned span may shift the later return transition to the first retained record; original evidence is incomplete and history unreliable there. This is accepted same-version residual behavior, not a cross-version reader difference or a claim that deleted evidence is recoverable. Require a prune-site comment and a test pinning the Y/X/Y counterexample's expected drift.
- **MAIN decision under (1170)=a — settlement trace:** The 18:44:31 R1/R2 dispositions supersede the earlier conditional proof/At-only fallback and unconditional count/no-re-dating wording. R1 and R2 are local policy-gap labels, not owner board IDs or new owner answers. The earlier 7ad4a0627/34ae81150 guard mitigation is retired by this MAIN disposition.
- The ticket states its compatibility tier before implementation, citing the
  writer/reader lines above for which version writes and reads what, and
  whether reopening the new version recovers. (AGENTS.md
  `### Compatibility Tiers`.)

## Source Proof and Compatibility Scenarios

**VERIFIED:** `SeedObserved` deduplicates by agent/source/ID, then
`summarizeObserved` sorts by At and identity and emits transitions at the
first surviving record of a new model (`agents-plugin-tool/internal/wslive/observed.go#L109-L109`, `#L148-L166`, `#L190-L214`).
Keeping first + changes + latest preserves a fixed input's Current/History.
It does not preserve the full input needed to interpret arbitrary late arrivals.
`ObservedCache` rebuilds from retained records after deletion or late arrival,
not from a durable unpruned history (`agents-plugin-tool/internal/wslive/observed_cache.go#L214-L231`, `#L258-L262`).

**INFERRED counterexample (source/math, not an executed test):** for one agent,
use valid distinct IDs and one supported source, with Y at strictly increasing
t1 through t12. First + recent 8 + latest retains t1 and t5..t12, deleting
t2..t4. A later-arriving X at tx with t1 < tx < t2 yields unpruned history
Y@t1, X@tx, Y@t2; retained history yields Y@t1, X@tx, Y@t5. Current remains
Y@t12 but the return transition is re-dated. Any fixed N only moves this
horizon. This extends the earlier Y@t1/Y@t3/Y@t4 then X@t2 counterexample
in `261115-refactor-ws-dashboard-send-latency-followups`, Follow-up Note.
In-order replay of already-known records is not this counterexample.

**R1 accepted residual, not universal equivalence:** a static prune preserves
the current fold. Subsequent in-order appends and replayed repeats matching
their neighbours preserve that equivalence; inserting a new different-model
record inside a discarded span does not. Pin the example above with expected
retained history Y@t1, X@tx, Y@t5 versus unpruned Y@t1, X@tx, Y@t2, and
unchanged Current Y@t12. Add the same limitation at the prune site, explicitly
marking incomplete original evidence/history reliability rather than treating
the drift as an accidental failure or claiming an impossible proof.

**R2 count scope:** removing both age guards resolves the short-lived-agent
conflict. First + all changes + recent 8 (including latest) bounds the
eligible valid deduplicated keep-set per agent immediately after a completed
eligible pass. Protected unreadable/invalid/identity-losing files are outside
that count; failed deletes remain best-effort and cannot promise a physical
file-count bound (`agents-plugin-tool/internal/wslive/observed_prune.go#L46-L50`,
`#L80-L84`, `#L104-L128`). Tests asserting the bound must use valid unique
records, successful deletes and no concurrent insertion. Separate failure and
concurrent-read tests retain their equivalence/protection assertions. The
throttle allows interpass growth; session end alone does not trigger a prune
(`observed.go#L265-L270`, `observed_prune.go#L70-L82`).

| Direction / scenario | Source-backed result and compatibility boundary |
|---|---|
| Existing writer → proposed new reader | Existing MCP, hosted Pi and runner writers use AppendObserved (writer sites above), which marshals the unchanged record and publishes via hard link (`agents-plugin-tool/internal/wslive/observed.go#L280-L325`). ReadObserved tolerates missing files/unknown members and seeds retained valid records (`observed.go#L312-L357`). No format conversion is needed; static keep-set equivalence supports the T0 target. |
| Proposed pruner → old evidence-capable reader | 4dc1e7c35 uses the same summarizer (`git show 4dc1e7c35:agents-plugin-tool/internal/wslive/observed.go`, lines 190–215) and tolerant ReadObserved (lines 300–334); it does not prune. 7ad4a0627/current readers also read retained records. Both display the same fixed-input Current/History after a valid keep-set prune; no claim covers binaries predating the evidence directory. |
| Intervening old writer → new reopen | AppendObserved skips present identities but can recreate a deleted record on replay (`agents-plugin-tool/internal/wslive/observed.go#L280-L288`, `#L308-L325`). Reopening reads only retained/replayed bytes; SeedObserved cannot reconstruct a deleted repeat's original return-transition time after an arbitrary late X (`observed.go#L148-L166`, `#L190-L214`). Deleted evidence is not automatically recoverable; T2 recovery is not established. MAIN's T0 disposition rests on old/new readers folding the same retained files identically, not equivalence to an unpruned alternate future. The accepted late-insertion residual applies within either reader version. |
| Concurrent writer / cache reader | Pruning has only a process-local TryLock and ignores failed removes (`agents-plugin-tool/internal/wslive/observed_prune.go#L42-L56`, `#L80-L84`); cross-process late arrivals remain possible. Cached loads rebuild retained evidence on deletion/late arrivals (`observed_cache.go#L214-L231`, `#L258-L262`), so caching does not repair the counterexample. |

**R1/R2 settled:** MAIN's 18:44:31 disposition accepts the documented residual,
removes both age guards with no fallback, and limits the count acceptance to
a completed eligible pass. No policy decision remains open here; implementation
must demonstrate this bounded contract, not universal late-arrival safety.

**Measurement technical resolution, AUTO-CONFIRMED by root from the fact-populator
measurement gap and approved by MAIN:** read-only enumerate the largest local model-metadata
directory, copy its metadata into an isolated private fixture while preserving
needed provenance, run the candidate only on that private copy, and compare
counts and Current/History. This replaces keep-set projection because an
executable private-copy result proves actual deletion/equivalence without
live mutation. Never prune or mutate the source directory. This is a settled
technical verification method, not an open policy item or an executed check.
No source or runtime changes, tests, live pruning or measurement were performed
by this fact pass.

## Constraints

- Every reader must keep seeing identical `Current` and `History` for every
  agent across a static prune, in-order appends and replayed pruned same-model
  records matching their neighbours. Different-model late insertion inside a
  pruned span has the explicitly accepted R1 residual; do not compare that
  future against an unpruned history as a universal equality requirement. The
  7ad4a0627 equivalence assertions (one agent, three agents, child replay then a
  second prune, concurrent reads, failed deletes) remain the baseline and
  must not be weakened (`agents-plugin-tool/internal/wslive/observed_prune_test.go#L90-L157`, `#L182-L215`, `#L257-L294`). Their age-specific counts and recent-file expectations are old-policy checks, not equivalence assertions; MAIN explicitly changed age/threshold policy (`observed_prune_test.go#L109-L123`, `#L159-L180`, `#L217-L255`). Keep the equivalence baseline unchanged and green; do not misreport obsolete count checks as unchanged-green coverage.
- Deletes stay limited to regular `.json` files directly inside
  `model-evidence/`; unreadable, invalid or identity-losing files are never
  deleted (`observed_prune.go:46-50`).
- Manuals: `ai-docs/manuals/release-compatibility.md` (cross-version store
  shape) applies to `agents-plugin-tool/internal/`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- MAIN acceptance: Phase 1 must prove short-lived-agent and replay-burst eligible valid-record keep-sets have at most first + changes + 8 per agent immediately after a completed eligible prune pass, not continuously or at session end. Preserve protected files and best-effort failed-delete behavior; count fixtures use successful deletes without concurrent insertion. Keep the existing 7ad4a0627 equivalence assertions unchanged and green, adapting explicitly changed age/threshold/count policy checks separately.
- MAIN acceptance: document R1 at the prune site and add a test asserting the known Y/X/Y late-arrival drift and unchanged latest Current, separately from static prune and same-model churn equivalence. No At-only guard or fallback remains.
- MAIN acceptance: run wslive race coverage; elsewhere use focused `-run` filters only. Go verification uses `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOENV=off` and isolated HOME/config/cache/runtime/temp roots. Full wslive race coverage is authorized; no full dashboard, MCP or project suite, shared-runtime operation, provider/network/credential access, signals, stash or push.
- MAIN acceptance / measurement technical resolution: read-only enumerate the largest local model-metadata directory, copy metadata into an isolated private fixture preserving needed provenance, run the candidate only on the private copy, and compare before/after counts and Current/History. Never prune or mutate source/live evidence directories. Use executable private-copy evidence rather than keep-set projection to prove actual deletion/equivalence; no measurement has run in this fact pass.

## Prior Decisions

- 261115-refactor-ws-dashboard-send-latency-followups (2026-11-16, Result (632db1e7a)): "Deletion, replacement, earlier lexical duplicate winners and late arrivals deliberately use the canonical retained-record rebuild." — bearing: constrains
- 7ad4a062 (2026-11-15, commit): "Mitigation: the 1 h At-and-mtime guard." — bearing: constrains; historical mitigation retired by MAIN decision under (1170)=a at 18:44:31, with R1 residual explicitly accepted.
- 34ae8115 (2026-11-15, commit): "Owner decision (1133)=a: retention pre-ship with a 1h guard. No real files were deleted before the merge." — bearing: constrains; guard superseded by MAIN decision under (1170)=a at 18:44:31, not a new owner answer.
- 261115-feat-ws-dashboard-observed-session-model / 4dc1e7c3 (2026-11-15, commit): "Immutable metadata sidecars survive old typed snapshots, with original producer/time deduplication." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wslive/observed_prune.go and observed_prune_test.go; reader/writer consumers audited above |
| scope.surface | internal | Internal retention helpers; shared on-disk model-evidence store, no API shape change authorized |
| scope.new_public_symbol | no | Fixed internal N=8; no new exported symbol or config knob specified |
| scope.new_type_contract | no | Existing ObservedRecord, Current and History contracts retained |
| scope.test_surface | existing | agents-plugin-tool/internal/wslive/observed_prune_test.go and observed_cache_test.go |
| complexity.reuse_points | confirmed | observedRepeats, AppendObserved, SeedObserved, summarizeObserved and ObservedCache retained-record rebuild read |
| complexity.side_effect_risk | high | Irreversible shared-file deletion races writers and replay; no durable recovery for discarded repeats proven |
| risk.correctness | high | Finite recent-eight keep-set cannot preserve transition dates for arbitrary late insertions |
| risk.fit | moderate | Settled R1/R2 require no age guard, post-pass eligible-record bound and retained throttle; protected files and failed deletes stay outside the physical count guarantee |
| risk.test | high | Static/churn equivalence, post-pass count and expected late-insertion drift need separate assertions; old age/threshold/count checks change |
| risk.security_or_contract | moderate | MAIN T0 disposition uses identical cross-version folds of retained files; same-version late-arrival residual is accepted and deleted bytes are not claimed recoverable |

## Phases

### Phase 1: Retain first, change points and the most recent N

Implement N=8 per agent plus first, every change point and latest in
`agents-plugin-tool/internal/wslive/observed_prune.go`, applying MAIN's
18:44:31 R1/R2 settlement: remove both age guards and the 2256 threshold,
retain append-time pruning and the 10-minute per-directory per-process
throttle, and preserve protected-file and failed-delete behavior.

Verification:
- Short-lived-agent repeats within minutes and replay bursts with fresh mtimes
  on old At must meet at most first + changes + 8 eligible valid records per
  agent immediately after a completed eligible pass with successful deletes
  and no concurrent insertion. Assert interpass throttle behavior separately;
  do not promise the bound between passes or merely at session end.
- Preserve the 7ad4a0627 SeedObserved/overlay equivalence assertions unchanged
  and green for static pruning and same-model churn. Adapt old-policy age,
  threshold and count checks explicitly; do not describe those checks as
  unchanged-green coverage.
- Add the prune-site residual-risk comment and a Y/X/Y test pinning the
  counterexample's expected return-transition drift with unchanged latest
  Current. Distinguish this post-late-insertion comparison from static prune
  equivalence; original evidence is incomplete and history unreliable there.
- Run full wslive race coverage; elsewhere use focused -run checks only with
  the offline isolated environment in Constraints. No verification runs are
  claimed by this ticket-population pass.
- Read-only enumerate the largest local evidence directory and copy metadata
  with needed provenance into an isolated private fixture. Run the candidate
  only on the private copy; compare before/after counts and Current/History.
  Never mutate or prune the source/live directory. Historical Background
  measurements remain unverified until this fresh baseline is recorded.

### Result (7bee94167) - 2026-11-16

VERIFIED: Phase 1 landed in `7bee94167`, `745f81c06` and `fb5739515`.
Per-agent retention is first + every model change + recent eight, including
latest. Both age guards and the directory threshold are removed. New-file
append triggering, the ten-minute process-local throttle and best-effort
removal remain; the bound applies only after an eligible successful pass.
No public API, configuration, record shape or architecture changed.

Decisions remain **MAIN decision under (1170)=a**, not new owner answers.
Technical safety refinements preserve static reader equivalence: retain a
duplicate identity's winner as well as its protected losers, and include
readable regular-target symlinks in canonical ordering while never deleting
the links or targets. The accepted R1 Y/X/Y test asserts the retained return
at t5 versus the unpruned t2, with Current still Y@t12. No age fallback was
introduced.

VERIFIED verification (complete commands, environments and outputs under
`ai-docs/tickets/assets/261116-refactor-ws-model-evidence-bounded-retention/`):

- `go test -race ./internal/wslive -count=1 -v` passed on final source;
  `validated-wslive-race.log` is the final full-package evidence.
- Exactly five focused dashboard/MCP/runner observed-store tests passed;
  the explicit `-run` filter and full output are in
  `final-focused-consumers.log`. No full suite outside wslive ran.
- `final-proofs.log` and `verification-proofs.log` prove all six original
  `7ad4a0627` equivalence assertion blocks byte-identical, with hashes and
  original/current line ranges; their final race tests passed. Old
  age/threshold/count checks were updated separately, not called
  unchanged-green. The historical/current summarizer is byte-identical.
- Final read-only enumeration captured 12 directories / 12,450 regular JSON
  files. The largest captured directory, `[redacted session identifier]`, supplied
  **3,848 -> 1,101 files**, **2,747 actual deletions**, on its isolated private
  copy only. All **141 agents** retained equal Current/History; uncached
  overlay and the already-warm cache also matched. Every per-agent bound
  passed. `final-copy-manifest.json` retains full provenance;
  `final-measurement.json` retains per-agent counts/digests, and
  `final-private-copy-prune.log` records executable deletion/equality checks.
- The final unpruned private seed and retained copy match captured bytes,
  modes and mtimes (`final-proofs.log`). The source/live directory was never
  pruned or written by this worker. An earlier 3,888-file capture passed,
  then a later source file disappeared during read-only restoration; that
  failure is retained, not retried or attributed to a writer. A fresh final
  capture replaced it. The ticket's historical figures remain unverified.
- All commands used isolated home/config/cache/runtime/temp roots and
  offline Go. Recorded command-root PIDs were absent after wait; scope and
  final PID checks are in `final-proofs.log`. No unenumerated-descendant
  absence is claimed. Full failing logs and diagnoses remain in `README.md`.

VERIFIED independent review: the route allocated one-pass Lite review. A
fresh xlarge reviewer checked correctness, settled contract/T0 and test
integrity at `07e23055b879747b3015fa81d736b2e6f301c393`; no Critical,
Important or Minor finding remained (`independent-review.md`). T0's
identical-retained-input interpretation is source-backed inference, not a
claim that deleted evidence can be recovered.

omitted: Shared runtime/rebuild operations, CI, integration, push, full
MCP/dashboard/project suites and lease/worktree release were prohibited or
MAIN-owned. No linked visual reference required a comparison. Branch and
private fixtures are retained for MAIN's merge report.

#### Edition (ad205513e) - 2026-11-16

VERIFIED: E3 corrects the public evidence presentation under MAIN's explicit
closed-ticket correction authority. The historical Result's captured session
identifier is minimally redacted; all other historical Result text is preserved.
Its `final-copy-manifest.json` reference is historical: that manifest and the
private archive, scripts, overlay fixture and raw per-agent reports are not
retained in the public evidence. Their current availability is unverified.
The retained aggregate counts, digests and test output remain evidence of the
reported runs, not a replacement per-file provenance receipt.

Local worktree/report paths and environment dumps are omitted from public
summaries and log headers. Removed review inputs are marked not retained,
and the stale held-lease claim is removed from the evidence README. These are
presentation corrections, not fresh private-copy measurements or reruns of
historical consumer checks.

VERIFIED: `ad205513e` adds a unique same-model symlink record before the most
recent eight. Whole `go test -race ./internal/wslive -count=1 -v` passed on
Darwin; removing only `c.regular &&` made the focused regression fail because
the symlink was deleted. Restoring production bytes exactly to the reviewed
base made the focused test pass. Windows skips only unavailable symlink
creation; no native Windows run is claimed. Compatibility remains T0 with no
production behavior, record format or reader/writer changes.
