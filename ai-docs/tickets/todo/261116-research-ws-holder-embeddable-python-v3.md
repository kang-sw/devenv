---
title: "Experiment: launch the session holder with the official Windows embeddable Python under AhnLab V3"
related:
  261112-research-ws-ade-desktop-shell-and-rename: "owner decision (494)=a chose a Go holder port as the long-term direction; this experiment may remove the need to wait for it on Windows"
  261113-bug-ws-only-the-dashboard-daemon-runs-an-exe: "the one-executable principle: only the daemon runs an exe because V3 prompts on new executables"
  261109-research-windows-code-signing-signpath: "signing track that a Go holder binary would need first"
---

# Experiment: launch the session holder with the official Windows embeddable Python under AhnLab V3

## Background

The session holder is a Python program (`agents-plugin-tool/internal/wsholder/holder.py`, about 1370 lines; pty on POSIX, ctypes ConPTY on Windows), started detached by the dashboard daemon (`agents-plugin-tool/internal/wsholder/manager.go`). The launcher it sits beside is also Python (`agents-plugin-pi/bin/ws-mcp-launcher.py`, about 2250 lines). Both need a `python3` on the host.

The project deliberately avoids vendoring or frequently launching unknown executables. AhnLab V3, common on Korean corporate PCs, tends to flag unknown bundled executables, and that is the origin of the one-executable principle (see `261113-bug-ws-only-the-dashboard-daemon-runs-an-exe`; V3 behavior is also recorded in `261109-research-windows-code-signing-signpath`).

An earlier owner decision, whiteboard item (494)=a, chose a Go holder port as the long-term direction. It is not implemented, and a Go holder binary would need code signing first.

Owner decision (1197)=a: run an experiment that launches the holder with the official Windows embeddable Python distribution (the `python.exe` inside the embeddable zip) on a PC that has V3, and see whether V3 reacts.

Owner note (gist: this goes ahead as option a, but it is only recorded as a todo ticket for now, because it can only be run on the owner's office PC, which has V3; and the owner has tested many Python versions on that PC over the years and has never seen V3 react to Python):

> a로 가긴 할 건데, 일단 이건 todo 티켓으로 적어만 둡니다. 제가 회사 출근해야 먹을 수 있는 개밥이거든요. 근데 생각해보면 지금까지 회사에서 무수한 python 버전을 제 PC에서 테스트했었는데 지금까지 python에서 v3 띄우는 건 못 보긴 했어요.

## Experiment protocol

Run on the owner's V3 PC; the lead cannot run it. Per the repo's hard Windows rule, do not build or test Go locally on that machine.

1. **Record the environment first.** V3 product and engine version, engine/signature update date, Windows version, whether cloud (reputation) detection is enabled.
2. **Download** the official "Windows embeddable package (64-bit)" zip for a currently supported CPython 3.x from python.org, and the matching release's published checksum/signature to confirm authenticity. Extract into a fresh empty directory; no installer is run.
3. **Fresh-file observation.** Note what V3 does at download and at extraction (real-time scan, prompt, quarantine). Also note whether SmartScreen shows a warning for the zip or for `python.exe` on first run (mark-of-the-web present vs removed).
4. **Launch the holder** with that interpreter: start the daemon in the normal way but with the holder's interpreter pointed at the extracted `python.exe` (or run `holder.py` directly with it using the same arguments the manager passes; read `manager.go` for the exact command line). Exercise a real hosted session and a terminal so the ConPTY path (ctypes) runs. Note: embeddable Python ignores `PYTHONPATH` and uses a `._pth` file, so check that the holder's imports resolve.
5. **Observe**, for each launch, and again after a delay:
   - V3 real-time detection (prompt, block, alert dialog) on first launch and on repeated launches.
   - Cloud detection delay: re-check after minutes, hours, and a day, since cloud verdicts can arrive late and quarantine the file afterwards.
   - Quarantine or deletion of `python.exe`, the DLLs, or `holder.py`.
   - SmartScreen reaction.
   - Whether behavior differs when the holder is spawned detached by the daemon versus started by hand.
6. **Repeat** after copying the extracted directory to a second location and under a renamed folder, to learn whether detection is path- or reputation-sensitive. Optional: repeat with a second Python minor version.

## Success and failure criteria

- **Pass:** no V3 real-time reaction, no quarantine, and no SmartScreen block across the observation window above (including the late cloud-verdict check), for repeated detached launches with working ConPTY sessions.
- **Fail:** any V3 prompt, block, quarantine, or deletion attributable to the embeddable Python files, or a SmartScreen block that users cannot reasonably click through.
- **Inconclusive:** the holder does not run under the embeddable layout for a non-V3 reason (for example import resolution); fix that and rerun before judging V3.

## What each outcome implies

- **Pass:** bundling the official embeddable Python with the Windows desktop app is viable, so the Windows desktop app could run standalone without first waiting for code signing. The Go holder port stays a long-term option, not a blocker. A follow-up ticket would design the bundling (pinned version, checksum, update path).
- **Fail:** the Go holder port ((494)=a) plus the code-signing track become the path for Windows; the holder keeps requiring a user-installed Python until then.

## Outcome Ledger

### Verified Findings

- The holder is Python (`agents-plugin-tool/internal/wsholder/holder.py`, 1374 lines at drafting) and the launcher is Python (`agents-plugin-pi/bin/ws-mcp-launcher.py`, 2258 lines at drafting).
- The one-executable principle and the V3 prompting-on-new-executables concern are recorded in existing tickets (`261113-bug-ws-only-the-dashboard-daemon-runs-an-exe`, `261109-research-windows-code-signing-signpath`).
- Owner report: many Python versions have been tested on the owner's V3 PC over time without V3 reacting. This is anecdotal and does not cover the embeddable zip launched detached by the daemon.

### Confirmed Decisions

- (1197)=a: run the embeddable-Python-under-V3 experiment; captured as a todo ticket and executed when the owner is at the V3 PC.

### Proposals

Non-authoritative (lead's prior opinion):

- Do not add a Lua runtime.
- The official embeddable Python might pass V3, but this is unverified.
- If it passes, the Windows desktop app could run standalone without waiting for signing.

### Open Questions

- Which Python minor version to test and later pin.
- Whether the embeddable distribution's lack of `pip` and its `._pth` import rules require changes to how the holder or launcher resolves modules.
- Whether the launcher (also Python) should move to the same bundled interpreter if this passes.
- How long the cloud-detection observation window must be to count as a pass.

### Rejected Alternatives

None recorded.
