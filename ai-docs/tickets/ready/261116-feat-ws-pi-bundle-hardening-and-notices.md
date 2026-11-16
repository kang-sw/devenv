---
title: Harden Pi bundle failure reporting, supported host range, and notices
sage-review-design: completed
sage-review-completeness: completed
---

## Intent

Prevent invisible Pi lead tool loss and ship deterministic, licensed extension artifacts with guarded native mirrors. Owner approved scope (1226)=a on 2026-11-16; implementation and a light ready check are authorized.

## Evidence

- Owner evidence: `62c84dcd-02-pi-agent-bundling-license-practicality.md` and `04b12939-01-pi-lead-tools-still-missing-after-fix.md` in the project review-paths cache. The latter diagnoses a model inventory mistake after a successful reload, not another current outage.
- `agents-plugin-tool/internal/wsagent/pi_lead.go`: `piLeadLaunch` falls back to MCP; `piBundledEntries` checks bundle freshness but can use source after mismatch. `PluginWarning` currently persists/logs the warning.
- `agents-plugin-pi/package.json`: host devDependency 0.84.4, peer `*`. Pi remains host-provided, not redistributed.
- `agents-plugin-pi/scripts/bundle.mjs`, `scripts/prepare-native-payload.py`, and `test/bundle-freshness.test.mjs` own generation and artifact freshness. Check both native-pi mirrors, last refreshed in the idle-stop integration.
- First repository commit year from `git log --reverse --format=%ad --date=format:%Y | head -1`: 2026. Only root LICENSE is a first-party license copy; bundled dashboard asset licenses are third-party and unchanged.

## Decisions

- Owner: keep committed dist/native payloads and no-Node release-host support; no release-time-only redesign, merge, push, or launcher execution.
- Item 1, T0 behavior-only: retain degraded MCP start for preflight failures to keep mail reachable, but expose an existing session error and inject an unmistakable system warning naming missing extension tools. Reject stale bundle input hashes even if source dependencies happen to exist. An independent, dependency-free watchdog also detects factory/session_start failure after preflight using a process-local completion signal emitted only after final awaited native initialization (not tool presence alone); it keeps dashboard/input-mail delivery alive and warns explicitly, without starting a second MCP client in healthy sessions. The RPC mapper carries notifications across the following turn-start error reset. No new stored/event shape.
- Item 2, T0 behavior-only host contract: declare `>=0.84.4 <1.0.0` from pinned devDependency 0.84.4 (tested baseline through the next major); check actual CLI-realpath host metadata at session startup and warn on unsupported/unverifiable host in the dashboard error and lead system prompt. Keep the host running rather than impose an old-version boundary: a metadata-only unknown/unsupported host may still work, and reachability beats a hard fail. Do not add a manifest field or protocol boundary.
- Item 3, T0 additive packaging: generate deterministic lockfile-derived `dist/THIRD_PARTY_NOTICES` from bundled dependencies' installed license texts and ship it with native first-party mirrors. Confirmed actual carried license texts: htmlparser2 10.1.0 is MIT; domelementtype 2.3.0 is BSD-2-Clause. boolbase 1.0.0 omits LICENSE in its npm tarball, so retain its upstream ISC notice explicitly in the generator.
- Item 4, T0 text-only: fill Apache appendix copyright with `Copyright 2026-2026 Seungwoo Kang (GitHub: kang-sw) <ki6080@gmail.com>` using owner-supplied identity. Preserve third-party license copies.
- Item 5, T0 build guard: regenerate first-party native src/dist as needed and assert each native manifest hash and dist header equals the source hash. Reuse existing `bundle_input_hash`, with no schema/manifest field addition (which would be T3).
- Compatibility: old readers continue to read existing fields and assets; new writers introduce no shared schema or data mutation. Existing PluginWarning/session error/event surfaces carry diagnostics; reopening either version requires no migration.
- Ready light check: evidence names the current functions/producer/tests, scope is bounded, decisions delegated by owner are settled; design and completeness light checks completed per owner instruction.

## Constraints

- Follow `ai-docs/manuals/shipped-surface-boundary.md`, `wsflow-mirroring.md`, and `release-compatibility.md` for matching paths.
- Never run local Go tests/builds/exes, ws/ws-mcp, the Pi launcher, or scripts/dev.py (AhnLab V3). Go checks only via approved w5090 sys_lock remote command. Pure node tests must not spawn a launcher. w5090 has no Node.
- Temp files only in `D:/Temp/exec11/`, delete own temporary artifacts at completion. Set TMP/TEMP for producer execution. Minimize touched files.
- Rebuild dist whenever bundle inputs change, in the same commit. Keep spawner.ts and neighboring launcher resolution work untouched when possible.
- No Co-Authored-By trailer. R1 independent review, fixes-only R2 only for substantive findings; minor findings go to Forward. One passing verification cycle closes work.

## Phases

### Phase 1: Harden launch and package artifacts

Implement the five decisions above. Add focused tests for loud fallback, host version decisions, notices and both native hashes. Rebuild deterministically and regenerate mirrors via the existing producer. Verify using pure node checks locally and focused Go tests remotely. Run R1 and substantive-fixes-only R2, record evidence, then close. Deferred: tool retirement, payload architecture redesign, generic launch resolution refactors, live daemon/installed-session rollout.
