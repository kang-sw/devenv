# Plan: perf-rules-audit-fixes

Apply the meaning-preserving findings of an independent fresh-read audit to
`ai-docs/manuals/performance.md` (branch `impl/develop/perf-golden-rules`,
base commit 6ca4858b2). Owner (1355)=b 2026-11-18: audit, apply
meaning-preserving findings, return policy-changing ones to the owner.

## Apply (performance.md only)

- F1: intro drops "slower than a developer PC"; first sentence becomes
  "Target machines are corporate Windows PCs with heavy security software
  (antivirus such as AhnLab V3)."
- F2: intro ends at "The goal is the fewest file touches per user action."
  (drop "files exist only for backup and restoration"; Rule 1 carries it).
- F3: Rule 1 drops its Pattern sentence.
- F4: Rule 2 first sentence becomes "One change is one appended line; an
  unchanged state writes nothing."
- F5: Rule 3 drops "Never edit a live file to drop old records."
- F6: Rule 4 body becomes "Open and lock once at startup and hold them until
  shutdown; no per-call open, lock, or `stat`. Why: each is a scanned file
  touch." (no Pattern).
- F7: Rule 5 Pattern becomes "Pattern: update memory, append, return." Keep
  "instead of relying on `fsync`".
- F8: Rule 7 becomes "**Decide by measurement.** Follow `## Measuring`.";
  the Measuring Baseline bullet absorbs the reason: "A clean machine, Linux,
  or fixture CPU time hides the scanner cost and is supporting evidence only."
- Factual fix in Rule 3 Why: per-record files cost a directory listing and
  deletes on every add; in-place pruning rewrites the live file. State both
  costs correctly.

## Do not apply

- F9 (shrink AGENTS.md item 6): owner (1347)=a asked for 3-4 core lines in
  AGENTS.md; keep item 6 as is.
- Rule 6 strengthening and the worked example's graceful-close legacy write:
  policy questions, left to the owner.

## Done when

The edits above are in one commit on the branch, the file still reads as a
coherent manual (rule numbering 1-8 intact, `summary:` frontmatter unchanged),
and nothing outside performance.md changed besides this plan file.
