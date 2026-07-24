---
related:
  - 260722-feat-dashboard-settings-panel: parent surface — this section registers into that panel's Phase 1 section registry (now .done)
  - 260722-feat-dashboard-hotkey-config-framework: binding registry this editor reads/writes (done)
related-mental-model: ws-web-dashboard
parent: 260710-epic-ws-dashboard-terminal-ux-polishing
---

# feat: Dashboard hotkey-rebind editor settings section

Split from `260722-feat-dashboard-settings-panel` Phase 2 (a deferred
forward-reference) at the point that ticket closed with its Phase 1 slice
delivered. Captured here so the future scope stays discoverable in the live
backlog instead of only inside a `.done` ticket.

## Context

The general-purpose Settings panel (settings-panel Phase 1, done) ships a modal
shell, a section registry, a shared namespaced preferences store, and a
Terminal-style section. Phase 2 of that ticket was always a deferred
forward-reference: a hotkey-rebind editor section that reads/writes through the
`260722-feat-dashboard-hotkey-config-framework` binding registry and registers
into the settings section registry.

## Scope (rough)

- Add a "Hotkeys" (or similar) section registered into the Phase 1 settings
  section registry.
- Render current bindings from the hotkey-config-framework binding registry and
  allow rebinding leaf commands.
- Persist overrides through the shared namespaced preferences store and feed
  them back into the binding registry so live dispatch reflects edits.
- Respect the terminal-passthrough guard and leader-key semantics.

## Open questions

- Rebind-capture UX (press-to-bind vs. text entry), conflict handling, and a
  reset-to-default affordance.
- Whether the binding registry currently exposes a write/override path or needs
  one added.

Needs a sage design review before promotion out of idea/.
