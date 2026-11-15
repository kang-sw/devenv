# Review 794 UI-C3: reveal a folded topic before target navigation

## Authority and baseline

MAIN assigned UI-C3 to EXEC2 after 649 and authorized static-only implementation on 2026-11-14. On 17:30:40Z MAIN explicitly approved a named internal WsCards topic-unfold export, preferring it over a pre-geometry callback unless it cannot precede history geometry. EXEC1 agreed at 17:30:45Z to a surgical pre-split patch in this isolated branch; MAIN owns serialized integration. The 793 asset split must preserve/map these owner and navigation boundaries, not choose this repair's semantics.

Base: d0ad8a9f1863e15b2bcc2d5f2d22eb062c920c14.
Branch: fix/review794-folded-topic-reveal.
Finding: UI-C3 in /Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/d9392112-11-sweep794-rereview-aggregate.md. MAIN is arranging its serialized findings-ticket capture; do not edit that shared ticket here.

## Required outcome

Search body/question/answer navigation with Overview inactive and valid #pN/item history navigation reveal the owning whole-topic fold before geometry, focus, scroll or keep. Reuse cards.js's existing foldUnit/topicUnit state owner so persistence, decoration and accessibility remain coherent. Preserve unrelated inner disclosures and topic folds, Overview's existing handoff, palette focus-retention behavior and the dashboard ID allowlist. Never bypass ownership by changing only data-size or weakening the CSS mask.

Prefer one small named WsCards owner export reused by the two navigation paths. History loadTarget currently materializes the target, opens DETAILS, reads offsetWidth and performs navigate/keep before returning: unfolding only after it returns is insufficient. MAIN also permits a minimal pre-geometry callback if the named export cannot establish the ordering. Decide from source evidence; record the exact ordering and rationale. Do not introduce another fold-state owner or a broader navigation redesign.

## Bounded files

Production candidates: agents-plugin-tool/internal/wsdashboard/assets/cards.js and dashboard.js, and agents-plugin-tool/internal/wswhiteboard/assets/stream.js only if required for pre-geometry history integration. Focused tests may extend the existing cards_test.go, whiteboard_bridge_test.go, board_frame_lazy_test.go, stream_script_test.go and testdata/search-palette.test.cjs patterns, or a narrowly named same-directory regression fixture if essential. Do not edit CSS/templates, unrelated features, scripts/board-headless.mjs (UI-T5 belongs to EXEC1), tickets or mirrored playbooks. Read every applicable AGENTS.md manual before edits. Stop on materially broader scope.

## Static verification and return boundary

This assignment grants NO executable verification. No tests, builds, syntax/format tools, Node/Python/browser/probes, package operations, daemon operations, helper execution or process signals. Read-only Git/source/document inspection and authorized static edits plus a scoped commit are allowed. Do not check out, merge, push, reset or force branches. Do not spawn execution/review workers that run commands outside this permission. MAIN will grant a separate finite execution slot later.

Author focused regressions for folded-topic search body/question/answer and valid history targets, including deferred/inert materialization where relevant. Assert owner state/persistence and reveal-before-first-geometry ordering, while preserving independent disclosures, Overview/focus behavior and invalid-ID rejection. Static inspection does not prove browser behavior; explicitly mark all authored tests unrun.

Commit one logical repair with this plan and AI Context explaining the internal owner API choice, ordering, source baseline and deferred verification. Return the exact commit, complete changed-file list, source citations, static checks actually performed, authored but unrun regressions, and a small finite proposed verification command set (not executed). State any unresolved gaps or scope boundary. Do not write durable notes, integrate develop, claim runtime acceptance or release the worktree.
