---
title: "dashboard 세션 페이지: 데이터 전용 서버와 클라이언트 측 조립"
related:
  261116-feat-ws-dashboard-session-page-payload-and-render-cost: predecessor; Phase 1 landed, Phase 2 (lazy history) in progress
  261113-feat-ws-dashboard-frontend-performance-profile-and-optimize: measurement scenarios and harness to reuse
  261115-feat-ws-dashboard-board-topic-transport: stream topic-delta transport any new payload must stay compatible with
  261116-feat-ws-whiteboard-retire-disk-pages: standalone/disk page rendering that a client-assembly design affects
---

# Dashboard session page: data-only server, client-side assembly

## Background

Owner decision (whiteboard (1294)=c, 2026-11-17): run Phase 2 (lazy history) of
`261116-feat-ws-dashboard-session-page-payload-and-render-cost` now, and capture
the larger "server sends data, client assembles" direction as this research
ticket. Discuss it only after the daemon file-I/O round 2 work lands.

Owner, 2026-11-17 12:44, verbatim:

> 다만 이건 백신하곤 별개라고 보는 게 이건 집에서 macos PC, 즉 백신 없는 PC로 카드 200개짜리 무거운 세션 받아올 때 있던 이슈였어서요. 그래서 서버가 데이터만 넘기고 CSR로 전환하자는 맥락도 여기서 나오게 되었습니다.

(Translation: this is separate from the antivirus. It happened at home on a
macOS PC with no antivirus, loading a heavy session of about 200 cards. That is
where the idea of the server sending only data and switching to CSR came
from.)

So the first-load cost of a heavy session exists independently of today's
Windows file-syscall amplification on the owner's office PC.

## Current state

- Phase 1 of the predecessor (4e3746a0e) landed:
  - out-of-line immutable CSS/JS/i18n;
  - a single board send instead of page plus stream;
  - a cheap memo key.
  - On the generated fixture, the history-heavy shape `8,6,4,2,60` fell from 2,944,449 to 985,210 B of framed HTML. The 32-topic shape fell from 1,991,993 to 252,920 B.
- Phase 2 (lazy history: closed topics past the cap, older panel versions, timeline gaps on demand; images by URL) is in progress on `impl/develop/render-cost-phase2`.
- Visible-only rendering of board topics (5e5c7835f, content-visibility after first layout) landed. The owner confirmed on 2026-11-17 at 12:47 that it dramatically cut frontend load.
- The predecessor's `## Decisions` holds a direction marked non-authoritative: "client-side markdown rendering is rejected for now; a later phase: server-rendered per-topic fragments cached by topic revision, assembled by the client".
  - That was an agent technical call, not an owner decision.
  - This research may revise it.

## Questions

Compare these on the heavy-session shape (about 200 cards; build a generated fixture of that shape, or reuse the existing performance fixture generator):

- **(A) Full CSR.** The server sends board JSON; the client renders markdown, mermaid, panel diffs and templates.
- **(B) Per-topic fragment cache.** The server renders per-topic HTML fragments cached by topic revision; the client assembles them and receives only changed fragments.
- **(C) Status quo after Phase 2.** Server-side render with lazy history and visible-only rendering.

Measure for each:
- server time (cold and warm);
- bytes for the first load and per update;
- client time-to-interactive;
- main-thread time;
- memory.

Measure on Windows (Electron and the PWA) and on macOS where possible.

Design concerns each option must answer:
- Sandboxed topic-body frames (`srcdoc`, cookie-less, `data-relay` images) and the localhost cookie / LAN token access rules.
- Standalone and disk whiteboard pages, which share templates today, and their interaction with `261116-feat-ws-whiteboard-retire-disk-pages`.
- Stream deltas: the `board-topics` transport, held hashes, and reconnect recovery.
- i18n: page and topic-frame subsets.
- Known-target navigation (eN/pN deep links), panel-diff gutter, mermaid.
- Compatibility tier (AGENTS.md `### Compatibility Tiers`) for any change to the stream event shape, region payload or held-hash semantics, with read and write sites.
- Whether (C) already brings the heavy session under an acceptable first load. If so, (A) or (B) may not be worth their cost.

## Outcome Ledger

### Verified Findings

- The first load of a ~200-card session was slow on a macOS PC without antivirus. This is the owner's report of 2026-11-17, not a measurement.
- Predecessor Phase 1 payload numbers above (4e3746a0e Result).

### Confirmed Decisions

- (1294)=c, 2026-11-17: capture this as research now. Discuss it after the daemon file-I/O round 2 lands. Predecessor Phase 2 proceeds independently.

### Proposals

- The predecessor's non-authoritative direction, (B) per-topic fragment cache plus client assembly, is one candidate, not a decision.

### Open Questions

- Which of (A), (B) and (C) to pursue, decided from measurements after Phase 2 lands.
- The acceptable first-load target for a heavy session (an owner preference).

### Rejected Alternatives
