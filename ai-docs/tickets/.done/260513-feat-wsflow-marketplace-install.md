---
title: wsflow marketplace and installer registration
parent: 260513-epic-wsflow-agentless-plugin
spec:
  - 260513-wsflow-marketplace-install
  - 260513-wsflow-installer-snapshot
related-mental-model:
  - plugin-runtime
  - claude-compatibility
completed: 2026-05-13
---

# wsflow marketplace and installer registration

## Background

The wsflow package now has Codex and Claude manifests, a package-local MCP
configuration, runtime contract tests, and a curated skill bundle. It still
needs the local installation and marketplace path that lets users discover and
install it separately from the full ws plugin.

## Decisions

Keep wsflow as a separate product identity in distributed metadata. The
repository may document that wsflow derives from shared ws runtime code for
maintenance, but the local plugin entry and installed Claude snapshot should
not ask users to think of wsflow as a ws mode.

Treat marketplace, installer, and release validation as one cohesive slice:
the package is only practically installable when all three agree on naming,
paths, and validation commands.

## Constraints

Do not change the existing ws install behavior except where shared installer
logic must support multiple local plugin packages. Preserve current ws
marketplace metadata, Claude snapshot behavior, and update-mode semantics.

Do not touch presentation materials during this ticket.

## Phases

### Phase 1: Register wsflow in local install and release validation

Update local marketplace metadata and installer/update behavior so wsflow is
available as a separate Codex and Claude-compatible plugin package alongside
ws. Keep installed paths, server keys, plugin names, and user-facing display
text distinct.

Update release or ship verification guidance so normal validation includes the
wsflow marketplace/install path in addition to package manifest validation and
package tests. Prefer small shared installer helpers over duplicated shell
blocks when the existing structure supports it.

### Result (c0d07ec) - 2026-05-13

Added wsflow to the Codex local marketplace, extended `install.sh update` to
snapshot and install both ws and wsflow through the local Claude marketplace,
and updated release and README guidance for separate wsflow installation.
Verification covered syntax, temp-HOME update-mode install behavior, package
tests, and Claude manifest validation for both plugin packages.

## Correction (2026-05-13)

The first implementation incorrectly made `install.sh update` snapshot, enable,
and install wsflow for Claude. Follow-up commit removes wsflow from `install.sh`
and keeps wsflow available through the Codex marketplace entry only.
