---
title: "Local-devenv launcher fingerprint ignores embedded assets, so asset-only changes never rebuild"
completed: 2026-11-13
---

# Local-devenv launcher fingerprint ignores embedded assets, so asset-only changes never rebuild

## Background

Found while merging 6649015ce (tracker store page in the wide layout), which
changed only `wsdashboard/assets/dashboard.{css,js}`. After the merge,
`ws-cli --help` followed by `ws-cli daemon-start` returned
`outcome: running` with the old daemon, twice: the launcher reused the cached
binary, so the dashboard kept serving the pre-merge embedded assets.

`local_devenv_source_fingerprint` in `agents-plugin/bin/ws-mcp-launcher.py`
walks only `*.go` plus `go.mod`/`go.sum` and the toolchain. Files pulled in
through `//go:embed` (dashboard and whiteboard assets, rsrc playbooks if
embedded, i18n tables) are not in the fingerprint, so a change to them alone
keeps the stale binary. Workaround used: `touch` a Go file in the embedding
package (`internal/wsdashboard/lucide.go`), then the refresh replaced the
daemon.

## Direction (unconfirmed)

Include the embed inputs in the fingerprint: either every regular file under
the tool tree except known-irrelevant ones (tests, `.git`), or the directories
named by `//go:embed` patterns. Keep it stat-only (the startup-time constraint
in the function comment). Mirror the launcher copy in wsflow per
`ai-docs/manuals/wsflow-mirroring.md`.

Related: the deployment-compatibility discussion (board topic
`deploy-compat`) — a stale binary under a newer checkout is one of its
mixed-version cases.


## Resolution (2026-11-13)

Fixed in f2ea25fd5: local_devenv_source_fingerprint stats every regular file under tool_dir (skipping *_test.go, testdata/, dist*, dot-dirs), so an embedded-asset-only change rebuilds. Stat-only walk ~0.03 s on the real tree.
