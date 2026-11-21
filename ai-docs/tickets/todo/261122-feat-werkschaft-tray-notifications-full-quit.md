---
title: "데스크톱 트레이 상주, 네이티브 알림, 완전 종료"
related:
  261121-feat-werkschaft-tauri-shell-return: "Phase 4 adoption decision and desktop shell prerequisite"
blocked-by: 261121-feat-werkschaft-tauri-shell-return#phase4
---

# Werkschaft tray residency, native notifications, and full quit

## Background

The owner asks for a simpler desktop lifecycle and notifications:

> tauri 앱 트레이에 숨기기 + tauri 네이티브 알람 등 구현이 가능할까요? 데스크톱 앱을 끄더라도 기본적으로는 트레이로 가게 하고, 트레이에 우클릭했을 때 "닫기"와 "완전 종료(모든 홀더와 데몬 끄기 = 모든 비동기 세션 종료)"가 되게요. 지금 수명이 앱 수명 / 데몬 수명 / 홀더 수명으로 삼중화되어서 좀 혼란스럽긴 한데, 앱 표시 쪽에서는 앱 자체 수명하고 백그라운드 데몬 수명으로 이원화하는 게 개념적으로는 더 다루기 편할 듯합니다.

Owner decision (2084)=a: capture a separate todo ticket now; implement only after `261121-feat-werkschaft-tauri-shell-return` Phase 4's Result records the owner decision adopting Tauri. The phase Result alone is insufficient if it does not adopt Tauri; revisit this ticket if the decision differs.

Evidence at `impl/develop/basin-spray-cider`: `agents-plugin-tool/desktop/src/main.rs:105-108` intercepts `CloseRequested` and calls `close::request`; `desktop/src/close.rs:17-46` checks draft readiness or asks for confirmation before destroying the window (local pages are destroyed directly). `desktop/Cargo.toml` pins Tauri 2.12.5 with `devtools`, no tray feature, and no notification plugin. The prerequisite's Phase 2 requires the daemon to survive shell exit and holder identities to survive daemon handoff.

Evidence at `develop`: `agents-plugin-tool/internal/wsdashboard/daemon_http.go:153-168` authenticates `/shutdown` and calls `d.Close()` for the daemon; `internal/wsholder/manager_test.go:168-173` documents holders and their children surviving daemon restart. `internal/wsdashboard/holders.go:46-72` exposes an individual holder stop. A search of `agents-plugin-tool` for all-holder shutdown and stop paths found no all-holder stop path. Verify the current baseline again before implementation.

## Decisions

- Owner decisions (2026-11-22 01:08 KST):
  - (2085)=a: Tray "닫기" exits only the desktop app; the daemon and holders continue running. Rejected: stopping the daemon or holders on "닫기".
  - (2086)=a: Window X-close hides the app to the tray. This supersedes the Tauri prerequisite's Phase 1 close behavior and its Phase 4 Windows check for this subsequent feature; Phase 2's daemon-survival requirement remains. Rejected: exiting the app on X-close.
  - (2087)=a: Check for unsent drafts and confirm only on real app exit via "닫기" or "완전 종료", not on hide. Preserve draft recovery and make check or exit failures recoverable. Rejected: prompting on hide.
  - (2088)=a: "완전 종료" confirms only when sessions are running and displays their count. Cancellation leaves the app, daemon and holders running; an incomplete stop must be reported rather than represented as a successful full quit. Rejected: unconditional full-quit confirmation.
  - (2089) confirmed (technical): Add a full-stop route alongside the existing bearer-authenticated `/shutdown`; stop all holders before the daemon and report remaining holders on partial failure. Keep ordinary `/shutdown` and daemon replacement behavior. Establish the directional compatibility tier from evidence; T0 is a proposal, not a settled verdict.
  - (2090)=a, with owner refinement: Notify on a question to the user and on session completion only once the session is fully idle: "정확히는, 모든 자식이나 백그라운드 태스크가 종료되어서 완전 idle이 된 시점이요." Do not notify for each child completion. Use native notifications inside the app without duplicate browser notifications. Rejected: per-child completion notifications and duplicate browser delivery.
  - (2091) confirmed (technical): Measure macOS unsigned-app notification permission and Windows portable AppUserModelID in the Phase 4 package build; fall back to browser notifications if either platform's native notification path fails.
  - (2092)=a: Tray left-click restores and focuses the window; on macOS follow menu-bar convention and open the menu. Rejected: leaving the window unrestored on other platforms.
  - (2093)=a: Implement this feature for Tauri only; Electron is slated for retirement and is not updated. Rejected: Electron parity.
- Present two user-facing lifecycle tiers: the app and the background daemon. Holders are a daemon implementation detail, not a third user-facing lifetime.

## Constraints

- Apply `ai-docs/manuals/shipped-surface-boundary.md` for `agents-plugin-tool/`, `ai-docs/manuals/performance.md` for `agents-plugin-tool/`, `ai-docs/manuals/release-compatibility.md` for `agents-plugin-tool/internal/`, and `ai-docs/manuals/ws-mcp.md` if `agents-plugin-tool/internal/mcp/` is touched, per AGENTS.md Implementation Conventions. Select a storage type before adding persisted data; account for interactive file operations and Windows antivirus costs.
- A new route and any other cross-version surface require a directional compatibility verdict backed by old/new readers, writers, and recovery evidence. T0 is only a proposal in (2089); T3 requires explicit owner approval and a minor bump; T4 cannot ship.
- Preserve the existing authenticated and origin boundaries; design and test the authorization, ordering, race and failure semantics of the full-stop route. Test destructive lifecycle operations only against isolated, owned test processes and profiles.

## Phases

Implement only after the prerequisite Phase 4 Result records the owner decision adopting Tauri. Later phases depend on the preceding phase's landed Result.

### Phase 1: Full-stop daemon contract

Implement the authenticated full-stop route: discover and stop all holders before stopping the daemon, and report remaining holders on partial failure. Design for concurrent starts and stop races. Test authorization and origin boundaries, discovery, termination, daemon shutdown, running sessions, races, partial failures and the evidenced directional compatibility tier. Preserve ordinary `/shutdown`, daemon replacement and surviving holders.

### Phase 2: Tray and desktop lifecycle

Build on Phase 1 for Tauri: X hides to tray without a draft prompt; "닫기" exits only the app; "완전 종료" stops holders and daemon. Check unsent drafts on real exit, and confirm full quit only for running sessions, showing their count. Left-click restores and focuses except on macOS, where it opens the menu. Verify hide and restore, both exit actions, draft recovery and check failures, zero and nonzero session counts, cancellation, and incomplete-stop reporting against an isolated daemon; expose only the agreed two user-facing tiers.

### Phase 3: Native notifications

Implement native notifications for user questions and session completion at full idle after all children and background tasks finish. Map native failure to browser fallback without duplicate delivery. Verify questions, child completion while work remains, final idle, permissions, denied or failed native delivery and fallback.

### Phase 4: Packaged macOS and Windows verification

Exercise the complete Tauri lifecycle and notification flow on packaged macOS and Windows builds, including running session counts and cancellation, draft recovery, daemon and holder termination, full-idle event selection and duplicate-free fallback. Measure notification permission on unsigned macOS and AppUserModelID on portable Windows; verify browser fallback if the native path fails. Record platform versions, signing and launch conditions, evidence and remaining limitations. Electron is outside this feature's scope.
