# Plan: a thin icon-only send button beside the dashboard composer

## Outcome

The session page composer lost its send button in merge 026a2d622 (keys
only: Enter or Ctrl/Cmd+Enter by the gear menu's input mode). On a phone's
soft keyboard one of send or newline is then unreachable. User decision
(whiteboard (32)): bring send back as a thin, icon-only button on the right
of the composer textarea, on every device, whose height follows the
textarea as it grows ("아이콘 하나 정도만 남게 폭을 줄이고, 대화창이랑 같이
수직으로 성장"). The old wide "send" text label stays gone.

## Settled constraints

- Files: `agents-plugin-tool/internal/wsdashboard/assets/dashboard.{tmpl,css,js}`
  and the template tests (`pages_test.go`) that assert composer markup.
- The button sends exactly as the keyboard path does (same function, same
  IME guard irrelevant for a click); disabled or no-op while the composer is
  empty and has no chips, matching the key path.
- Icon only: reuse the page's existing `icon` template set (add a send/arrow
  icon there if none fits); `title` and `aria-label` "send" for
  accessibility. Narrow width (about one icon plus padding), stretches to the
  textarea's height as it auto-grows; light and dark themes hold.
- The textarea placeholder key hint stays.
- Keep `#cnote` and the input-mode menu unchanged.

## Boundary

Composer markup, styling, and its click wiring only. No route, snapshot, or
adapter change. Return to the lead if the textarea's growth logic must be
rewritten rather than followed.

## Verification

`go test ./internal/wsdashboard/ -count=1` and `go vet ./...` in
`agents-plugin-tool`; a headless Chrome check (as the previous shell delegate
did) that the button is narrow, matches the textarea height after typing
several lines, and sends on click. Report what was not verifiable.
