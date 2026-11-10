# Tracker avatars colored by org relation to the viewer

## Request

User (whiteboard, 2026-11-10), about the tracker's person avatars (`<i class="av">`),
which today are all the same blue: color them by hierarchy and relation. Bosses in a
yellow-to-red family by rank, subordinates in a green family, and people with no
relation in plain gray.

## Outcome

Every person avatar in the tracker UI gets a relation class computed from the viewer's
position in the org tree:

- self: unchanged (`.av.me`, primary color).
- superiors (the viewer's lead chain, `VS.members[id].leads[0]` walked upward): warm.
  Direct lead amber/yellow, two levels up orange, three or more levels up red.
- subordinates (`VS.viewer.reach[id]` = level below the viewer): green. Direct reports
  the strongest green, deeper levels progressively lighter/softer green (cap at 3 steps).
- everyone else (peers, other branches, removed or unknown members): neutral gray.
- `pend` (pending assignment) keeps its outline style, with the outline/text in the
  relation color instead of teal; `more` (+N) stays muted.
- The avatar `title` (tooltip) names the relation, e.g. "Your lead", "2 levels up",
  "Direct report", "Level 2 report"; add i18n keys to every table under
  `internal/wsi18n/tables/` (Korean in `ko.json`; every table holds every English key).

## Scope

- `agents-plugin-tool/internal/wsdashboard/assets/tracker.js`: one helper (for example
  `avRel(id)` returning the class suffix and tooltip) used by every site that renders
  `<i class="av">` for a person (grep `class="av`), so all of them stay consistent.
- `agents-plugin-tool/internal/wsdashboard/assets/tracker.css`: relation color tokens
  for light and dark themes, readable contrast for the 2-char initials.
- `ai-docs/ref/tracker-mockup/i18n-en.js` if it mirrors tracker i18n keys (it did for the
  quick-add chips).
- Not in scope: other dashboard avatars outside the tracker, any server-side change.

## Verification

- `nice -n 19 node --check` on tracker.js; i18n table completeness test and the
  wsdashboard asset tests run remotely only:
  `~/.local/bin/ws-rgo -C <worktree>/agents-plugin-tool -m 30 -- test ./internal/wsi18n/... ./internal/wsdashboard/... -count=1 -timeout 30m`.
  Never run go build/test/vet locally (it freezes the user's machine).
- Never touch the user's daemons on ports 29417 / 45743.
