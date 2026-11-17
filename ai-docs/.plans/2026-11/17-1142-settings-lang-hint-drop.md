# Settings page: drop the floating "browser language fallback" hint card

## Context

Owner 2026-11-17 11:41, with a screenshot
(`C:\Users\user\.cache\ws@kang-sw-devenv\dashboard\sessions\renewaldevoteetranspirefencing\inbox\attachments\99eecc97f9f2fed6-1.png`):

- In the dashboard settings, under the "언어" (language) select set to
  "브라우저 설정 따르기", a hint "브라우저 언어가 없으면 영어" renders as its
  own floating card-like block.
- The owner reads it as a card that does not need to exist.

## Outcome

1. Remove that hint from the language setting: it no longer renders, in
   either UI language.
   - Drop its i18n strings if nothing else uses them.
   - Keep the select and its options unchanged.
2. If the hint is produced by a generic per-setting description mechanism
   (for example a `description` field in a settings manifest), remove only
   this setting's description.
   - Do not change how other settings render their descriptions.
   - If the text comes from `dashboard-config-manifest.json`, edit both the
     ws and wsflow manifests, since they keep the same keys.
   - Say in the report whether such a manifest field crosses versions
     (compatibility tier, with source lines).
3. The "앱 업데이트" section below keeps its current spacing.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing
  `agents-plugin-tool/`. If you touch `agents-plugin-tool/internal/`, also
  read `ai-docs/manuals/release-compatibility.md`.
- Other workers are active in this repo:
  - dashboard JS/CSS visible-only rendering;
  - `wswhiteboard` close changes;
  - daemon caching;
  - desktop-electron gate fixes.
  Keep this change to the language hint.
- NO local `go test`, Go test executables, ws/ws-mcp executables or the
  launcher on this PC (owner AV rule). Do not start the daemon or Electron.
  - Go tests run on remote w5090 only, if you change Go, a template or a
    manifest:
    `D:/sys_lock.sh remote w5090 run lang-hint <min> --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/porthole-diabetic-banjo' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wsdashboard'`.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- LOOK: render the real settings markup and CSS in local headless Chrome
  (static fixture or the existing fixture approach). Capture before and
  after, and check that the gap to "앱 업데이트" looks normal.
- Temp files only under `D:/Temp/settings-lang-hint-drop/`. Keep the two PNGs
  for the lead and delete everything else.
- Commit with `## AI Context` and the Co-Authored-By trailer naming your
  model. Do not push.

## Verification

- The after screenshot shows no hint block under the language select.
- `go test ./internal/wsdashboard` on w5090, if Go, a template or a manifest
  changed.
- Report:
  - the commit;
  - the files changed;
  - where the hint came from;
  - the screenshot paths;
  - the tier, if a manifest was touched.
