# Dashboard conversation: hover action pill on chat blocks (copy; dimmed edit/rewind)

## Context

Owner 2026-11-16 18:44: on hover over each conversation block (lead replies
and the owner's prompts) show an action gutter with Copy; prompt edit and
rewind are harness-controlled, so reserve room for them and show them dimmed
for now. Mockup `ai-docs/assets/mockups/261116/chat-actions/chat-actions.html`
(committed 1a122e8fe). Owner pick 18:55 for (1267): "the concept where the
popup appears only on hover" = variant B (floating pill).

## Outcome

1. Variant B as mocked: a small pill on the message block's bottom-right
   edge (positioned relative to the whole message block, outside the bubble,
   since finished bubbles have `contain: paint`), shown on `.msg:hover` /
   `:focus-within`, faint on touch devices; hidden at rest with opacity only
   (stays in tab order, no reflow).
2. Lead reply: Copy only. Owner prompt: Rewind, Edit, Copy (Copy at the right
   end). Rewind/Edit are focusable, icon dimmed, tooltips
   "이 프롬프트 이전으로 되감기 · 나중에 지원" / "프롬프트 수정 · 아직 지원하지 않음"
   (via i18n keys in all 11 locale tables).
3. Copy copies the block's raw text: the lead reply's raw markdown, the owner
   prompt's raw text before display parsing (`parseUserContent`). Feedback:
   green check + "복사됨" for 1.6 s, pill stays visible meanwhile, no duplicate
   toast (`copyText(..., quiet)`). When the stored text is truncated, Copy
   says so (tooltip/label).
4. No pill on a streaming reply or on held/queued messages.

## Decisions (lead, technical)

- Raw text is fetched lazily on Copy from a new read-only dashboard route
  keyed by the conversation entry key, not embedded in the page (keeps page
  weight). Additive route: T0; state the tier in the commit `## AI Context`.
  User batches without their own key need a stable per-envelope identity
  only if required for copy; keep it minimal.
- Add a check icon to `sprite.svg` (the `icon` helper fails on unknown names).

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before editing
  `agents-plugin-tool/`.
- No new CSS class names that collide across modules
  (TestCSSClassesDoNotCollideAcrossModules).
- The gutter markup lives in the cached entry templates (entries merge by key).

## Verification

- Go tests ONLY on remote w5090 (`D:/sys_lock.sh remote w5090 run ...`); NO
  local `go test`, test exes, ws/ws-mcp exes or launcher. Node tests locally.
  Acceptable reds: the two Node-only TestTopicStateBadge* on w5090.
- Tests: route returns raw text for reply and prompt entries (and 404 for
  unknown key); template renders the pill for finished replies and prompts,
  not for streaming/held; Copy handler uses the fetched text.
- LOOK: side-by-side against mockup `#v=b` states (rest, hover agent, hover
  user, copied, focus, narrow 420px), dark, via a page rendered on w5090 or a
  static harness with the real CSS/JS. Temp under D:/Temp/chat-actions-impl/.
- R1 review; fixes-only R2 for substantive findings.
