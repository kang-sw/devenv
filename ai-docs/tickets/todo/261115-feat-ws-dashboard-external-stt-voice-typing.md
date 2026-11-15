---
title: "Dashboard voice typing through an external transcription provider"
related:
  261115-feat-werkschaft-electron-shell: "EXEC4 owns the Electron shell; coordinate scoped microphone acceptance without changing its shell contract here"
---

# Dashboard voice typing through an external transcription provider

## Background

MAIN's 2026-11-15 02:01:51 peer-lead message relays owner approval
(872)=a and the owner's streaming addition. Add push-to-talk transcription
at the dashboard composer through a daemon-mediated external provider.
This is accepted backlog capture, not provider validation or implementation.

## Decisions

- Use one OpenAI-compatible multipart transcription client:
  `POST {base}/audio/transcriptions`, with an audio file, model selection and
  language selection. Provider-specific correctness must be established from
  exact official documentation before ready promotion; the proposed presets
  below are attributed inputs to that verification, not verified API facts.
- Keep the provider key in daemon configuration. The settings API returns
  only a masked representation and never exposes the key to LAN viewers.
  Audio travels renderer -> local daemon -> provider. Browser-direct calls
  and a CORS workaround are rejected.
- Push-to-talk records with `MediaRecorder` as webm/opus. Stopping recording
  uploads the audio, then inserts the final transcription at the composer
  caret. This ticket does not choose partial insertion/replacement behavior.
- The owner's addition, verbatim: "중간중간에 만들어지는 텍스트를 스트리밍으로 돌려주는 식이면 그것도 표기".
  English translation: if intermediate text is returned as a stream, display
  it too. Provider/model pairs that actually support streaming show partial
  text as it arrives; non-streaming pairs insert final text. MAIN's example
  of OpenAI gpt-transcribe with `stream=true` is unverified, not a capability
  guarantee. Realtime WebSocket transcription is outside this contract.
- Browser microphone acceptance includes secure loopback
  `http://127.0.0.1`; LAN HTTP origins hide or disable the microphone control.
  Electron acceptance requires `NSMicrophoneUsageDescription` through packager
  `extendInfo`, `systemPreferences.askForMediaAccess`, and session permission
  handling restricted to the daemon origin. Coordinate with EXEC4's shell
  work; this approval does not broaden native authority for other origins.
- Compatibility tier: **T0 additive** for new daemon routes/configuration and
  dashboard behavior. Exact new route names and manifest/configuration fields
  are not selected by this capture. No existing API, protocol or store
  lifecycle is replaced.

### Provider verification prerequisites

The following are MAIN-supplied presets/examples, retained as requests to
check before ready promotion. Neither these examples nor the reference
pointers were checked against external documentation in this offline capture.

| Requested preset | MAIN-supplied base/model or field claim | Official reference pointer supplied by MAIN |
|---|---|---|
| OpenRouter | Base `https://openrouter.ai/api/v1`; suggested `openai/whisper-large-v3` or `openai/gpt-4o-mini-transcribe` | `openrouter.ai/docs/guides/overview/multimodal/stt.md` |
| OpenAI | Described as `gpt-transcribe`; possible languages-array rather than `language` branch; possible `stream=true` | OpenAI speech-to-text guide |
| Groq | Base `https://api.groq.com/openai/v1`; suggested `whisper-large-v3-turbo` | Groq STT docs |
| Custom (OpenAI-compatible) | User-supplied base URL and model | Exact selected endpoint's official API documentation |

Before promotion, establish exact supported model IDs, multipart field names,
language handling, audio-format acceptance and streaming request/response
capabilities from official model/API documentation. Report contradictory or
missing evidence to the lead rather than choosing a different provider
contract. No OpenAI base URL, languages-array field name or streaming event
format is inferred here.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`).
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`).
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/` if configuration wiring touches it).
- If implementation adds package configuration manifests, apply
  `ai-docs/manuals/wsflow-mirroring.md` to the affected shared/wsflow surface;
  this capture selects no manifest field or mirror change.
- Outside scope: realtime WebSocket, Deepgram/ElevenLabs/AssemblyAI adapters,
  chat-completions `input_audio`, diarization/timestamps, splitting audio over
  25 MB, and OS keychain integration. No audio save/retention policy is approved
  by this capture.
- Preserve the Electron ticket's exact-origin pin, isolated renderer
  (`contextIsolation`, sandbox, no Node integration or preload), credential-free
  attach and no-daemon-replacement boundaries. Its Decisions select plain
  CommonJS in `agents-plugin-tool/desktop-electron/` and `@electron/packager`;
  that ticket and its implementation remain EXEC4-owned.
- This documentation lane runs local source inspection and ticket/diff guards
  only. No builds, tests, browsers, providers, network, listeners or daemon
  operations are authorized. TASK3 stays first; release and MAIN handoff remain
  with the root lead.

## Prior Art

Static inspection at `40f96ea17` found composer selection handling in
`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js` (search
`selectionStart` and `setRangeText`), and existing dashboard configuration
ownership in `agents-plugin-tool/internal/mcp/dashboard_config.go`
(`DashboardSettings`). These are integration pointers, not evidence of an
existing transcription client or secret-safe settings API.

The shell Decisions in `261115-feat-werkschaft-electron-shell` require the
exact discovered daemon origin, no preload/native page authority and plain
`@electron/packager`. Read that ticket's current decisions with EXEC4 before
shell work; this capture does not edit it or claim microphone support exists.

## Phases

### Phase 1: Daemon-mediated composer voice typing with supported partial text

Implement the approved push-to-talk, provider settings, masked-key handling,
local-daemon upload and caret insertion contract after the provider evidence
prerequisites are satisfied. Show streamed partial text only for verified
supporting pairs, retaining final-only behavior otherwise. Coordinate scoped
Electron permissions with EXEC4 without implementing a second frontend or
loosening origin isolation. Return any required unapproved partial-text or
retention mechanism to the lead before selecting it.

Future verification must cover multipart request shape and actual streaming
capability shape for selected provider/model pairs; renderer -> daemon ->
provider mediation; masked settings and no key leakage to LAN viewers;
secure-context controls; final insertion at the composer caret;
non-streaming fallback; partial text display where supported; and packaged
Electron microphone purpose text, OS permission handling and denial for
non-daemon origins. Use isolated fixtures, never the shared daemon or port
29417. This capture asserts no provider integration or executable acceptance.
