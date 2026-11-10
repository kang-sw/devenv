---
title: "Dashboard: shared toast framework, first used by the tracker notification layer, and tab-label alerts"
related:
  261110-bug-ws-dashboard-render-and-frontend-load-accumulation: "its Decision 10 keeps the main stream open while the tab is hidden so tracker events still arrive; this ticket builds the alert surface on that"
---

# Dashboard toast framework and tab-label alerts

## Background

User, 2026-11-10 whiteboard: "닫혀있는 탭에서도 예를 들어 알람이라던지
날아오면 사실 라벨에 표시되는 게 더 좋지 않나요? 아니면 전반적으로
프론트엔드에 Toast 프레임워크를 만들고, 첫 사용자로 store 쪽의 알림 레이어로
쓰면 어떨까" - an alert should show in the tab label even while the tab is
in the background; and a general frontend toast framework whose first user
is the tracker store's notification layer. Board (589) = a: a separate
feature ticket, not part of the performance ticket.

Current state (develop 88326de57, code reading):

- Nothing sets `document.title` or the favicon for alerts.
- Two independent single-message toasts exist: `ltoast` in
  `wsdashboard/assets/dashboard.js` (~:1193-1211; copy and open results) and
  `tktoast` in `wsdashboard/assets/tracker.js` (~:2806-2812; save,
  register, and upload results). Each shows one message and hides it after
  a timer.
- Tracker notifications reach the page as `tracker` events on the main
  stream (`dashboard.js` ~:3517) and show only in the Notify tab and the
  `UNREAD` badges (`tracker.js` ~:582).
- 261110-bug-ws-dashboard-render-and-frontend-load-accumulation Decision 10
  (board (588)) keeps the main stream open while the tab is hidden and still
  handles `tracker` events then, which is what lets a hidden tab learn of
  new notifications.

## Confirmed direction (user, 2026-11-10)

- One shared toast framework replaces the two toasts.
- Its first consumer is the tracker store notification layer.
- The tab label shows alerts while the tab is in the background.

## Open questions

- Which events count toward the tab label: tracker unread notifications,
  open whiteboard asks waiting on the user, pending permission prompts?
- Label form: an unread count prefix in `document.title`, a favicon badge,
  or both; when it clears (on focus, on read).
- Toast behavior: stacking limit, persistence for actionable toasts,
  click-through to the item or comment, and whether a toast fires for
  events that arrive while hidden (queued for show) or only while visible.
- Whether browser `Notification` API (OS-level) alerts are in scope.
