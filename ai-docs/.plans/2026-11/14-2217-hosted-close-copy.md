# Plan: make the hosted-session "닫기" description truthful

## Problem

Idea ticket `261114-bug-ws-hosted-close-copy-conflicts-with-reopen` (may live on a docs branch, commit 6a0f78caf; read it via `git show 6a0f78caf` if absent from this tree). The dashboard hosted-session settings popover describes 닫기 as "대화를 완전히 끝냅니다. 다시 열 수 없습니다.", but a hosted Reopen exists (POST /agents/<id>/reopen, handoff.go Reopen, ko.json rail.reopen "다시 열기"). The copy is false.

## Outcome

Change only the user-facing description string(s) for close (Korean and any other locale files carrying the same claim) so they state what close actually does: ends the conversation/process and stops mail wake-ups; it can be reopened from the rail's "다시 열기". Keep it short, matching the popover's style. Bug fix of UI copy; no behavior change.

## Boundaries

- Locale/copy strings and tests that pin them only. No handler, route, lifecycle, or reopen behavior change; do not remove reopen.
- If the reopen affordance is not actually reachable from the UI (only an API), stop and report instead of guessing copy.
- Promote nothing; leave the idea ticket where it is (you may reference it in the commit's Ticket Updates).

## Verification

Machine-wide heavy-job throttle: no builds, go test, Node tests, or browsers in this turn. Static edit + commit; report the exact focused test command to run later.

## Return

Commit on `impl/develop/hosted-close-copy` with this plan. Report old/new strings per locale and the deferred test command.
