---
title: "Electron shell: decide on the default hunspell dictionary download"
related:
  261115-feat-werkschaft-electron-shell: "the shell whose session defaults trigger the download"
---

# Electron shell: decide on the default hunspell dictionary download

## Background

Whiteboard topic `ship-electron` (Electron 배포). In the v0.50.0 Windows smoke
on a fresh profile, `ko-3-0.bdic` (11 MB) appeared in the shell's private
profile directory. When spellcheck languages are set, Electron/Chromium
downloads hunspell dictionaries from Google's CDN by default. The shell
configures no spellcheck behaviour: `agents-plugin-tool/desktop-electron/main.js`
sets no `spellcheck` web preference and makes no `session` spellchecker
calls, so Chromium's defaults apply. The download therefore happens without
the user asking for it and contacts a third-party host from a local tool.
On macOS, Electron normally uses the OS spellchecker, so the download is
expected mainly on Windows (and Linux); the worker confirms this.

## Open Decisions

- Choose one:
  - (a) Keep spellcheck, but point the download elsewhere or block it with
    `session.setSpellCheckerDictionaryDownloadURL` (for example a local or
    unreachable URL), so the shell makes no Google CDN request.
  - (b) Turn spellcheck off in the dashboard view (`spellcheck: false` web
    preference, or an empty language list).
  - (c) Bundle the needed dictionaries with the app and serve them locally
    through the download URL.
  - (d) Keep the Chromium default and document it.
- If spellcheck stays: which languages (OS locale only, or the dashboard's
  ten UI languages)?

## Phases

### Phase 1: Apply the chosen spellcheck policy

Acceptance:

- On a fresh Windows profile, the observed network and profile contents
  match the chosen option. For (a)-(c), no request goes to the Google
  dictionary CDN and no `.bdic` file is downloaded from it.
- The Korean composer still works with the chosen option, including IME
  input and, if spellcheck stays, its underline.
- macOS behaviour is checked and recorded.
