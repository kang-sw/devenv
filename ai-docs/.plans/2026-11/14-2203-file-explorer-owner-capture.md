# File explorer owner-decision capture

## Assignment

Draft one epic and the first todo children for the owner-approved reusable stateful file explorer, using ticket tooling and conventions. Documentation only in this housekeeping checkout. Do not edit product source, run tests/build/browser/network/package operations, promote to ready, invoke fact population or Sage review, merge, or touch the shared runtime. Return exact draft paths and complete static diff for lead review before any commit. A later explicit lead instruction may authorize the one logical documentation commit including this unchanged plan.

## Authority

MAIN mailbox 2026-11-14T12:55:59Z relays owner answers: 742=ok; 741,744,746=a; 745=a with room for future Tauri-shell widening. MAIN 13:01:22 requests epic + first tickets, static/docs only. MAIN owns all decision IDs; do not mint any. No visual reference has been approved or supplied; a designer mockup is in progress, so do not invent a reference or treat it as approved.

Confirmed product scope: reusable stateful explorer card; isolated instances rather than the present singleton; internal tabs including middle-click; details/list/icon/gallery view modes; per-window filename and content search; float/fullscreen; conversation Files tab reuses the component. Read-only operations are open/preview/search/copy path/send path reference to chat, not filesystem modification. Each card stores its state in browser storage (additive T0), not a new daemon/shared persistence store. Code syntax highlighting and PDF preview belong in the first user-facing feature delivery. Existing guarded roots remain the access boundary; leave architectural room for future Tauri expansion without widening roots now.

All owner-approved ideas must remain represented in the epic: agent change-tracking view; Git badges and diff tab; worktree/root switch; drag-to-chat path references; ticket/board links; path bar; tree and column view; space quick-look; bookmarks/recent plus hidden/ignored toggle; split dual pane; keyboard navigation; virtualized large directories. Do not add unapproved budgets, storage fields, endpoints, dependencies, renderer libraries, API names, or root permissions.

## Decomposition boundary

MAIN explicitly names isolation of current singleton #files as the first technical step. Draft the first refactor child to isolate instances while preserving current behavior and security. Draft the first feature child for the approved reusable card's base user-facing delivery including tabs, view modes, per-window search, per-card browser state, conversation reuse, PDF and code highlighting. Record remaining approved epic scope as planned children without inventing implementation order or mechanisms. The epic is a board artifact, no implementation phases. Actionable children have stable Phase 1 sections and no fabricated Results. Preserve accepted outcomes while returning any unresolved mechanism or grouping choice as an unnumbered gap to the lead rather than recording it as settled. All new files stay todo; no ready/Sage/fact work.

## Evidence supplied

Current implementation facts, not future policy: one #files instance shared by conversation and toolbar (dashboard.js:790-797; cards.js:119-126,693-704); browser card state dashboard.js:27-35,730-731. Existing /fs list/preview/raw/find/search and guarded roots are reusable. Preview PDF/code highlighting are gaps, not current features. Security evidence: fs_routes.go:145-203 local and non-cross-site alternate-root reads, validated absolute directories; remote LAN-token reads stay session-root. fs_act_route.go:16-34 and fs_outside.go:408-446 require local viewer + matching Origin for host actions/cap mint. Outside linked-file grants are separate, narrow, expiring capabilities, not blanket file-browser authority. Folder-picker expansion is an unresolved separate proposal, not part of these approved Files roots.

## Verification and stop

Read ticket conventions and templates/checklists. Complete content and intent trace against written drafts. Use static ticket guardrails only; report any missing related stem or uncertainty accurately. Record compatibility per slice: instance refactor and asset-only additive per-card browser behavior T0; any future additive backend endpoint is only a candidate T0 after its semantics are settled, never automatic authorization. No T2, on-disk shared-store change, protocol change, or root-policy change. No implementation, release, real installation, or runtime acceptance claim. Keep unrelated files out. Report drafts, known gaps, and explicit deferred implementation/review/visual-reference gates.
