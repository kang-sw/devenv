---
title: "dashboard 내부 익명 피드백 버튼"
related:
  261118-feat-ws-remote-chat-bridge: sibling idea from the same discussion
---

# In-dashboard anonymous feedback button

## Background

Parked idea from the 2026-11-18 feedback-channel discussion (lead session
sharpener-washstand-limeade, board topic `feedback-channel`).

Goal: a feedback button inside the dashboard, like the "Report Issue" button of
an ordinary open-source desktop program (the owner cited VS Code), that lets an
outside user send feedback without signing in to anything. VS Code's reporter
still needs a GitHub account; this idea removes that step.

Why it is parked (owner, 2026-11-18 16:11): in-house users can message the
owner directly and outside users can file GitHub issues, so the current
channels are sufficient for now. Revisit when outside usage grows or GitHub
sign-in proves to be a real barrier.

## Decisions

- Audience is outside, public users of the shipped workflow product, not only
  in-house colleagues (owner answer (77)=b, 2026-11-18).
- Preferred shape is an in-dashboard button posting to a sign-in-free receiver,
  over a plain form link or `mailto:` (owner preference, 2026-11-18). Rejected
  for now: Google Form link (a file-upload question forces Google sign-in),
  other hosted form services, MS Forms (in-house only), `mailto:` (manual
  triage).

## Constraints

- Nothing below is settled; it is the evidence and the open questions the
  promotion to `todo/` must settle through the Open Decision Queue.
- Evidence (Pi exploration, 2026-11-18): the dashboard has no outbound
  network path today; the daemon binds `127.0.0.1` by default
  (`agents-plugin-tool/internal/wsdashboard/daemon.go:561-571`). A feedback
  POST would be the product's first outbound call, so it falls under
  `ai-docs/manuals/shipped-surface-boundary.md` (downstream-first) and the
  file-touch rule in `ai-docs/manuals/performance.md`.
- Open questions raised but not answered:
  - Where submissions land: private storage (for example a sheet) with manual
    promotion to GitHub issues, or auto-created public GitHub issues ((78),
    folded into this idea without an answer). Privacy risk of the second: an
    anonymous user may paste internal paths or code into a public issue.
  - Receiver host: Google Apps Script web app (free, sheet built in, no
    per-IP rate limiting) versus a Cloudflare Worker or similar (IP rate
    limiting, one more account and deploy). The endpoint URL ships in public
    source, so spam handling (size cap, honeypot field, rate limit) is needed.
  - Downstream control: whether the endpoint is a config value that a
    downstream project can disable or redirect to its own receiver.
  - Consent and payload: send only on an explicit click with a preview of
    exactly what is sent; which diagnostics may be attached (version, OS,
    harness, locale) and which never are (logs, paths, code).
  - Screenshot attachment (capture library weight, leak risk).

## Phases

### Phase 1: Settle and build

Settle the open questions above, add a mockup of the button and compose
sheet, then implement the button, the opt-in payload preview and the receiver.
