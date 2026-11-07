# Plan: dashboard session page shell tidy-up (composer keys, fixed split, lead rows)

## Outcome

User-requested changes to the live session page
(`agents-plugin-tool/internal/wsdashboard/assets/dashboard.{tmpl,js,css}`,
`pageview.go`/`pages.go` where regions are declared). Today the agents panel
grows and shrinks with content and pushes the conversation around; the lead's
status appears three times; the composer spends width on copy and send
buttons.

1. **Composer buttons.** Remove the copy button (`#ccopy`) and the send button
   (`#csend`) from the composer. The composer exists only with a live adapter,
   so send covers copy; the board's own copy-answers line stays. Keep the
   `#cnote` status line (it reports send results, held state, errors). Put a
   short key hint in the textarea placeholder that follows the input mode.
2. **Input mode.** Add an "Input mode" group to the existing gear menu
   (`#gmenu`, beside Layout), saved in localStorage like the layout choice:
   - default "Enter sends": Enter sends, Shift+Enter inserts a newline;
   - "Ctrl/Cmd+Enter sends": Enter inserts a newline (today's behavior).
   In both modes an Enter while an IME composition is active
   (`e.isComposing` or keyCode 229) never sends — Korean input must not
   send a half-composed syllable.
3. **Fixed split.** In the live layout, the agents panel (tree region) and the
   conversation region share the column at a fixed ratio, default 1:3
   (agents:conversation), each scrolling internally; content changes never
   resize either. A draggable divider between them adjusts the ratio, clamped
   so neither collapses (reasonable minimums), saved in localStorage and
   restored on load. Region swaps over SSE must keep the ratio and each
   panel's scroll position (the page already preserves view state across
   swaps; follow that mechanism). The tabs layout and narrow screens keep
   working (in tabs mode the split applies inside the conversation tab).
   This replaces the old tree height cap.
4. **Lead status de-duplication.** Remove the top lead status line (the
   `lead` region rendered above `#grid`) and its SSE region wiring. Compact
   the lead root row in the agents tree to one line: phase, the running or
   permission tool name with its arg preview, and the elapsed time; its
   finished batches stay behind the existing disclosure. The approval wait
   must stay visible after this change: the conversation's `permwait` line
   and the tree's permission row already show it — verify both still render
   in the permission phase (user decision: an approval indicator is enough,
   handling stays in the terminal).

## Settled constraints

- No new routes, no server-side state; all preferences are browser-local.
- Follow the page's existing patterns (gear menu buttons with `aria-pressed`,
  localStorage keys under the `ws.dashboard.` prefix, region swap handling).
- Light and dark themes both hold.
- Update tests that assert removed markup (`pages_test.go`, `pageview_test.go`
  and friends) and add assertions for the new markup where tests already
  cover the template; JS behavior has no harness here — say what you
  verified by hand or headless and what you could not.

## Boundary

Touch only the session page assets and the Go code that declares or renders
its regions. Return to the lead if a change needs a new route, a snapshot
schema change, or an adapter change. Another delegate is concurrently adding
markdown rendering to conversation lead replies (entry/reply bubbles in
`r-conversation` and the Go that builds them); do not restyle message bubbles
beyond what the split needs.

## Verification

`go test ./internal/wsdashboard/ -count=1`, `go vet ./...`,
`go test ./... -count=1` in `agents-plugin-tool`. If a headless browser is
available, load a rendered page fixture and check the split and key handling;
otherwise report it as unverified for the lead's live check.
