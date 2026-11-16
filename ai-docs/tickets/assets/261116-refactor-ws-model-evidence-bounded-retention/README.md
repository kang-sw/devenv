# Bounded retention: public evidence summary

## Scope and preserved anchors

This is MAIN-authorized, evidence-only cleanup of delivery
`01d14188534b2e26fe49840f162b61c3eec96b86`. Production source, tests and contract
are unchanged by the evidence cleanup. Result anchor `7bee94167` and final
source anchor `fb5739515` remain the implementation references.

The root lead consolidated the four unpushed docs/evidence commits into
`cae8e7ea530be240295b378b20a9c8d66e797fe2`, retaining the sanitized tree
exactly and preserving source/preparation history through `fb5739515`.
Every commit in the resulting implementation range is manifest-free; no
recovery ref was created. The private old/new mapping is not retained in this
public evidence. Source/tests remained byte-identical to the original delivery.
No Go rerun, runtime action, integration or push occurred in that cleanup.

The later E3 tidy-up removes local paths and environment dumps from these
public summaries and log headers. The closed ticket records an appended
correction Edition and a minimal captured-session-name redaction; its other
historical Result text is preserved.

## Private archive and deterministic aggregates

The historical cleanup reported a private archive of 29 original public
artifacts and 15 byte-original logs, an archive index of sizes/SHA-256 values,
and independently written, synced and verified copies (not hard links).
That archive, index, private mapping, manifests and raw per-agent reports
are **not retained in this public evidence**; their current availability is
unverified. The aggregates below survive without per-file provenance.

| Capture | Files | Total record bytes | Sorted-pair SHA-256 |
|---|---:|---:|---|
| Initial | 3,888 | 536,284 | `e6393fc39e1e17dcf8fcfbbf35d178a9648f21077fd02f316f793d71d054524b` |
| Final | 3,848 | 532,428 | `23e310d0c65b65168e80f3332c15083810edec9937aa78b41acdb53a1ff74b98` |

Aggregate encoding: sort `(name, lowercase SHA-256)` tuples lexicographically.
For each tuple append UTF-8 filename bytes, one NUL byte, 64 ASCII digest
bytes, and one LF byte. SHA-256 the concatenation. Captured names are unique
ASCII filenames; total bytes is the sum of record sizes, not disk allocation.
Per-file names, source inode/device/mtime detail and per-agent runtime IDs
remain private, not public rows.

## Retained verification

These are historical commands/results from the reviewed implementation,
not new runs or a claim that its former public manifest paths still exist:

- `validated-wslive-race.log`: full wslive `-race` command/output.
- `final-focused-consumers.log`: five-test consumer filter/output.
- `final-private-copy-prune.log`: private-copy command with local input/output
  paths redacted and full test output;
  **3,848 -> 1,101 files**, **2,747 actual deletions**, all **141 agents'**
  Current/History, overlay and warm cache equal. Every eligible bound passed.
- `final-private-copy.log`: aggregate capture summary, 12 directories and
  12,450 files enumerated; stable individual reads, not an atomic live snapshot.
- `verification-proofs.log`: six byte-identical original `7ad4a0627`
  equivalence assertion blocks and historical summarizer identity.
- `final-proofs.log`: retained-byte/source digests and scoped PID summary.
- `measurement.json` and `final-measurement.json`: aggregate results and
  hashes of private reports, without per-agent rows or retained raw reports.

`independent-review.md` retains its historical clean one-pass Lite verdict
for correctness/contract/T0/test integrity, not a review of either cleanup.
Its private report path is omitted; references to removed inputs are marked
not retained. Separate Fit/convention review was outside that partition.
File:line references describe the original evidence before header redaction.

The closed Result's removed artifact names are historical locators only.
Manifests, scripts, the overlay fixture, superseded logs, byte-original logs
and the final unpruned private seed are not retained here. The historical
cleanup reported leaving the seed untouched; no current availability or
reproducible private-copy recipe is claimed.

## Preserved failure diagnoses and limits

Raw failure logs are not retained here. Historical diagnoses:

1. Broad cache enumeration exceeded 10 seconds; enumeration narrowed to the
   known directory schema without a timeout increase.
2. The initial private-copy fixture rejected macOS's `/tmp` canonical spelling
   before pruning; its private-root check was corrected.
3. A new protected-file test still treated a symlink alias as a unique repeat;
   only that duplicate-specific membership expectation changed. Original
   equivalence assertions remained unchanged and passed the final race run.
4. A captured original later disappeared during read-only restoration. Its
   writer/cause remains unattributed; no investigation or restoration retry
   occurred. A fresh final capture supplied the retained measurement.

The ticket's historical measurements remain unverified. T0 means identical
folds of retained input, not recovery of deleted evidence or universal
late-arrival equivalence. Original/live stores were not accessed during
cleanup. Integration, shared runtime, push and CI were lead-owned.
