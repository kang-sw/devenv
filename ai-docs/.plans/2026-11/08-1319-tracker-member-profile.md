# Plan: tracker member profile - structured org / family / given name

Branch `delegate/tracker-member-profile` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/decree-thinness-strum`). Commit on this
branch; do not merge or push.

This is a data-structure change to the reference mockup
`ai-docs/ref/tracker-mockup/` (README is the data-structure contract) and its
Go port `agents-plugin-tool/internal/wstracker` (golden parity through
`testdata/gen-golden.js` and `golden_test.go`). Both must change together and
stay in parity. Read `ai-docs/manuals/shipped-surface-boundary.md` before
editing under `agents-plugin-tool/`.

A parallel presentation-only pass (branch `delegate/tracker-mockup-detail`)
is editing `app.js`, `app.css`, `README.md`, and `SCENARIOS.md` layout text
at the same time; keep your `app.js` changes to name and avatar rendering and
the join dialog so the later merge stays small.

## Decisions (user, board 2026-11-08)

User on (383) b: "repo에 처음 등록할 때 간단한 프로필을 받게 하는 게
좋겠습니다. 소속/성/이름 이런 느낌으로? 기본적으로는 새 repo 가입할 때 이전
프로필을 재활용하거나, 혹은 이전에 등록햇던 프로필 중에 선택하게 하고요
(이건 로컬에서). 회사에서 보통 이름을 SW개발팀 홍길동 이런식으로 짓게
하는데, raw 문자열이면 문제가 될 거 같아서 아예 항목들을 나누게."

1. **Structured profile** ((391) ok). A member carries `profile: {org,
   family, given}` instead of the `name` string, wherever `name` lives today:
   the roles file genesis members, `roles.add_member` (the stand-in for the
   self-join `member.join`, Settled (231)), the view state's `members` and
   `viewer`, and the Go port. `org` is one free-text string and may be empty;
   no org tree (team structure stays in `leads`). Validation: `given`
   required and non-empty; `family` may be empty (single-name people);
   flag (with a reason, in the existing flagged-write style) a profile write
   missing `given`. Keep the view state's derived display name available (for
   example a `name` field computed from the profile) if that keeps the diff
   small; note the choice.
2. **Display rules.** Display name: if `family` or `given` contains Hangul,
   CJK ideographs, or kana, `family + given` with no space ("홍길동");
   otherwise `given family` ("Ada Park"). `org` is not merged into the name;
   it shows as a muted prefix or tooltip where a person is named (Team tab,
   holders line, @-picker, item window), choose and note. Avatar ((383) b):
   CJK names -> `given` up to two characters ("길동"); others -> first letter
   of `given` + first letter of `family`, upper-cased ("AP"); a single name
   -> its first two letters. Replace `initials()` with this.
3. **Who writes it** ((392) b). The member writes their own profile at join
   (part of the join payload) and may change it later with a new
   `member.profile` event (`profile`). An admin may also change any member's
   profile with the same event naming `member` (department move, typo fix).
   Field writes merge by the existing (203) rule. A non-admin writing someone
   else's profile is flagged. The change shows in Activity, with the admin as
   the author when an admin made it.
4. **Local saved profiles** ((393) ok). The list of a person's earlier
   profiles is machine-local, never in the ledger; the real flow belongs to
   the later caller ticket. In the mockup, simulate it with localStorage: a
   "Join store" dialog (reachable from the store page or the quick switcher,
   your choice) that defaults to the last-used profile, lets the user pick one
   of the saved profiles or edit the fields (an edit saves a new entry), and
   emits the join (the `roles.add_member` stand-in or `member.join` if the
   engine already has it) for the chosen fixture viewer. Also a "Edit
   profile" action for self, and for admins on a member in the Team tab.
5. Fixtures: convert every member to a profile; give the fixture team a
   few Korean members (for example an org like "SW개발팀" and names like
   홍길동, 김민수, 남궁민) alongside the Latin ones, so both display rules show.
   Keep ids stable so scenarios and golden tests still line up.

## Verification

- `node --check` on every mockup script; no console errors in headless
  Chromium (cached Playwright under `~/.npm/_npx/*/node_modules/playwright`).
- `cd agents-plugin-tool && go build ./... && go vet ./internal/wstracker/ && go test -count=1 ./internal/wstracker/`, with the goldens regenerated from the
  changed engine (`testdata/gen-golden.js`) and the Go fold matching them.
  Add tests for: profile validation (missing `given` flagged), self edit,
  admin edit of another member, non-admin edit of another member flagged,
  concurrent field merge, and the display-name / avatar rules (Korean,
  Latin, single name).
- Screenshots to `/tmp/tracker-member-profile/`: Korean and Latin avatars in
  the Items list and Team tab, the holders line with org, the join dialog with
  saved profiles, the edit-profile dialog; light and dark.
- README: update the roles file shape, the `roles.add_member` row, add a
  `member.profile` row, the view state `members`/`viewer` shape, the display
  rules; add Settled rows for (383) b with the note, (391), (392) b, (393).
  Update SCENARIOS.md only where a step names a member's display name.

## Return

Commit, changed files, screenshot paths, test output, the choices made
under 1 and 2, and anything left open (stop and report rather than inventing
a protocol-layer change in `wsledger`).
