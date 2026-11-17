# Perf: stop inlining images as base64 in dashboard pages; serve image files by URL

## Context

The owner's PC runs AhnLab V3, so every file touch costs 10-100x. The owner
measured initial session loading in Edge DevTools on 2026-11-17 17:03-17:10:
- `/s/<rid>?frame=1` takes 5.9 s for 210 kB.
- `topic?name=<topic>` responses take 0.4-2.3 s for 34-206 kB.
- `image?path=C:/Users/user/.cache/ws@kang-sw-devenv/...inbox/attachments/...`
  returns 404 in 1.9 s each.

The owner's diagnosis (17:10) is that base64 encoding and transfer of JPEG
images strongly affects initial session loading. Images include whiteboard
panel and markdown images, chat or inbox attachment images, and topic images.
The server reads each image file, base64-encodes it and inlines it into
HTML/JSON responses such as topic payloads and frame HTML, possibly also
`srcdoc` panel frames.

## Owner decision (2026-11-17 17:10, binding)

> "jpeg base64는 아무래도 없애는 게 낫겠습니다. LAN에서는 그냥 루트 내 이미지만
> 지원, 로컬에서는 base64없이 그냥 이미지 그대로 보내면 되고요. (어차피 로컬이니까)"

In English:
- Remove base64 image inlining.
- **Local (loopback) clients** get the image file itself, served by URL, with
  no base64.
- **LAN clients** are served only images inside the session root. Paths
  outside the root are not served to LAN.

## Outcome

1. **Find every place that inlines images as base64 or data: URIs** into
   served pages or JSON. Search for `base64`, `data:image`, `EncodeToString`
   in `wswhiteboard`, `wsdashboard` and `mcp`, plus the frontend.
   - Replace each with a URL to an image route that streams the file as is:
     correct `Content-Type`, no re-encoding.
   - Use cache headers so the browser does not refetch: an ETag from
     size+mtime, or a long max-age with a version query.
   - Prefer the existing `image?path=` or file-serving route if one exists;
     reuse it rather than adding a new one.
2. **Access rule on the image route.** Loopback requests may fetch local image
   paths, as today's behaviour for local allows. LAN requests are restricted
   to images inside the session root.
   - Reuse the existing credential deny set (`fs_denied.go`) and the root
     checks. Do not weaken them.
   - Images in sandboxed `srcdoc` panel frames must still load. Check how the
     panel sandbox fetches assets: there is a "host-page relay for served file
     images" decision in branch `impl/develop/render-cost-phase2`
     (`b7894d2b8`). Read its recorded decision and stay consistent with it.
3. **Make the 404 cheap.**
   - Find why a missing attachment image costs 1.9 s before answering 404.
     Suspects: path resolution, EvalSymlinks, deny-set build, or a root walk.
   - Make a miss cost at most a stat or two.
   - Report why those attachment paths are missing: moved, pruned, or the
     wrong root.
4. **The inbox attachment images** (chat screenshots under
   `~/.cache/ws@.../dashboard/sessions/<rid>/inbox/attachments/`) are outside
   the session root.
   - Keep them viewable locally.
   - For LAN, they fall outside the "root only" rule. Report whether they are
     served to LAN today. If they are, keep that behaviour only if it was
     already explicitly allowed. Otherwise follow the owner rule and report it.

## Constraints

- **Tier.** State it with source lines.
  - Responses change shape from data: URIs to URLs. Older daemons and newer
    pages are served by the same daemon, so this is expected T0.
  - Stored board panels may contain data: URIs authored by agents. Leave
    stored content as is: only stop the server from generating base64.
- **Security.** LAN must never gain access to paths it cannot reach today.
  Write a test for: LAN out-of-root denied, LAN in-root allowed, loopback
  local allowed, and the credential deny set still enforced.
- **Coordination.** Do not touch:
  - CV CSS `dashboard-board.css:93-96`;
  - `dashboard-session-view.js` (switch-click worker);
  - composer bubble code;
  - tracker view code (tracker-refetch worker);
  - `wslive` (observed-fold worker).
- **No local runs on this PC.** No `go test`, Go test exes, ws/ws-mcp exes,
  launcher or daemon (antivirus rule).
- **Go runs only on w5090:**
  `D:/sys_lock.sh remote w5090 run image-nob64 30 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/pretext-shopping-example' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go vet ./... && GOOS=windows go vet ./internal/... && go test -count=1 ./internal/wsdashboard ./internal/wswhiteboard ./internal/mcp'`
  - Redirect the output to a file under `D:/Temp/image-nob64/`.
  - Wait for the run to finish before ending your turn.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- **Process.** Simplest complete change; R1 review, then a fixes-only R2.
- **Commit.** Commit on `impl/develop/image-no-base64` with your own model's
  Co-Authored-By trailer. Do not push.
- **Temp files** go only under `D:/Temp/image-nob64/`.
- **Urgency.** This is on the owner's critical path for page load.

## Report

- Each inlining site and its replacement.
- The access rule and its tests.
- The 404 cost cause and its fix, plus why the paths were missing.
- Bytes per topic and frame response, before → after, estimated or tested.
- The commit and files.
- The w5090 result.
- The tier.
- Anything left alone.
