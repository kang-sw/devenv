# Review 794 D-C1: safe whiteboard takeover guidance

## Authority
MAIN 2026-11-14 14:49:15 assigns this existing Important finding for STATIC repair and commit only, after H/Tauri preparation. H docs checkpoint is paused and Tauri source is ready awaiting its heavy slot; this is independent static follow-through while waiting. No new scouting/spec/ticket authority or decision IDs.

## Evidence and scope
Root fully read /Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/ae2737c2-01-sweep794-distribution-correctness.md, D-C1. The existing copy-then-update recipe carries source EmbedDir and Embeds through serialized meta.state, so regeneration can fail if the source host directory vanished, write into the source host if live, or evict source-named shared historical snapshots. Authoritative topic/item history is not the historical-embed data at risk.

Edit only the takeover recipe and required mirrored resource manifests in:
- agents-plugin/rsrc/whiteboard/whiteboard.md
- agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md
- agents-plugin-pi/rsrc/whiteboard/whiteboard.md
- the corresponding existing resource hash manifests, located from repository evidence
A focused existing/new textual regression test may be authored ONLY if it fits the package's established resource test location and requires no execution now. Do not alter product Go storage/protocol/API, CLI schema, snapshots or live boards. No generic migration feature, new tool, or new exported contract.

## Required outcome
Make the shipped guidance safe: after a quiescent source copy into an independently backed-up destination, explicitly prepare ONLY the copied destination's render bookkeeping before first regeneration. Rebind or clear the inherited host embed directory and safely discard source-derived retention bookkeeping while preserving all authoritative topic/entry/item/counter state and leaving source metadata, source host output and shared historical snapshot files untouched. Do not use destructive traversal or a generic overwrite/cleanup operation. Verify exact on-disk keys and commit/recovery behavior in source before prescribing edits; if a safe bounded guidance-only recipe is impossible, stop and report that scope boundary instead of changing Go or inventing an API. A conservative explicit restriction is preferable to an unsafe unconditional promise, as the finding permits.

Read matching shipped-surface-boundary, skill-authoring and wsflow-mirroring manuals IN FULL before editing. Pi parity is required. T0 resource content/hash refresh only, no manifest field/protocol/store shape change. Keep shared guidance host-neutral and downstream-first. Do not encode this repository's owner/MAIN/session names or machine paths into shipped text.

## Verification and return
STATIC ONLY: source/doc/Git inspection and exact content/hash calculation are allowed. No tests, build, syntax execution, browser, live whiteboard operations, fixtures, native app, network, daemon commands or children. Author tests if appropriate but label them NOT RUN. Use edit for targeted existing-file changes; preserve immutable plan. Commit one logical unit with AI Context and explicit paths on fix/review794-whiteboard-takeover. No merge/rebase/push/reset/release; no unrelated ticket edits. Return full commit, complete patch summary, evidence pointers, deferred focused verification commands and any gaps. Root will inspect and mail MAIN; no implied acceptance or execution slot.
