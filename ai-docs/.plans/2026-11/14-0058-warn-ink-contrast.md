# Light-theme warn-ink contrast on three surfaces (ticketless UX fix, (527)=a)

Source: the delete-modal minors QA measured these other `--wb-warn-ink` on a
warn-tint pairs below WCAG AA 4.5:1 in light theme. EXEC approved a
ticketless fix on 2026-11-13.

| rule | `dashboard.css` | measured light |
|---|---|---|
| `.chip.mcpbad` | ~1846 (warn 16% tint) | 4.20:1 |
| `.tm-gap` | ~695 (warn 12% tint) | 4.39:1 |
| `.hdrow.lost .hdcause` | `.hdcause` ~987 (on the lost row's background) | 4.49:1 |

## Required outcome

- Each of the three texts reaches at least 4.5:1 in light theme. Dark theme
  is no worse than today for each.
- Keep the warn look: amber ink on an amber tint.
- Follow the precedent the delete modal set on `fix/ux-delete-modal-minors`
  (9daebb982, not yet on develop): a scoped ink of
  `color-mix(in srgb, var(--wb-warn-ink) 80%, var(--wb-fg))`. Use the same
  form, adjusting the mix only if a surface needs more, and say so.
- Scope each change to its rule. Do not change the global `--wb-warn-ink` or
  `--wb-warn` tokens.
- For `.hdcause`, check what background the lost row actually paints, and
  whether `.hdcause` appears elsewhere on a different background. Change only
  the failing case if the others pass.

## Boundary

- Touch `agents-plugin-tool/internal/wsdashboard/assets/dashboard.css` only,
  those three rules (or a new scoped rule next to each). No layout change, no
  JS, no i18n.
- Do not edit the `.rdeldlg` rule: that is on another branch.
- Do not touch the shared daemon:
  - No `scripts/dev.py`.
  - Kill nothing you did not start.
  - Never contact 127.0.0.1:29417.
- Contrast can be computed from the resolved token values (both themes'
  `--wb-*` definitions in the CSS) with a small script. A headless render on
  your own daemon is optional: build `ws-mcp` to `/tmp` and run
  `WS_CACHE_HOME=<tmp> WS_CONFIG_HOME=<tmp> HOME=<tmp> <bin> dashboard --port 0`,
  and stop it by PID when done.

## Verify

- A table of foreground, background and contrast ratio for each surface,
  light and dark, before and after.
- `go test ./internal/wsdashboard/...` from `agents-plugin-tool`. The CSS
  collision guard must stay green.
