---
title: "P2: 고해상도 전체 화면에서 지속되는 Electron 지연 원인 진단"
related:
  261115-feat-werkschaft-electron-shell: "preserve shell security and navigation contract during A/B diagnostics"
  261113-feat-ws-dashboard-frontend-performance-profile-and-optimize: "shared rendering and input candidates are tracked separately from native presentation"
  261116-bug-ws-dashboard-hidden-cached-page-cost: "shared cached-page overhead, not an established Electron-only cause"
---

# P2: Diagnose persistent Electron lag at high-resolution fullscreen

## Background

Dashboard performance finding P2, 2026-11-16. The owner reports severe Electron
lag at fullscreen 3440x1440 that persists across session switches until app
restart. At 1600x900 it is fine; Chrome is fine at fullscreen. Size: Phase 1 S
(diagnostics); corrective work sized after evidence. No visual change is intended.

Evidence source read for this capture: `D:/Temp/dash-scroll-perf/SUMMARY.md`,
2026-11-16 investigation. Durable facts and limitations are copied here because
the temporary summary is not a repository artifact:

- Windows 11, external 3440x1440 ~100 Hz, 125% scaling; RTX 4050 plus Intel UHD.
  Installed app 0.50.0 / Electron 44.11.3 / Chromium 152.0.7977.169.
- `agents-plugin-tool/desktop-electron/main.js:49-90` uses BaseWindow plus two
  WebContentsViews and an overlay on Windows, unlike macOS BrowserWindow.
- A fresh warmed instance reported hardware compositing, RTX 4050 / ANGLE
  D3D11, DirectComposition, not SwiftShader. Very early software status was
  transient initialization, not evidence of permanent fallback.
- The sampled owner Electron was occluded by Edge, so its low measured GPU
  activity is not comparable with the owner's exposed lag. No valid native
  Electron wheel/frame-time reproduction or aged GPU-fallback evidence exists.
- The investigation's browser control used Edge/Chromium 157, not the exact
  Electron runtime. Browser comparisons therefore do not isolate embedding.

## Reproduction

With separately authorized isolated profiles, warm the same session/workload
and compare 3440x1440 with 1600x900, recording actual client pixel area/DPR,
exposure, time since startup and GPU status. Repeat session switches and an app
restart. Compare secure plain BrowserWindow with the existing composed window
on the same runtime, then compare Electron runtimes with embedding fixed.
Do not activate, resize, reload or close the owner's windows for capture.

## Candidate fix directions

First add diagnostics for `child-process-gone` (especially GPU), GPU feature
status snapshots over time via `app.getGPUFeatureStatus`, warmed renderer/frame
and pixel-area measurements. Then use matched embedding/runtime A/B results to
choose a correction. Do not blindly disable the GPU, force
`ignore-gpu-blocklist`, or claim snapshot-iframe accumulation as the cause.

## Decisions

Owner priority update (2026-11-16 10:05): performance, including this
perf-render diagnostic track, is top priority in parallel with exe-launch
elimination. The P2 label preserves the original finding identity; it does not
override this scheduling directive. No implementation starts without the
owner's explicit assignment.

Compatibility tier: **n/a** for local diagnostic and embedding/runtime
investigation with unchanged daemon protocol, stores and configuration manifests.
No rendering policy or runtime pin is selected. This is a performance-only
change: preserve appearance and behavior, sandbox/context isolation, exact-origin
navigation and request guards, approvals, drafts and focus.

## Open questions

- Is the persistent state a compositor/context problem, embedding sensitivity,
  runtime-version issue or shared dashboard work?
- What matched exposed fresh-versus-aged capture distinguishes these causes?
- Which runtime/embedding correction, if any, is justified by Phase 1?

## Phases

### Phase 1: Capture matched embedding, runtime and GPU-lifecycle diagnostics

Add bounded GPU process-loss/status diagnostics and perform the plain
BrowserWindow and Electron-runtime A/B comparisons above. Retain security guards
in every diagnostic variant. Record warmup, exposure, resolution, workload,
source/runtime pins, profiles and limitations. No root cause or fix is implied
by the existing measurements.

### Phase 2: Apply the evidence-selected correction

Depends on Phase 1's landed Result and an explicit correction decision. Implement
the smallest measured fix, rerun matched high/low-resolution and aged/session-
switch controls, and demonstrate unchanged visuals, security and input behavior.
No GPU-policy change is preauthorized. This docs capture runs no builds, tests,
Electron instances or live-window operations.
