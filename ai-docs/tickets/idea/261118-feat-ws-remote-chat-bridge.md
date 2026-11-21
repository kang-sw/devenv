---
title: "원격 대화 전용 앱: 휴대폰·데스크톱에서 lead 세션과 대화하고 파일을 명시적으로 push"
related:
  261118-feat-ws-dashboard-anonymous-feedback-button: sibling idea from the same discussion
  261113-refactor-ws-dashboard-drop-lan-serving: remote-access direction (local-only daemon, remote later)
  261112-research-ws-daemon-extension-protocol-chat-adapter: home of the swappable transport adapter (daemon connects out to an adapter)
---

# Remote conversation app for lead sessions (phone and desktop)

## Background

Parked idea from the 2026-11-18 feedback-channel discussion (lead session
sharpener-washstand-limeade, board topic `feedback-channel`).

Goal: drive lead sessions remotely from a phone chat app: send messages and
board answers (`76=a` form) to a session, and receive lead replies, new open
asks, approval waits and completion notices.

Why it is parked (owner answer (76)=c, 2026-11-18): a chat bot is plausible
long term, but the owner is uneasy that Telegram stores message plaintext on
its servers. A direction must be worked out first.

## Direction update (2026-11-21)

Revisited in the owner's idea triage (lead session board topic
`idea-remote`). The goal is unchanged (talk to lead sessions from away from
the desk); the shape changed from "chat bot in a third-party app" to "our own
remote app". This ticket is updated in place rather than replaced because the
goal, the inbound/outbound evidence below, and the confidentiality concern
all carry over.

## Decisions

- Parked as an idea; no implementation now (owner answer (76)=c,
  2026-11-18).
- Application-level encryption of message content with a master password was
  considered and set aside by the owner: the human reader would then need a
  separate decrypting app (owner, 2026-11-18 15:47). Superseded on
  2026-11-21: the remote app is now our own app, so a decrypting client is
  no longer an extra cost (owner, board (1485) note).
- Our own remote app, designed from scratch for phone and PC desktop alike:
  conversation only plus an explicit file-push model; the whiteboard and
  other dashboard surfaces are left out of the remote app (owner, (1485)
  note, 2026-11-21).
- A chat that arrives from the remote app reaches the lead with a notice
  that the user may not see the whiteboard (wording to settle; e.g. "this
  chat came from the remote app; the user might not see the whiteboard"), so
  the lead answers in the chat (owner, (1474) note, 2026-11-21).
- Transports are compared on remote setup ease and mobile accessibility;
  reuse of an existing client app is not a criterion (owner, (1485) note).
- Still an idea; ticketize only, no research or implementation dispatched
  yet (owner, (1486) note, 2026-11-21).

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

- Transport candidates after the 2026-11-21 revisit (lead's assessment from
  model knowledge, NOT verified; prices, plans, and platform limits must be
  checked before any choice):
  - Matrix as the transport (owner's current leaning, recorded from the
    (1485) note "B안", lead's reading): E2E built into the protocol
    (Olm/Megolm), the user picks a homeserver (matrix.org, paid hosting, or
    self-hosted), encrypted attachments fit the file-push model. Open
    points: E2E device verification friction for a first-time user, and
    whether our own app needs its own push gateway (Sygnal-like) and APNs
    credentials, which would put one server on us anyway.
  - Own relay with a master-password-derived key: simplest first setup
    (scan a QR from the desktop, enter the password), but we run the relay
    (cost, availability) and own the E2E design.
  - Tailscale (or another personal VPN) to the local dashboard: E2E by
    WireGuard and no relay of ours, but on iOS it needs the VPN active
    (one VPN at a time) and gives no push; deferred for mobile accessibility
    (owner, (1485) note).
  - Telegram used as a free carrier (e.g. two accounts): once content is
    encrypted the Telegram app cannot read it, so a custom client is needed
    anyway, and automating a user account risks bans; lead advised against.
- Facts to verify when this leaves the idea stage: matrix-rust-sdk mobile
  bindings and whether device verification can be automated between the
  daemon-side bot and the app; whether a custom Matrix app needs its own push
  gateway; paid Matrix hosting prices and matrix.org bot-account policy; cost
  and WebSocket limits of a hosted relay (e.g. Cloudflare Workers).
- Swappable transport: the adapter boundary belongs to
  `261112-research-ws-daemon-extension-protocol-chat-adapter`; this ticket
  stays transport-agnostic above that boundary.

## Phases

### Phase 1: Choose a direction

Verify the facts listed in `## Constraints`, pick the transport for the
remote app (setup ease and mobile accessibility first), and plan the app
and its daemon-side adapter as a `todo/` ticket.
