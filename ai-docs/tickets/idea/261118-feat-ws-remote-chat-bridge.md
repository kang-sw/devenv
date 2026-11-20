---
title: "휴대폰에서 lead 세션을 제어하기 위한 원격 채팅 브리지"
related:
  261118-feat-ws-dashboard-anonymous-feedback-button: sibling idea from the same discussion
  261113-refactor-ws-dashboard-drop-lan-serving: remote-access direction (local-only daemon, remote later)
---

# Remote chat bridge for driving lead sessions from a phone

## Background

Parked idea from the 2026-11-18 feedback-channel discussion (lead session
sharpener-washstand-limeade, board topic `feedback-channel`).

Goal: drive lead sessions remotely from a phone chat app: send messages and
board answers (`76=a` form) to a session, and receive lead replies, new open
asks, approval waits and completion notices.

Why it is parked (owner answer (76)=c, 2026-11-18): a chat bot is plausible
long term, but the owner is uneasy that Telegram stores message plaintext on
its servers. A direction must be worked out first.

## Decisions

- Parked as an idea; no implementation now (owner answer (76)=c,
  2026-11-18).
- Application-level encryption of message content with a master password was
  considered and set aside by the owner: the human reader would then need a
  separate decrypting app (owner, 2026-11-18 15:47).

## Constraints

- Nothing below is settled; it is the evidence and the options the promotion
  to `todo/` must settle through the Open Decision Queue.
- Evidence (Pi exploration, 2026-11-18):
  - No Telegram, Slack, Discord, Matrix or webhook chat bridge exists;
    `slack-ping` in `agents-plugin-wsflow/hooks/claude-mod/wake.ts:11` is an
    event-origin name only.
  - Inbound insertion point: the composer posts
    `POST /s/<reply-id>/message`; the daemon checks same origin and the
    localhost credential, enqueues into `wsinbox`, and the host adapter drains
    it as a user turn
    (`agents-plugin-tool/internal/wsdashboard/inbox_routes.go:25-38,153-180,490-545`,
    `agents-plugin-tool/internal/mcp/dashboard_inbox.go:14-20,50-84`).
    Steer and interrupt routes exist.
  - Access: the daemon binds `127.0.0.1` by default; optional LAN mode uses a
    separate token (`wsdashboard/daemon.go:561-571`,
    `wsdashboard/access.go:17-51,122-168`).
  - Outbound: the session WebSocket streams rendered HTML regions
    (`wsdashboard/sse.go:338-373`), so relaying replies and board asks needs
    a structured event feed. The tracker hook
    (`agents-plugin-tool/internal/wstrackerhook/doc.go:1-26`) can call an
    external command per tracker event, enough for notifications only.
  - Rough size: text-only single-owner local prototype ~300-600 lines plus
    tests; durable multi-session bridge with structured outbound events
    1,000+ lines.
- Options discussed, none chosen:
  - Telegram bot with outbound long polling (no open port). Bot chats are
    never end-to-end encrypted; content is stored on Telegram servers.
    Mitigations: chat/user ID allowlist, TOTP for dangerous commands
    (approve, interrupt, merge), re-auth timeout, sending only titles and
    item numbers instead of content.
  - Slack Socket Mode or Discord bot: same shape, same third-party storage.
  - Telegram Mini App decrypting client-side with WebCrypto: keeps content
    encrypted but needs a hosted HTTPS page and a ciphertext relay (about
    twice the prototype size).
  - No bot: open the dashboard from the phone over a personal VPN
    (Tailscale, WireGuard). End-to-end, little new code, but must be squared
    with `261113-refactor-ws-dashboard-drop-lan-serving` and company VPN
    policy.
  - Notifications-only first (new asks, approval waits, completions) as a
    smaller step.

## Phases

### Phase 1: Choose a direction

Pick the transport and the confidentiality posture from the options above,
then plan the bridge as a `todo/` ticket.
