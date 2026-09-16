---
title: "Refine Pi live-agent and footer status UX"
---

# Refine Pi live-agent and footer status UX

## Background

Live Pi dogfooding exposed two compact-status presentation improvements. The live-agent gutter separates total elapsed time from the related active-time value, while the custom footer replaces Pi's built-in footer but currently shows only the host-provided Git branch. This ticket keeps both improvements together as a focused Pi status-UX pass.

The footer must remain cheap to render. Git work belongs behind an in-memory cache and lifecycle-driven refreshes, never in the render path or synchronously on the user-input path.

## Decisions

### Live-agent duration layout

Current shape:

```text
gutter-probe | worker | running | 1m | gpt-5.6-luna (high) | 17k | active 25s | $0.33
```

Desired shape:

```text
gutter-probe | worker | running | 1m (25s) | gpt-5.6-luna (high) | 17k | $0.33
```

- Relocate the existing active-time entry from its separate trailing position; do not delete or replace that entry.
- Remove only its `active ` prefix and render the same active-time value immediately after total elapsed time, enclosed in parentheses as `<total-time> (<active-time>)`.
- Preserve the active-time display's current color and other styling after it moves.
- Keep the existing `worker` label unchanged.

### Footer Git status

- Keep the current custom-footer scope: TUI lead and fork sessions only.
- Append compact Git status immediately after the existing cwd and parenthesized branch display:

  ```text
  ~/devenv (develop) merging ↑1 ↓2 +12 -3 ~2 ?1
  ```

- Follow `shell/statusline.sh` for the compact counters and their semantic colors:
  - `↑N`: commits ahead;
  - `↓N`: commits behind;
  - `+N`: unstaged added lines;
  - `-N`: unstaged deleted lines;
  - `~N`: unstaged changed paths;
  - `?N`: untracked paths.
- Do not add a staged-only indicator.
- When no repository operation is active and every compact counter is zero, append no Git-status suffix; do not render `working tree clean`.
- Place the repository operation state directly to the right of the branch:
  - render `conflicting` in red whenever unmerged paths exist; it overrides any operation label;
  - otherwise render `merging`, `rebasing`, `cherry-picking`, or `reverting` in yellow when that operation is active.
- When width is insufficient, omit the Git-status indicators as a group while preserving cwd and branch.

### Git-status refresh and cache

- Hold Git status in an in-memory cache keyed by working directory.
- Request an asynchronous refresh after every `turn_end`, not only at `agent_settled`. Coalesce overlapping requests so only one Git query per working directory is in flight.
- While the lead is idle, request a low-frequency refresh every five minutes.
- If user input arrives when an idle refresh is due or pending, defer that idle refresh for 30 seconds. Further input restarts the full 30-second delay, and the refresh runs only if the session is still idle when the delay expires.
- Never await Git from the user-input path. Rendering reads only the cache and requests a re-render when a refresh publishes a new value.
- Bound Git execution with a short timeout. On a transient failure, retain the last successful cached value; outside a Git repository, render no Git-status indicators.

## Constraints

- The live-agent change is a presentation-only relocation of the existing active-time entry: do not change total-time or active-time accounting semantics.
- Do not move, remove, rename, or restyle any unrelated live-agent gutter field.
- Preserve all existing footer content and extension-status behavior; this ticket augments the cwd/branch area rather than replacing another field.
- Do not perform Git, filesystem, history, or registry traversal from the footer render function.
- Do not introduce high-frequency polling or filesystem watchers; refreshes are limited to the confirmed lifecycle requests and the single low-frequency idle timer.
- Keep cached rendering bounded and responsive across reload and shutdown.

## Prior Art

- `shell/statusline.sh` defines the confirmed compact Git counters and color intent. Its `+` and `-` values are unstaged line totals, while `~` and `?` are path counts; it is not a staged/conflict contract.
- `agents-plugin-pi/src/agent-footer.ts` owns the current custom footer and cached render path.
- `agents-plugin-pi/src/index.ts` owns Pi lifecycle and input-hook registration.

## Phases

### Phase 1: Relocate the live-agent active-time entry

Update the Pi live-agent gutter to produce the confirmed duration layout. Preserve the existing active-time value, accounting, and style; only its position and `active ` prefix change.

### Phase 2: Add cached Git status to the custom footer

Add the confirmed counters and repository-operation state beside the existing cwd/branch display. Populate a per-working-directory cache through asynchronous `turn_end` refreshes and the debounced five-minute idle refresh, preserving an O(1) cache-only render path and non-blocking input handling.
