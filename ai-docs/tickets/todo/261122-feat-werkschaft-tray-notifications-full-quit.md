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

- Window close leaves the desktop app resident in the tray. The tray right-click menu offers "닫기" and "완전 종료"; full quit stops all holders and the daemon, ending all asynchronous sessions.
- Present two user-facing lifecycle tiers: the app and the background daemon. Holders are a daemon implementation detail, not a third user-facing lifetime.
- Provide native notifications. The event selection and fallback mapping remain open.

## Constraints

- Apply `ai-docs/manuals/shipped-surface-boundary.md` for `agents-plugin-tool/`, `ai-docs/manuals/performance.md` for `agents-plugin-tool/`, `ai-docs/manuals/release-compatibility.md` for `agents-plugin-tool/internal/`, and `ai-docs/manuals/ws-mcp.md` if `agents-plugin-tool/internal/mcp/` is touched, per AGENTS.md Implementation Conventions. Select a storage type before adding persisted data; account for interactive file operations and Windows antivirus costs.
- A new route and any other cross-version surface require a directional compatibility verdict backed by old/new readers, writers, and recovery evidence. T0 is only a proposal in (2089); T3 requires explicit owner approval and a minor bump; T4 cannot ship.
- Settle the open decisions before promotion. Preserve the existing authenticated and origin boundaries; design and test the authorization and failure semantics of any full-stop route. Test destructive lifecycle operations only against isolated, owned test processes and profiles.

## Open Decision Queue

Last ID: (2093)

- (2085) [policy, open] Does tray "닫기" quit only the desktop app while leaving the daemon and holders running? This is the lead's assumed reading and needs owner confirmation.
- (2086) [policy, open] Confirm that X-close hides to tray, superseding the prior approved CloseRequested and draft-recovery close behavior exercised by the Tauri prerequisite (its Phase 1 close contract and Phase 4 Windows check; Phase 2 requires daemon survival).
- (2087) [policy, open] Should the draft check run on an actual app quit, and not when merely hiding the window to the tray? Specify recovery and failure behavior.
- (2088) [policy, open] When asynchronous sessions are running, should full quit show their count and require confirmation? Define cancellation and incomplete-stop behavior.
- (2089) [technical, open] What route and authentication contract should stop all holders plus the daemon, including ordering, races and partial failures? An additive route with T0 compatibility is proposed; establish the actual directional tier with evidence before deciding.
- (2090) [policy, open] Which events raise native notifications, including questions awaiting the user and child completion, and how should they map to the dashboard's existing browser-notification fallback without duplicates?
- (2091) [technical, open] How should macOS unsigned-app notification permission risk and Windows AppUserModelID for portable launches be handled and verified?
- (2092) [policy, open] What should a tray left-click do, including restoring and focusing the app?
- (2093) [policy, open] Should this behavior be Tauri-only after adoption, or also have Electron parity while Electron remains supported?

## Phases

The phases describe provisional work boundaries; implement only after the prerequisite adoption decision and this queue are settled. Later phases depend on the preceding phase's landed Result.

### Phase 1: Full-stop daemon contract

Implement the settled daemon-and-holders full-stop contract and its authorization. Test holder discovery and termination, daemon shutdown, running asynchronous sessions, races, failures and the chosen compatibility behavior. Preserve ordinary daemon replacement and its surviving holders.

### Phase 2: Tray and desktop lifecycle

Build on Phase 1 to add tray residency, the right-click actions and the settled close, draft and confirmation behavior. Verify hide, the chosen restore and quit actions, full quit, cancellation and failure paths against an isolated daemon; expose only the agreed two user-facing tiers.

### Phase 3: Native notifications

Implement the settled native notification events, permissions and browser-fallback mapping. Verify event selection, no duplicate delivery, denied permissions and the agreed platform identity handling.

### Phase 4: Packaged macOS and Windows verification

Exercise the complete lifecycle and notification flow on packaged macOS and Windows builds, including running session counts and cancellation, draft recovery, daemon and holder termination, notification permission and identity, and the selected Electron parity scope. Record platform versions, signing and launch conditions, evidence and remaining limitations.
