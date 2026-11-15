---
title: "Launcher: a minor version bump in the devenv source breaks the installed plugin until reinstall"
---

# Launcher: a minor version bump in the devenv source breaks the installed plugin until reinstall

## Background

Dogfood capture (2026-10-21). After `32c6e4d57` bumped the source tree to
`ws@0.48.0`, the installed Claude plugin snapshot (`ws@0.47.0`, contract
`required_mcp: ">=0.47.0-dev <0.48.0"`) stopped starting its MCP server, and
`ws-cli` failed the same way, so neither the tools nor the documented CLI
fallback were usable.

With the local devenv runtime contract active, the installed launcher
rebuilds `ws-mcp` from the source tree whenever the source fingerprint
changes. The build succeeds, but the 0.48 binary is outside the installed
contract's range even with `allow_same_minor`, and the launcher fails hard.
`WS_MCP_LAUNCHER_DEBUG=1` output:

```text
ws-mcp-launcher: local devenv source changed or runtime missing; rebuilding from source
ws-mcp-launcher: installing or repairing incompatible runtime
ws-mcp-launcher: runtime capabilities version mismatch
ws-mcp-launcher: local devenv build failed or produced incompatible runtime: exit=0
ws-mcp-launcher: local devenv runtime was forced but no compatible local runtime could be installed
```

Only the last line shows without the debug variable, and it does not say
that the fix is to refresh the install (`./install.sh update`). The
source-tree launcher (`python3 agents-plugin/bin/ws-mcp-launcher.py`)
worked as a stopgap, since its contract matches the source.

## Decisions

### Compatibility addendum - 2026-11-15

- Under `AGENTS.md` and `ai-docs/manuals/release-compatibility.md` as landed in
  `c0ec803cb`, the reported old-plugin startup lockout is a **T3 old-version
  boundary**, not the former “T2 breaking” category. T3 requires a minor bump
  and explicit owner approval; prefer transparent T0 behavior where feasible.
  This grades the refusal, not an unchosen repair, and does not authorize
  relaxing a runtime or shared-data safety check.
- Current-source analogue, not a re-test of the historical 0.47 launcher:
  the new runtime producer supplies version metadata; the installed-style
  launcher reads it against its plugin contract. `version_compatible` permits
  local drift only within the same minor
  (`agents-plugin/bin/ws-mcp-launcher.py:134-149`), and
  `runtime_capabilities_compatible` rejects a mismatch (`:731-739`). Forced
  local-runtime failure terminates instead of downloading a fallback (`:723-726`).
  This is runtime admission, not a shared-session/store writer; reopening data
  cannot make the rejected runtime acceptable. The historical observation above
  remains the incident evidence; these source lines describe this checkout.
- T0–T2 now permit patches, but T2 specifically requires an old write followed
  by demonstrated reconstruction on new-version reopen. No overwrite/recovery
  path is claimed here. T4 (a new binary unable to read old data) never ships;
  migration must lower it to T3 or below. The fallback-versus-message question
  below remains open; this addendum chooses neither architecture nor behavior.

## Open Questions

- Should a version-incompatible local build fall back to a compatible cached
  or released runtime instead of failing, or should it keep failing with an
  actionable message naming the reinstall step?
- Is this only a devenv-maintainer concern (the local devenv contract is
  inactive downstream), which would bound the fix to the message and this
  repository's manuals?

## Resolution
Triage 2026-11-15 (owner (1057)): DROP - Older plugins being blocked on a minor bump is the already known policy (T3 boundary); no separate action needed.
