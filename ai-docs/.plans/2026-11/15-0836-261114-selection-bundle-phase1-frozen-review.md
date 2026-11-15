# Frozen Phase 1 review plan

Ticket: `ai-docs/tickets/ready/261114-feat-ws-selection-bundle-comment.md`.
Branch: `impl/develop/scoff-cone-frays`.
Base and pre-commit HEAD: `6a4b598b8f1c95352c9eb927cd3269e9411d5789`.
Scope: Phase 1 only. This plan is immutable after creation. The root lead owns the single independent full-delta xlarge review; no nested or duplicate lite review, commit, or phase Result precedes its findings.

## Implementation and review boundary

- New `wsinbox.Selection` and `SelectionPart` are transient parsed views, not persisted or request/cancel fields. `SelectionBlock` serializes independently escaped source mentions and one comment, even when empty, then existing `[Image #N]` references inside the outer selection.
- `ParseSelections` retains ordered groups between ordinary text, preserves exact raw segments, and treats malformed envelopes as opaque ordinary text, including nested valid-looking groups and malformed opening/closing tags. It does not extract a partial selection from a failed envelope.
- Existing `Normalize` canonicalizes valid grouped text inside Parts, includes its mentions in the existing per-send mention cap, applies the existing text cap, and checks explicit tail image references against the entry's assigned existing image numbers. Malformed groups remain ordinary text, with pre-existing outer whitespace normalization unchanged.
- Existing Merge uses the new group-aware remapper for Parts: ordinary markers still remap as before; valid selection tails remap with the existing image map; source and memo are literal. Failed envelopes stay opaque even during remapping. Legacy chip/topic note remapping is unchanged.
- The dashboard handlers require no source change: existing text/images/refs/refNums and cancel text/images/nums carry the groups through Normalize, merge, cancel, steer, interrupt, resend, drain, and delivery. New direct-handler tests start no listener, holder, provider, or daemon process.
- Legacy `mentions.go`, its tests, and existing dashboard route tests are unchanged byte-for-byte. No browser/UI, playbook, manifest, version, daemon, protocol, adapter, migration, or whole-ticket lifecycle change is included.

## Numeric-entity technical evaluation (T0 candidate, pending review)

The old Merge's flat `renumberMarkers` would otherwise rewrite marker-looking source/memo in new Parts. After ordinary XML escaping, only literal image-marker-shaped content in source/memo/attributes gets its opening bracket escaped as `&#91;`. Actual tail references keep the existing raw marker. The new group parser decodes the bracket escape before the legacy entity unescaper, once; the legacy MentionBlock/ParseMentions implementation is untouched. This is a candidate within the approved escaped-text representation, not authority for an unrelated protocol change.

For source and memo input `&#91; | &amp;#91; | & | < | [Image #1]`, exact candidate bytes are:

```xml
<wb:selection>
<wb:mention topic="t" id="3" entry="e1">&amp;#91; | &amp;amp;#91; | &amp; | &lt; | &#91;Image #1]</wb:mention>
<wb:comment>&amp;#91; | &amp;amp;#91; | &amp; | &lt; | &#91;Image #1]</wb:comment>
[Image #1]
</wb:selection>
```

`TestSelectionNumericEscapeBytesAndOneDecode` pins these bytes, exact one-decode source/memo equality, stable canonical reserialization, and unchanged old flat remapping changing only the tail from 1 to 4. `TestSelectionEscapesSurviveTheOldFlatMarkerRemapper` additionally covers marker-looking topic attributes, numeric-entity-looking literal content, and unchanged legacy decoding. Encoded image-tail references are rejected by the strict parser.

Compatibility evidence is bounded: old known-field JSON round trips retain the group in existing Parts (no unknown field to drop); the unchanged old leading mention reader leaves the whole outer selection as plain text; the unchanged old flat marker algorithm sees and changes only the true tail in canonical serialized groups. This is not an old installed binary/live session acceptance claim. Raw noncanonical text supplied directly to an old writer has no new-server canonicalization guarantee. Older guidance/readers do not gain rich selection presentation. Phase 2 owns composer/history rendering and drafts, and Phase 3 owns installed guidance. No T2 change, protocol number, schema change, or new image protocol is proposed.

## Verification completed before freeze

All commands run from `agents-plugin-tool/` through `/tmp/ws-selection-phase1-amends-humvee-thumping/verify.py`:

1. `go test -race -v -count=1 -timeout=90s ./internal/wsinbox` — pass. Includes all legacy tests, new strict grammar/escaping/limit/remapping/stored lifecycle cases, and four fuzz seeds (not an extended fuzz campaign).
2. `go test -race -v -count=1 -timeout=90s ./internal/wsdashboard -run '^TestSelectionText'` — pass. Covers real existing handler functions directly without listeners: two pushes/cross-entry source, independent text, empty/different comments, image merge/cancel/resend/steer/interrupt/drain, existing reference confinement, sniffing, 10 MiB limit, eight-image merged cap and no leaked refusal files.
3. `go vet ./internal/wsinbox ./internal/wsdashboard` — pass (empty tool output; exit 0).
4. `git diff --check` — pass.
5. `git diff --exit-code -- agents-plugin-tool/internal/wsinbox/mentions.go agents-plugin-tool/internal/wsinbox/mentions_test.go agents-plugin-tool/internal/wsdashboard/inbox_routes_test.go` — pass, confirming no legacy implementation/assertion edits.

Complete final logs and raw ownership evidence are in `/tmp/ws-selection-phase1-amends-humvee-thumping/{inbox-final,dashboard-final,vet-final}/`. Each directory has `output.log`, `owned.json` (exact argv/cwd/clean environment and pre-release PID/PGID/start identity), and `final-ps.txt` (raw final system process scan). Each registered group was quiescent after its command. Earlier non-race development runs are retained in sibling `inbox-v1`, `dashboard-v1`, and `inbox-v2` directories and passed; final evidence supersedes their changed source inputs.

Environment is a clean whitelist, not inherited provider credentials: GOPROXY=off, GOSUMDB=off, GOTOOLCHAIN=local, GOENV=off; fresh per-run HOME/XDG config/cache/data/state, WS cache/config, TMPDIR/GOTMPDIR; existing `/Users/kang-sw/Library/Caches/go-build` and `/Users/kang-sw/go/pkg/mod` reused. `-count=1` disables Go test result reuse. Each command has a 180-second outer deadline in addition to the 90-second Go test deadline. The trampoline is blocked on stdin until its new-session PID/PGID/start identity is recorded. No holder-spawning tests were launched; selected code and package init paths were inspected first. No port, provider request, dependency download/install, owner daemon operation, merge, push, or version bump occurred.

## Root reviewer handoff

Review the complete frozen patch and all four changed files, not only tracked `git diff` (three new files are initially untracked). Evaluate strict whole-group security/fallback, exact logical source/memo versus actual image references, old-writer numeric-entity compatibility, normalization/limits and inbox/handler lifecycle preservation. The working-tree patch, file hashes and immutable plan hash are recorded in `/tmp/ws-selection-phase1-amends-humvee-thumping/frozen-manifest.json`; the full patch is its sibling `phase1.patch`. The root will return findings for bounded fixes before a single logical Phase 1 implementation commit. Omitted: Phase 2–4, all browser/native-IME acceptance, installed mixed-version/runtime checks, extended fuzzing, full process-spawning dashboard/MCP suites, and daemon operations, per the assigned boundary and safety restrictions.
