# Whiteboard policy-only options

## Authority

MAIN assigned this single bounded task at 2026-11-15 14:42Z, for a post-ship v0.50.1 merge. Read /tmp/wb-check-options-brief.md in full; its owner decision1166=a explicitly authorizes input semantic tightening and display behavior. Owner words must appear verbatim in commit AI Context:

23:41 answer: "1166=a"
Option a (chosen): "보드가 그 명령을 거절하고 "선택지가 필요하면 정책 질문으로 쓰라"고 돌려준다. 이미 저장된 항목의 선택지는 화면에 그리지 않는다."

## Scope and contract

- In agents-plugin-tool/internal/wswhiteboard, locate the current "Options on a non-policy item" info-line site. Refuse wb:add with options or recommend for a non-policy class, and wb:patch whose resulting non-policy item leaves or sets either. Refuse whole batches, preserving normal atomic refusal behavior; tell the caller to use class="policy" for a pick. Reclassification to policy and clear="options" repair paths must continue working for legacy items.
- In the existing whiteboard renderer under agents-plugin-tool/internal/wsdashboard (or the actual renderer owner discovered in source), do not render stored non-policy options or recommendations: no stray option buttons or star recommendation line. Stored data remains unchanged and readable.
- Add focused add/patch refusal tests, both repair-path tests, a stored-check options render regression and recommendation-only coverage where relevant. Follow existing style; do not broaden into unrelated whiteboard behavior.
- Read the bundled whiteboard playbook. If already explicit that options are policy-only, leave it untouched; otherwise add at most one sentence and any required mirrors/manifests. MAIN/owner separately polishes prose, so report any documentation touch precisely. Read all path-matching manuals and skill-authoring/mirroring rules before edits.

## Compatibility and verification

State directional tier with exact source lines for old/new writers/readers. This is explicit tool input semantic tightening, not merely presentation: old daemons keep accepting and storing these items, new renderer hides their stray choices, and old stored data MUST remain readable. No field/schema migration, silent data deletion, tool/arg rename, or version boundary. Do not call formerly accepted inputs unchanged-compatible; explain the tightened validation separately from storage/display compatibility.

Offline Go: GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local. Run touched test names and -run 'Whiteboard|Item|Option|Policy|Check' in internal/wswhiteboard. For renderer, run the focused relevant internal/wsdashboard test(s), never the full wsdashboard or internal/mcp package and never whole Go suite. If browser testing is needed, use only existing cached Playwright: WS_PLAYWRIGHT_MODULE=/Users/kang-sw/.npm/_npx/705bc6b22212b352/node_modules/playwright and PLAYWRIGHT_BROWSERS_PATH=$HOME/Library/Caches/ms-playwright; no download. Preserve exact commands and full outputs, failures included. No skips, deletions to hide failures, timeout increases or blind reruns. No providers, network/download/install, credentials, unrelated signals/stashes/files, shared daemon/runtime changes, push or CI dispatch. No old workers/root reuse beyond this newly leased worktree.

## Delivery

Target ETA15:22Z / 00:22 KST. Commit the implementation and this unchanged plan on the acquired branch, with owner words verbatim, why this approach, and source-backed tier in AI Context. Return clean HEAD/base/changed-file list, full report/command/output artifact paths, focused verification evidence, compatibility assessment and gaps. Reports use short, complete plain English sentences with VERIFIED/INFERRED labels and citations. Report blockers or ETA slips over30minutes immediately. Stop before expanded scope or further unsettled semantics. MAIN alone merges develop, pushes, runs CI or operates shared runtime. Hold the lease until MAIN reports merge; no follow-on work.
