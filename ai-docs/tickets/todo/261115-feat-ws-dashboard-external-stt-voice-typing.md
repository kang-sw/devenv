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
  language selection. Apply the provider/model-specific fields and streaming
  boundaries documented below; shared endpoint shape does not imply identical
  capabilities.
- Keep the provider key in daemon configuration. The settings API returns
  only a masked representation and never exposes the key to LAN viewers.
  Audio travels renderer -> local daemon -> provider. Browser-direct calls
  and a CORS workaround are rejected.
- Push-to-talk records with `MediaRecorder` as webm/opus. Stopping recording
  uploads the audio, then inserts the final transcription at the composer
  caret. Streamed previews follow the separate owner (877) decision below.
- The owner's addition, verbatim: "중간중간에 만들어지는 텍스트를 스트리밍으로 돌려주는 식이면 그것도 표기".
  English translation: if intermediate text is returned as a stream, display
  it too. MAIN's official-doc research now supports OpenAI `gpt-transcribe`
  with `stream=true` [O1][O2]. Realtime WebSocket transcription remains outside
  this contract.
- **VERIFIED:** MAIN's 2026-11-15 02:15:58 relay confirms owner (877)=a.
  For OpenAI streaming models only, show streamed partial text in the input
  as a dimmed preview. Replace that preview wholesale with the final transcript
  on done; no partial text is committed. Preserve the original final insertion
  at the composer caret. OpenRouter and Groq remain final-only; OpenAI support
  is model-specific as documented below. Concurrent caret/selection mutation
  behavior is not selected by this decision.
- **VERIFIED:** The same MAIN relay confirms owner (878)=b. Keep only the
  last recording in memory for a resend button after failure. Never write it
  to disk; drop it on a new recording or window close.
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

### Provider evidence and field boundaries

The following **VERIFIED** bullets record MAIN's official-doc research,
retrieved 2026-11-15 and read in full from the resent report. They are
attributed documentation findings, not independent retrieval or runtime
acceptance. MAIN made no provider calls; `platform.openai.com` returned 403,
so its OpenAI evidence came from `developers.openai.com`.

- **VERIFIED:** OpenAI accepts Bearer-authenticated multipart
  `POST https://api.openai.com/v1/audio/transcriptions` with `file` and `model`
  [O1]. Its listed models are `gpt-transcribe`, `gpt-4o-transcribe`,
  `gpt-4o-mini-transcribe`, `gpt-4o-mini-transcribe-2025-12-15`, `whisper-1`
  and `gpt-4o-transcribe-diarize` [O1]. The guide recommends `gpt-transcribe`
  and marks the others deprecated with shutdown on 2027-02-26 [O2]; this
  documentation recommendation does not select a new ticket default.
- **VERIFIED:** OpenAI `gpt-transcribe` takes the `languages` array instead
  of `language`; never send both [O2]. Other OpenAI models take a single
  `language` [O1][O2]. `response_format=json` is supported; the
  `gpt-4o-(mini-)transcribe` models accept JSON only [O1]. The report lists
  `keywords` for `gpt-transcribe` only [O1]; optional fields are not a mandate
  to add controls.
- **VERIFIED:** OpenAI `stream=true` returns SSE [O1]. The report establishes
  support for `gpt-transcribe`, `gpt-4o-transcribe`, `gpt-4o-mini-transcribe`
  and `gpt-4o-transcribe-diarize` [O2]; `whisper-1` ignores it [O1].
  `transcript.text.delta` carries partial `delta`, and `transcript.text.done`
  carries full final `text`, with `languages` additionally reported for
  `gpt-transcribe`. Framing is `data: {json}` [O1][O2]. Diarized segment
  events do not expand this ticket's scope.
- **VERIFIED:** Groq accepts Bearer-authenticated multipart
  `POST https://api.groq.com/openai/v1/audio/transcriptions` [G2]. Models are
  `whisper-large-v3-turbo` and `whisper-large-v3` [G1][G2]. Uploads can use
  `file`; language selection uses single ISO-639-1 `language`, not an array.
  `response_format` accepts `json`, `text` or `verbose_json`; `prompt` is
  limited to 224 tokens [G1][G2]. There is no `stream` field, so Groq is
  final-only [G1][G2].
- **VERIFIED:** OpenRouter accepts Bearer-authenticated
  `POST https://openrouter.ai/api/v1/audio/transcriptions`, including multipart
  `file` with `model` [R1][R2]. Use single `language`, with auto-detection when
  omitted, not an array. Responses are single JSON, not streamed;
  `response_format` accepts `json` or `verbose_json`, while `text`, `srt`
  and `vtt` return HTTP 400. `prompt` is accepted but ignored [R1][R2].
  The separate JSON `input_audio` transport is evidence, not the selected
  multipart contract.
- **VERIFIED:** OpenRouter documents a dynamic transcription model catalog
  and examples `openai/whisper-1`, `openai/whisper-large-v3`,
  `openai/gpt-4o-transcribe`, `microsoft/mai-transcribe-1.5` and
  `microsoft/mai-transcribe-2` [R2]. These are evidence, not authority for a
  replacement model, default or runtime discovery mechanism. The original
  `openai/gpt-4o-mini-transcribe` example and `gpt-transcribe` routing remain
  **UNCONFIRMED** by this report.
- **VERIFIED:** OpenAI lists webm and a 25 MB upload maximum [O1][O2]. Groq
  lists webm, with 25 MB free-tier and 100 MB dev-tier limits [G1]. OpenRouter
  lists provider-dependent webm support and a 25 MB multipart limit [R1][R2].
  OpenAI requires the extension to match content type [O1]. These container
  listings do not establish opus codec acceptance.

[O1]: https://developers.openai.com/api/reference/resources/audio/subresources/transcriptions/methods/create
[O2]: https://developers.openai.com/api/docs/guides/speech-to-text
[G1]: https://console.groq.com/docs/speech-to-text
[G2]: https://console.groq.com/docs/api-reference
[R1]: https://openrouter.ai/docs/api/api-reference/stt/create-transcription.md
[R2]: https://openrouter.ai/docs/guides/overview/multimodal/stt.md

### Remaining readiness gaps

- **UNCONFIRMED:** Opus codec acceptance and the exact recorder MIME/content
  type need evidence for the selected provider/model pairs. Webm container
  support alone does not validate the approved webm/opus recording path.
- **UNCONFIRMED:** The exact OpenAI SSE content type, any `[DONE]` sentinel
  and multipart encoding of the `languages` array are not established here.
  Streaming support for the separately dated mini model is not established
  by the report's explicit supporting-model list.
- **UNCONFIRMED:** Maximum recording duration is not established. OpenRouter's
  documented 60-second upstream timeout [R2] is not a recording-duration cap.
- **UNCONFIRMED:** The original OpenRouter mini-model example and any custom
  endpoint's model/field/format/streaming contract need exact official evidence.
  No replacement model or preset default is selected here.
- **INFERRED:** Owner (877)'s preview replacement and the original caret
  insertion contract leave concurrent editing and selection changes
  unspecified. Their interaction needs disposition before implementation;
  this ticket supplies no mutation policy.

Return missing or contradictory evidence to the lead before ready promotion.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`).
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`).
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/` if configuration wiring touches it).
- If implementation adds package configuration manifests, apply
  `ai-docs/manuals/wsflow-mirroring.md` to the affected shared/wsflow surface;
  this capture selects no manifest field or mirror change.
- Outside scope: realtime WebSocket, Deepgram/ElevenLabs/AssemblyAI adapters,
  chat-completions `input_audio`, diarization/timestamps, splitting audio over
  25 MB, and OS keychain integration. Recording retention is limited to the
  owner (878) in-memory resend decision above.
- Preserve the documented field split: OpenAI `gpt-transcribe` uses
  `languages`, not `language` or both; other documented pairs use single
  `language` [O1][O2][G1][G2][R1][R2]. Keep OpenRouter/Groq final-only and
  restrict owner (877) previews to documented supporting OpenAI pairs.
- MAIN's generic-client suggestions are not additional owner decisions.
  No roughly 60-second recording cap, runtime model fetch, non-SSE JSON
  fallback policy or optional-field controls are selected by that report.
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
local-daemon upload and caret insertion contract after the remaining readiness
gaps receive evidence or lead disposition. Apply the documented endpoint,
model and language-field differences. For documented supporting OpenAI models only,
show dimmed partial previews and replace them wholesale with the final
transcript on done without committing partial text. Retain final-only behavior
otherwise. Implement the last-recording, memory-only resend button after
failure, with discard on a new recording or window close. Coordinate scoped
Electron permissions with EXEC4 without implementing a second frontend or
loosening origin isolation.

Future verification must cover provider/model-specific multipart request shape,
including `languages` versus `language`, and actual streaming capability for
selected OpenAI pairs versus final-only OpenRouter/Groq; renderer -> daemon ->
provider mediation; masked settings and no key leakage to LAN viewers;
secure-context controls; final insertion at the composer caret;
final-only fallback; dimmed partial previews only for supported OpenAI models,
wholesale final replacement on done and no committed partial text; last-recording
resend after failure, no disk writes and discard on a new recording or window
close; and packaged Electron microphone purpose text, OS permission handling
and denial for non-daemon origins. Verify recorder format acceptance against
the remaining codec/MIME gaps without assuming it from webm listings. Use
isolated fixtures, never the shared daemon or port 29417. This capture asserts
no provider integration or executable acceptance.
