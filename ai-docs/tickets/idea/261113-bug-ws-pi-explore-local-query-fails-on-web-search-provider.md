---
title: "Pi explore on a local-repository question fails with a web-search provider error"
---

# Pi explore on a local-repository question fails with a web-search provider error

## Observed (2026-11-13)

A Pi lead session called the explore tool for a purely LOCAL repository investigation. The tool returned a web-search-tool-unavailable / provider error instead of searching the repo. The session fell back to two read-only forks. Reported by the Pi peer lead; it was not reproduced here.

## Ideas (non-authoritative)

- Check whether explore routes every query through the web-search provider, or fails as a whole when that provider is unavailable.
- A local question should not depend on web search being available.
