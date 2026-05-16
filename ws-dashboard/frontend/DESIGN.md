# ws dashboard visual system

## Product intent

The ws dashboard is an operational control surface for workspaces, work roots,
agents, terminals, editors, viewers, diagnostics, and tasks. It should read like
a dense instrument panel: fast to scan, calm under failure, and optimized for
long-running sessions rather than promotion or brand storytelling.

The visual system is dark-first. Dark canvas, dark panels, hairline separators,
compact spacing, square corners, and restrained state color are the default
contract for new dashboard UI. Light-mode values must not be hardcoded into
components; if a later light theme is added, it should remap semantic tokens
without changing component structure.

## Token vocabulary

Define and consume dashboard-local semantic tokens from `src/styles.css`.
Components should prefer purpose-based names over raw color names.

### Canvas and panels

- `--ws-color-canvas`: app background behind all dashboard surfaces.
- `--ws-color-panel`: default panel surface.
- `--ws-color-panel-raised`: slightly lifted or selected panel surface.
- `--ws-color-panel-subtle`: recessed navigation, viewer, or reserved surface.
- `--ws-color-panel-hover`: hover affordance on rows and neutral controls.
- `--ws-color-panel-selected`: selected row or active scope background.

### Borders and separators

- `--ws-color-border-subtle`: low-emphasis separators and grid lines.
- `--ws-color-border`: default component boundary.
- `--ws-color-border-strong`: interactive boundary or higher-emphasis rule.
- `--ws-border-width-hairline`: the default `1px` separator width.
- `--ws-radius-square`: the default `0` radius. New dashboard UI should stay
  square unless a component has a specific functional need.

### Text

- `--ws-color-text-primary`: primary labels and selected resource names.
- `--ws-color-text-secondary`: status lines and secondary metadata.
- `--ws-color-text-tertiary`: eyebrows, section labels, and timestamps.
- `--ws-color-text-disabled`: unavailable actions and stale placeholders.
- `--ws-font-family-sans`: dashboard sans stack.
- `--ws-font-size-*` and `--ws-line-height-*`: compact type scale for dense
  operational UI.

### Actions, state, and focus

- `--ws-color-action`: primary action, active indicator, and selected rail.
- `--ws-color-action-hover`: action hover/focus boundary.
- `--ws-color-action-text`: text on primary action fills.
- `--ws-color-state-success`: online, healthy, and completed state.
- `--ws-color-state-info`: loading, refresh, and neutral progress state.
- `--ws-color-state-warning`: stale, pending, or degraded-but-usable state.
- `--ws-color-state-error`: failed, inaccessible, or blocking state.
- `--ws-color-focus`: keyboard focus outline.
- `--ws-focus-ring`: default inset focus ring.

## Typography and density

Use small, legible type with clear hierarchy rather than large display text.
Operational rows should remain compact enough for long workspaces while leaving
space for badges and status. Prefer the token scale:

- Eyebrows and section labels: `--ws-font-size-01`.
- Metadata, badges, and command log rows: `--ws-font-size-02`.
- Row titles, panel titles, buttons, detail values: `--ws-font-size-03`.
- Status pane titles and viewer titles may use `--ws-font-size-04` or
  `--ws-font-size-05` when a larger empty-state anchor is needed.

Keep line heights tight but not clipped. Avoid extra outer gutters; use
hairlines and alignment to separate regions.

## Component rules

### Buttons

Buttons are square, compact, and explicit. Neutral buttons use panel surfaces
with action-colored text. Primary buttons use the action fill only for high-value
operations such as refresh after failure. Disabled buttons keep their footprint
and use disabled text, not reduced opacity that harms contrast.

### Headers

Panel headers are functional toolbars. Keep them short, left-align title and
state, and reserve the right edge for actions. Headers use a single bottom
hairline instead of shadows.

### Navigation rows

Rows are full-width, left-aligned, and selected with both a background and a
left action rail. Indentation communicates hierarchy; do not add cards for each
resource. Hover should be subtle and never stronger than selected state.

### Badges and chips

Badges are inline, square, and low-height. Use border and text before filled
color. State dots use semantic state tokens and should remain small enough not
to dominate labels.

### Notices and errors

Inline notices use a left rail plus muted surface. Error text should be legible
without turning the entire panel into a bright alert. Reserve strong error color
for borders, dots, and critical text.

### Future split-group rows

Future terminal, agent, editor, viewer, diagnostics, and task split groups
should reuse row density, selected rails, hairline separators, and semantic state
tokens. Split-group UI should not introduce rounded cards, gradient panels, or
heavy drop shadows.

## Constraints

- Do not build rounded, card-heavy marketing layouts in dashboard surfaces.
- Do not use gradients, decorative shadows, glass effects, or light-mode default
  fills for core panels.
- Do not hardcode raw colors in new components when a semantic token exists.
- Do not change command identifiers, resource API behavior, or routing identity
  to satisfy visual work.
- Keep this guide and token layer dashboard-local under `ws-dashboard/frontend/`.
