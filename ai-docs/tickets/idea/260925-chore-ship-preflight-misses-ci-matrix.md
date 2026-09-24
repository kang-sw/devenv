---
title: Ship pushes main and the release tag before develop CI has run
related:
  260924-bug-ticket-index-read-timeout-below-ssh-roundtrip: its storage tests were the ones that failed only on the CI matrix at v0.46.18
---

# Ship pushes main and the release tag before develop CI has run

## Background

The v0.46.18 ship (`ai-docs/ship/ws.md`) passed every pre-flight check,
including `go test ./...` on the maintainer's mac (Apple git 2.50.1, with a git
identity configured). It then pushed develop, main, and `v0.46.18` back to back.
The tag-triggered `ws-mcp release` run (36072178336) failed on both Linux and
Windows, so no release assets were published. By then main already carried
`release_tag: v0.46.18`, and the launcher download for downstream installs from
main returned 404 until a fix-forward release.

`.github/workflows/ws-mcp-ci.yml` already runs the same Linux + Windows matrix
on every develop push. It failed on 2b0277f7 as well. It could not gate the
ship, though: develop had 137 unpushed commits, so its first CI run started in
the same minute as the main and tag pushes.

Failure classes the mac pre-flight cannot see:
- newer git wording (git 2.55 `[remote rejected] (incorrect old value
  provided)` for a lost CAS push);
- a missing host git identity in test fixtures;
- Windows-only process and timeout behavior in test harnesses.

## Open questions

- Should the ship config push develop first, wait for `ws-mcp CI` to go green
  on the pinned SHA (`gh run watch`), and only then promote main and push the
  tag? The alternative is to dispatch CI on a throwaway ref before any release
  push. The first option still publishes develop early, which this project
  already does.
- Should routine develop pushes happen between ships, so that CI signal
  accumulates before release time instead of all arriving at once? This is a
  posture question for this repository only; the ship config is
  project-local, not shipped text.
- Whether `lead-ship`'s generic procedure should name a "CI green on the
  promoted SHA" pre-flight hook for downstream projects that have CI. That
  would be a shipped-playbook change and needs its own decision.
