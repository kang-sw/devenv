---
title: "Werkschaft용 Tauri 셸 재검토 (WKWebView 가용성, 용량, 메모리)"
related:
  261115-feat-werkschaft-electron-shell: the Electron migration this research may reverse
  261113-feat-werkschaft-thin-tauri-shell: the earlier Tauri thin shell
  261112-research-ws-ade-desktop-shell-and-rename: original shell research
---

# Revisit a Tauri shell for Werkschaft (WKWebView viability, size, memory)

## Background

On 2026-11-15 the owner chose (843)=b: migrate the Werkschaft desktop shell
from Tauri to Electron (261115-feat-werkschaft-electron-shell). The recorded
reason was macOS: Tauri uses WKWebView there, so browser defaults (zoom keys,
external links, close confirmation / `beforeunload` on window close) had to be
re-implemented natively in the Tauri host, and Electron's bundled Chromium was
expected to remove the "works in the browser, not in the app" class of defects.

On 2026-11-17 the owner kept Electron for now ((1279)=a) and asked for this
research. Owner, verbatim: "네, 다만 todo에 tauri 리서치로 남겨놓고 macos 등
사파리 기반 웹뷰에서 실제로 문제가 되는지(그때는 모달이랑 줌 정도 때문에 바로
도망쳤었는데, 이 부분은 곰곰이 생각해보면 얼마든지 구현이 가능한 부분들이라)
확인하는 리서치를 하나 둡시다. 배포 용량이나 메모리 사용량 같은 측면에서
tauri가 더 가벼운 건 꽤 자명해 보여서요."

In short: the earlier retreat was mainly over modals and zoom, which now look
implementable; Tauri is expected to be lighter in distribution size and memory.
The question is whether WebKit-based webviews (macOS WKWebView) cause real,
lasting problems for the dashboard, and what a return would cost.

## Questions

1. **WKWebView viability.** Run the current dashboard in a Tauri shell on
   macOS. Which browser defaults and dashboard features fail or differ? Cover
   at least: modals and close confirmation, zoom keys, external links, file
   and image handling, clipboard, IME (Korean), and CSS features the
   dashboard uses (`:has()`, `content-visibility`, container queries). For
   each gap, estimate the native implementation cost in the Tauri host.
2. **Windows.** WebView2 is Chromium (Edge) based. Confirm parity with the
   Electron build, and note the Evergreen runtime version variance across
   user machines.
3. **Size and memory.** Measure, not assume:
   - installed and download size of a Tauri build vs the Electron build
     (win32 and darwin), excluding and including the bundled daemon;
   - idle and working-set memory with the same board open.
4. **Rendering performance.** Run the same dashboard scroll and typing
   scenario on WebView2, WKWebView and Electron Chromium.
5. **Operations.**
   - DevTools availability in release builds.
   - Tauri's signed updater and installer (NSIS/MSI, dmg) vs the current
     custom Electron updater (Windows self-update issues).
   - Code-signing requirements per platform.
6. **Porting cost** of Electron-side work already done:
   - bundled daemon auto-start, including the Windows job-object breakaway
     fallback;
   - attach-to-running-daemon;
   - the DevTools toggle;
   - request/navigation guards;
   - the localhost token/cookie injection;
   - the Python-missing guide.

## Constraints

- Research only. No change to the shipped Electron shell from this ticket. A
  shell change becomes a separate actionable ticket after the owner decides.
- Follow the machine rules in force when it runs: no local `go test` or ws
  executables on the owner's Windows PC; Go builds and tests on remote
  workers; temp files under `D:/Temp/<purpose>/`.

## Outcome Ledger

### Verified Findings

- The 11-15 switch rationale is recorded in
  261115-feat-werkschaft-electron-shell Background: on macOS WKWebView,
  browser defaults must be re-implemented natively (Tauri host `main.rs`
  around lines 96-202 at the time); Electron was the recorded fallback in the
  Tauri ticket's Decision 1.
- Recorded sizes:
  - Electron costs about 100 MB per version (261115 decision 857).
  - The ws-mcp binary is about 41 MB (261112).
  - No measured Tauri bundle size is recorded in the repository.

### Confirmed Decisions

- (1279)=a, 2026-11-17: keep Electron for now. Prioritize performance and the
  auto-updater, then use this research to decide whether to return to Tauri.

### Proposals

### Open Questions

- Questions 1-6 above.

### Rejected Alternatives
