---
title: "ws tracker wiki: native-git wiki branches in the store repository"
parent: 261107-epic-ws-personal-common-store
related:
  261107-feat-ws-tracker-ledger-protocol: the store repository, auth, signing keys, and sync cadence the wiki branches share; the wiki deliberately does not use its event protocol
  261108-feat-ws-tracker-data-structure: unchanged by this ticket (no new event types)
  261108-feat-ws-tracker-ledger-attachments: prerequisite for Phase 1; the attachment read route whose content checks the wiki image route reuses, and the filtered/unfiltered fetch split beside which the wiki fetch runs unfiltered
  261109-feat-ws-tracker-shared-markdown-editor: prerequisite for Phase 2; the wiki card edits pages with its shared editor and preview
  261108-feat-ws-tracker-support-agent-chat: the support-agent profile that gains the wiki tools
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 00f08d64c08fc052
sage-review-completeness-reviewed: 00f08d64c08fc052
completed: 2026-11-09
---

# ws tracker wiki: native-git wiki branches in the store repository

## Background

Side discussion on the board (2026-11-08, topic "트래커: 위키 기능"): the user
asked whether a wiki fits the tracker's protocol model, then proposed keeping
the wiki out of the event protocol entirely: "그냥 git branch 하나를 위키로
쓴다던지 ... 위키용 브랜치는 뭔가 native git 모델을 따라가는". Sequenced
after the tracker core children by board (376) b. The open questions were
settled on the board on 2026-11-09 (topic "트래커: 위키", items (72)-(91)).

## Decisions

- **Storage: wiki branches in the same store repository, following the
  native git model** (board (377) b). Pages are markdown files on a shared
  wiki ref; history, rename, delete, diff, blame, and search come from git
  and plain file tools. Concurrent edits merge with git's own 3-way merge:
  non-overlapping edits merge automatically. The store repository's auth and
  sync are shared.
  Rejected: wiki pages as a tracker event kind (`page` / `page.edit`) in the
  ledger protocol (more engine surface for what git already does); a wiki
  repository separate from the store (second auth and sync path).
- **Several wikis per store, one ref each** (board (86) a, (87) ok): each
  wiki is `refs/heads/wiki/<name>` in the store repository. The namespace is
  outside `refs/heads/tracker/` because the ledger sync fetches
  `+refs/heads/tracker/*` and warns on every ref there that is not a ledger
  ref (`wsledger/remote.go` `classifyRef`, `WarnBadRef`), the same reason the
  attachment refs live under `refs/heads/attach/` (board (73), (71) a;
  pending 261108-feat-ws-tracker-ledger-attachments). The fetch refspec is
  `+refs/heads/tracker/*` (`agents-plugin-tool/internal/wsledger/remote.go#L95`,
  `refPrefix` in `agents-plugin-tool/internal/wsledger/ids.go#L18`).
  Separate refs keep concurrent saves to different wikis from forcing each
  other's fetch-merge-repush, and let a whole wiki be handled as a unit.
  Rejected: one `refs/heads/wiki` with top-level folders as spaces (git
  cannot hold `refs/heads/wiki` and `refs/heads/wiki/<x>` together, so it
  would need a migration later); reserving the namespace with only
  `wiki/main` (the picker and create path are a small cost).
- **Wiki names** (board (89) ok): `[a-z0-9][a-z0-9-]*`, at most 64
  characters. Rejected: validating against git's full ref-name rules (a
  narrow allowlist fails more clearly).
- **A wiki is created by its first page save** (board (90) ok): the first
  commit pushes the new ref; MCP `wiki_update` writes and the drawer card's
  "new wiki" action use the same path. A store with no wiki shows an empty
  state with a "new wiki" button. Rejected: a separate create operation or
  an empty ref (git has no empty branch).
- **No whole-wiki delete in this ticket** (board (91) a): tools and UI offer
  page delete and revert only; removing a wiki ref is left to an admin with
  plain git. Rejected: any member pushing a ref deletion (the server loses
  the history; only other clones' copies could restore it, against
  recovery-over-prevention and the append-only constraint); an "archived"
  marker (can be added later without a format change).
- **A remote wiki ref deleted or rewound is followed, with a local
  recovery copy** (board (96) c): when a fetched `refs/heads/wiki/<name>` is
  gone or no longer descends from the local copy, the clone follows the
  remote (the picker drops a deleted wiki, and nothing resurrects it), keeps
  the replaced tip under a never-pruned, unlisted per-clone recovery ref
  (`refs/wiki-known/<name>/<oid>` or a sibling local-only namespace, never
  pushed), and reports a warning. Deletion is judged against a never-pruned
  known value per wiki ref, recorded on push and fetch (the ledger's
  `refs/ledger/known/` precedent, 1f804457): only a once-known ref can read
  as deleted, and a local-only wiki (a store that gained a remote later) is
  pushed on the first sync. `wiki_query` lists recovery points and
  `wiki_update` restore can restore a page (or recreate the wiki by a save)
  from one. Rejected: following the remote with no copy (a force-push or
  delete wipes every clone's copy on the next sync, destroying the recovery
  copy); refusing and keeping the local ref like the ledger's rewind refusal
  (an admin's delete is undone by the next save from a stale clone, and the
  picker keeps listing it).
- **Pages** (board (83) ok): a page is a `.md` file at any folder depth in
  the wiki tree; its page path is the file path without `.md`. Rejected: a
  separate metadata or index file (the git tree is already the page tree).
- **Sync rides the store's sync cycle** (board (82) ok): the store fetch
  also fetches `refs/heads/wiki/*`; a save pushes at once. Rejected: a second
  sync path.
  - The wiki fetch is unfiltered, with an explicit `--no-filter` (the
    promisor remote may carry `partialclonefilter`, and ledger git runs keep
    `GIT_NO_LAZY_FETCH=1`, so a filtered wiki fetch would break page read,
    search, diff, blame, and the save merge). Every clone therefore holds
    every wiki image: the accepted consequence of (85) a, unlike the
    on-demand fetch of item attachments. The wiki fetch limits negotiation
    to wiki refs (`--negotiation-tip`), because a wiki image can share a
    blob oid with an item attachment whose commits were fetched filtered.
  - Pushing a save at once is a deliberate wiki exception to the epic's idle
    push, which governs ledger events.
  - A store with no remote commits wiki saves locally only. With a remote, a
    transport or credential failure refuses the save and the editor or agent
    keeps its draft; a save is never committed locally to be pushed later,
    since a deferred merge could conflict with no editor present.
- **Edit permission: any store member edits any page** (board (375) a). Per-page
  locks or per-commit validation cannot be enforced on a shared history.
- **Text conflicts refuse at save** (board (74) a): the daemon commits on top
  of the commit the edit started from, fetches, 3-way merges, and pushes.
  When lines overlap, the save is refused and the editor (or the agent tool's
  answer) shows the saver's version and the other version together; the
  saver merges and saves again. A conflict never enters the branch. A
  rejected push (non-fast-forward) retries by fetch and merge, never force.
  Rejected: committing conflict markers; parking conflicted merges on a
  per-clone pending ref.
  - The base is explicit: a `wiki_query` page read returns the commit it read
    from, and every `wiki_update` operation (write, rename, delete, restore)
    and the drawer editor's save require it as `base`. There is no fallback
    to the current tip, which would overwrite a concurrent edit unseen.
    Creating a wiki (its first save, or recreating one after a delete) uses
    an explicit new-wiki form instead of a base, refused when the ref
    already exists; an omitted `base` is still refused.
  - A save, rename, delete, or restore whose `base` is not an ancestor of
    the current remote tip (ref absent included) is refused with the
    conflict answer shape, so a stale clone never re-pushes history a
    remote rewind or delete dropped.
  - Any non-clean merge refuses, not only overlapping lines:
    modify/delete and rename conflicts too.
  - The refused answer carries the saver's text, the other side's text, the
    other side's commit, and the merge base commit.
  - The merge runs in the bare store clone within the existing git 2.34
    floor (`git merge-tree --write-tree` needs 2.38; a temporary index with
    `read-tree -m` and `merge-file` stays inside it). Raising the floor is
    rejected.
- **Signing** (board (75) a): wiki commits are signed with the clone's
  existing SSH key (the ledger's `gpg.format=ssh` setup) and verified
  against the roles file keys. Unverified commits show in history with a
  warning but are not refused (a shared branch cannot refuse an
  already-pushed commit). This warning, rather than the epic's "ignore and
  flag" for rule-breaking events, is a deliberate wiki exception: the epic
  rule governs ledger events. Rejected: unsigned commits relying on the
  author field (anyone with push access can forge it).
- **No `@mention` notices from wiki commits in this ticket** (board (76) b).
  Deriving notices needs a per-person read watermark over wiki history; the
  storage format does not change when it is added later.
- **Images in pages live in the wiki branch** (board (85) a): a pasted image
  is committed with the page under `_attachments/<sha256>.<ext>` of that
  wiki's tree, and the page links it with the same `attachment:<sha256>`
  syntax item bodies use (pending 261109-feat-ws-tracker-shared-markdown-editor
  Phase 2); the wiki image read route looks it up in the
  page's own wiki tree. Reverting a page restores its images with it.
  `wiki_update` takes image bytes base64-encoded in its argument, capped at
  the roles file's `attachments.max_bytes`. In a page, an `attachment:`
  target resolves to the wiki image route with the wiki name; item bodies
  keep the item attachment route. In Phase 2, a pasted image is held
  daemon-local for the preview before save, never pushed to the attachment
  refs, and written into `_attachments/` by the page save.
  Rejected: the per-member attachment refs (`refs/heads/attach/...`),
  whose staging and expiry exist to protect the event refs from older
  clients, which does not apply to the wiki.
- **Image read route security** (board (88) ok): the wiki image route uses
  the attachment read route's rules (pending
  261108-feat-ws-tracker-ledger-attachments): the type is decided by magic bytes from
  the image allowlist, never by member-supplied metadata, with CORP
  same-origin and a Sec-Fetch-Site check that keeps the `none` allowance
  (06084496). The route is addressed by wiki name and sha256, sits behind a
  tracker-host method (wsdashboard imports no tracker or ledger package,
  057a2ab0), and is listed in `TestLANModeGuardsEveryRoute`.
- **Links** (board (78) ok, revised by (86) a / (87)): item bodies and
  comments link a page with a standard markdown link
  `[text](wiki:<name>/<path>)`; pages refer to tracker items with the
  existing `#ID` references. Links are always absolute, also within one
  wiki (board (89)). Rejected: `[[wiki:path]]` (not standard markdown, the
  renderer would need new syntax).
- **UI: wiki in the Items panel for reading and editing** (board (77) a;
  placement board (129) a): the tracker's left Items panel header gets an
  *Items | Wiki* switch; under *Wiki* that area shows a page tree (left) and
  the page (right), and editing happens in place. In the narrow layout Wiki
  is one tab (the tree folds into a Pages control). The store page
  (`#/<storeId>`) uses the same switch at full width. Page list, rendering
  with the tracker renderer, editing with the shared editor
  (`261109-feat-ws-tracker-shared-markdown-editor`), a wiki picker and "new
  wiki" as before. History browsing and recovery go through the agent tools
  (the page's more menu only points to the support agent). The look is the
  approved static mockup `ai-docs/ref/wiki-mockup/index.html` (board (133)
  a). Rejected: a card in the right tab pane (board (129) c, too narrow for
  long pages plus the editor); a pane-tab page list with a floating page
  window (board (129) b, opening and closing windows between pages); a
  read-only card; no UI. Not in scope (board (134) ok): a formatting
  toolbar beyond the shared editor's Write/Preview, a history/restore UI,
  a rename/delete UI.
- **Agent tools** (board (79) ok): MCP `wiki_query` (page read, list,
  search, history, diff) and `wiki_update` (write, rename, delete, restore a
  page to a given commit), each taking the wiki name. Restore is always a
  new commit. The support-agent profile gets both tools, since the support
  agent works the tracker on the user's behalf, and its embedded instruction
  text (`wsagent/instructions/tracker-support.md`) gains a line on them.
  Rejected: separate tools per
  operation (the tracker uses the same query/update pair).
- **Recovery over prevention** (board (375) note): vandalism is rare, but
  recovering from it must be convenient; the user expects to ask an agent to
  do it, so recovery is an agent tool surface first.
- **Two phases** (board (84) ok): Phase 1 is the server side and the agent
  tools, already useful to agents on its own; Phase 2 is the Items panel
  wiki UI.

## Constraints

- No change to the tracker event protocol or `wstracker` engine; the wiki
  adds no event types.
- Wiki refs are append-only for clients (no history rewrite, no force push,
  no ref deletion through ws tools).
- `@mention` notices from wiki commits are out of scope (board (76) b).
- Whole-wiki delete or archive is out of scope (board (91) a).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)

## Prior Decisions

- 404feada (2026-11-08, commit): "User side discussion on the board: wiki in the tracker protocol vs a native-git wiki branch; answers (375) a with recovery note, (376) b (idea after core children), (377) b (separate branch in the store repo)." — bearing: supports
- 1f804457 (2026-11-08, commit): "Fetched refs land in refs/ledger/remote/ and a never-pruned refs/ledger/known/ records the last value seen on or pushed to the remote, so a deletion stays distinguishable from a ref that never reached the remote." — bearing: constrains
- 1f804457 (2026-11-08, commit): "Push rejection is read from git's porcelain '!' line in the runner's combined output; transport and credential failures stay errors so Sync reports them and keeps the writes pending." — bearing: supports
- 06084496 (2026-11-09, commit): "reloading a tab the link menu opened ... all send Sec-Fetch-Site: none and were 404ing. none is a user-started navigation, not another site embedding or probing the image" — bearing: constrains
- 087228bc (2026-11-09, commit): "one tracker-support-claude conversation opened through POST /agents. A default-type subagent had only Agent plus the profile's five ws tools" — bearing: constrains
- 261108-feat-ws-tracker-support-agent-chat (2026-11-08, Decisions): "wsagent gains a builtin profile layer (the builtinWorktreeProfiles() precedent in internal/mcp/worktree_profile.go) and the support instruction text is embedded in the binary" — bearing: constrains
- 261107-feat-ws-tracker-ledger-protocol (2026-11-07, Decisions): "Key and clone id storage ((245) ok). One personal SSH ed25519 key at ~/.ws/tracker/keys/, shared by all of a person's stores; a random clone id generated on first open" — bearing: supports
- 261107-epic-ws-personal-common-store (2026-11-07, Cross-Child Decisions): "Repository boundary (user, board (124)): one repository per team or confidentiality boundary ... access control is delegated to the hosting repository's permissions." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsledger/remote.go fetch path or a sibling wiki package; agents-plugin-tool/internal/mcp new wiki tools; agents-plugin-tool/internal/wsagent/builtin.go SupportWSTools; agents-plugin-tool/internal/wsdashboard image route and assets/tracker.js for Phase 2 |
| scope.surface | public-interface | new MCP tools wiki_query and wiki_update, a new daemon image route, and the refs/heads/wiki/<name> ref namespace other clones read |
| scope.new_public_symbol | yes | MCP wiki_query, wiki_update |
| scope.new_type_contract | yes | wiki tool argument and answer schemas; refs/heads/wiki/<name> naming rule; _attachments/<sha256>.<ext> tree layout; wiki:<name>/<path> link form |
| scope.test_surface | new-files | no wiki tests exist, grep for wiki under agents-plugin-tool/internal Go sources returned nothing; existing agents-plugin-tool/internal/mcp/tracker_tools_test.go and agents-plugin-tool/internal/wsagent/builtin_test.go cover neighbours |
| complexity.reuse_points | confirmed | ssh signing via gpg.format=ssh in agents-plugin-tool/internal/wsledger/gitio.go#L257; Sec-Fetch-Site checks in agents-plugin-tool/internal/wsdashboard/image_route.go; SupportWSTools in agents-plugin-tool/internal/wsagent/builtin.go#L30; attachment route itself pending 261108-feat-ws-tracker-ledger-attachments |
| complexity.side_effect_risk | high | saves push to a shared remote and the store sync gains a second fetch refspec beside the ledger fetch |
| risk.correctness | high | base-commit 3-way merge, conflict refusal with both versions, and non-fast-forward retry without force |
| risk.fit | moderate | must ride the ledger sync cycle without entering the refs/heads/tracker/ namespace classifyRef warns on |
| risk.test | moderate | needs real git remotes with concurrent writers and signing keys to exercise merge and retry paths |
| risk.security_or_contract | high | commit signature verification against roles keys, magic-byte image serving, and append-only ref rules for clients |

## Phases

### Phase 1: wiki branches, sync, save path, and agent tools

- Fetch `refs/heads/wiki/*` with the store sync; save path that commits on
  the edit's base commit, fetches, 3-way merges, pushes, retries a rejected
  push by fetch and merge, and refuses a text conflict with both versions.
- Sign wiki commits with the clone's key; verify against roles file keys and
  report unverified commits in history answers.
- Wiki creation by first save, name validation, page path rules.
- Remote delete/rewind handling: follow the remote, keep the replaced tip
  under the local recovery ref, warn; recovery points in `wiki_query` and
  restore from them in `wiki_update`.
- MCP `wiki_query` / `wiki_update` with the operations above; restore as a
  new commit; add both to the support-agent profile.
- Image blobs under `_attachments/` written by `wiki_update` (base64 bytes,
  allowlisted types, size cap) and a read route with the attachment route's
  checks.
- Starts after 261108-feat-ws-tracker-ledger-attachments Phase 1 lands,
  whose read route checks it reuses.

#### Verification

Go tests against real git repositories in temp dirs (a bare remote and two
clones with signing keys):
- non-overlapping concurrent saves merge;
- overlapping, modify/delete, and rename conflicts refuse with the answer
  fields above;
- a non-fast-forward push retries by fetch and merge and never forces;
- a missing `base` is refused;
- no-remote local commit; transport failure refuses and leaves the ref
  unchanged;
- signed commits verify against roles keys and an unsigned or foreign-key
  commit is reported unverified;
- wiki name validation and page path rules;
- a remote ref deleted or force-rewound by plain git is followed, the old
  tip survives under the recovery ref, a warning is reported, a later save
  from that clone does not recreate a deleted wiki, and restore from a
  recovery point works;
- the wiki fetch is unfiltered and leaves `classifyRef` warnings unchanged;
- an image used both as an item attachment and as a wiki image reads from a
  second clone;
- a stale-base save after a remote force-rewind is refused and re-pushes
  nothing; the new-wiki form is refused on an existing ref;
- a store that gains a remote pushes its local-only wikis rather than
  dropping them;
- the image route serves by magic bytes only, refuses non-allowlisted bytes
  and oversized writes, applies CORP and Sec-Fetch-Site, and is in
  `TestLANModeGuardsEveryRoute`;
- MCP tool tests for both tools, and the support profile lists them.
From `agents-plugin-tool/`: `go test` on the touched packages plus
`./internal/mcp/`, `./internal/wsagent/`, `./internal/wsdashboard/`, and
`go vet ./internal/...`.

### Result (34218aa) - 2026-11-09

Commits on `impl/develop/much-haiku-grove`: 17d383e (wsledger wiki branches,
sync, save path, host methods), 86ecf88 (dashboard image read route),
0cb6003 (MCP `wiki.query` / `wiki.update`, support profile, runtime maps),
34218aa (review fixes).

- Wikis live in package `wsledger` (`wiki.go`): `refs/heads/wiki/<name>`,
  pages `<path>.md` at any depth, images `_attachments/<sha256>.<ext>`.
  `fetchRefs` fetches the wiki namespace last, `--no-filter`, with
  `--negotiation-tip` limited to wiki refs. `absorbWiki` follows the remote;
  a rewind or a deletion of a wiki this clone knew keeps the replaced tip
  under the local-only, never-pruned `refs/ledger/wiki-recovery/<name>/<oid>`
  and warns (`WarnRewound` / `WarnDeleted`), judged against
  `refs/ledger/known/wiki/<name>`; a wiki written before the store had a
  remote is pushed by `Sync`, `Register`, and the next save
  (`pushLocalWikis`, create-only or fast-forward under a lease).
- `WikiSave` requires an explicit `Base` (ancestor of the tip; otherwise a
  `stale-base` conflict, including an absent ref), refuses `NewWiki` on an
  existing ref, applies the edit on the base tree, commits it signed with
  the clone's key, merges three-way against a moved tip (Go-level tree merge
  plus `git merge-file`; kinds `text`, `modify-delete`, `add-add`,
  `tree-file`, `rename`), pushes the candidate commit by oid under
  `--force-with-lease` on the tip it saw, and on a rejection fetches and
  merges again up to five times, never forcing. `*WikiConflict` carries
  kind, reason, base, theirs, the saver's text, and the tip's text. A store
  with a remote never keeps a commit the remote refused; without a remote
  the local ref moves alone.
- Reads: `WikiList`, `WikiPages`, `WikiRead` (returns the commit as the
  next base), `WikiSearch` (`git grep`), `WikiHistory` (signatures verified
  against member keys, revoked keys included as for attachment commits;
  unverified commits reported, not refused), `WikiDiff`, `WikiRecovery`,
  `WikiImageRead` (hash-checked). Restore writes the page from an older
  commit as a new commit and brings back the `attachment:<sha256>` images it
  references.
- `wstrackerhost` wraps the above; `WikiSave` types images by magic bytes,
  bounds them by `attachments.max_bytes`, records a landed save as a sync
  and a refusal as nothing. `ReadWikiImage` backs the dashboard route
  `GET /tracker/stores/<id>/wiki/<name>/attachments/<sha256>`, which shares
  the attachment route's tail (cross-site refused with `none` kept, magic
  bytes only, nosniff, CSP, CORP same-origin) and is in
  `TestLANModeGuardsEveryRoute`.
- MCP `wiki.query` (list, pages, read, search, history, diff, recovery) and
  `wiki.update` (write, rename, delete, restore; `images` as base64) are
  gated like the tracker tools (a support conversation reaches its pinned
  store alone), listed in `SupportWSTools` with one instruction line, and
  present in the ws, wsflow, and pi `runtime.json` (pi mirrors ws byte for
  byte; its bridge test pins the tool set).
- Verification: `go test` passes on `./internal/wsledger/` (wiki tests
  cover every bullet of the list above against a bare remote and two
  clones), `./internal/wstrackerhost/`, `./internal/wsdashboard/`,
  `./internal/mcp/`, `./internal/wsagent/`, `./internal/wsrsrc/`,
  `./cmd/ws-mcp/`; `go vet ./internal/...` and `gofmt -l` clean; wsflow and
  ws runtime-contract and downstream-neutral Python tests pass. The pi node
  test did not run (no `node_modules` in the worktree); its tool set was
  checked against `runtime.json` by script.
- Deviations: wiki code lives in `wsledger` rather than a sibling package
  (the route facts allowed either); the tool names are `wiki.query` /
  `wiki.update` (dotted, like the tracker tools) rather than the ticket's
  underscore spelling.
- Review (lite): four Important findings, all fixed in 34218aa. Minor
  findings left open: a delete that prunes a folder conflicts spuriously
  with an addition inside that folder (refused, not wrong); a page holding
  a NUL byte fails `git merge-file` with a raw error instead of a text
  conflict; `commit` arguments accept any commit of the store clone, not
  only the wiki's history; warnings found by a save's own fetch reach that
  save's result but not the next Sync report.
- Phase 2 remains; the ticket stays in `ready/`.

### Phase 2: wiki in the Items panel

- The render route gains a page mode taking the wiki name: `attachment:`
  oids render when found in that wiki tree's `_attachments/` or among the
  held pasted images; a save referencing an expired held image is refused,
  as for items.
- The UI follows the *Items | Wiki* switch placement (board (129) a) and
  the approved mockup `ai-docs/ref/wiki-mockup/index.html` (board (133)
  a): wiki picker, page tree, page rendering (tracker renderer, `wiki:`
  links resolved in item bodies and pages, `attachment:` images from the
  wiki tree), in-place edit with the shared editor and preview, image paste
  into the wiki branch, refused-save view showing both versions, empty
  state with "new wiki"; the narrow layout's Wiki tab and the store page
  use the same pieces.

#### Verification

- wsdashboard page tests pin the card's markup hooks and the wiki routes.
- `node --check` on changed JS.
- A headless browser probe against an isolated dev daemon covers:
  - the empty state and new wiki;
  - page edit and save;
  - a refused save showing both versions;
  - `wiki:` link resolution from an item body;
  - image paste rendering in preview and after save.

### Result (389b387) - 2026-11-09

Commits on `impl/develop/much-haiku-grove`: d62a564 (wiki page routes,
render wiki mode, `wiki:` links), 14c45cf (ticket Decisions: the wiki lives
in the Items panel), 265870b (merge develop), 9abd5fd (wiki in the Items
panel), a0cd827 (editor bar layout and refusal text from the probe), 389b387
(review fixes).

- Server: `GET /tracker/stores/<id>/wiki` lists `{name, tip, unpushed}`;
  `.../wiki/<name>/pages` returns `{pages, commit}`; `.../page?path=`
  returns `{path, content, commit, html}` (the host renders the page in
  wiki mode so the browser never renders twice); `POST .../save` takes
  `{base, newWiki, op, path, to, content, commit, message, attachments}`
  and answers 200 `{commit, merged, created, warnings}`, 409 `{conflict}`
  with both texts, 422 for a refused save (a held image that expired, an
  invalid name or path). `POST .../render` takes `wiki` and resolves
  `attachment:` oids from that wiki's `_attachments/` tree and the held
  pasted images. The renderer turns `[text](wiki:<name>/<path>)` into
  `<a data-wiki data-page>`; POSTs stay same-origin, localhost needs no token,
  and every new route is in `TestLANModeGuardsEveryRoute`.
- Browser (`tracker.js`, `tracker.css`, `dashboard.js`): the Items header
  carries the *Items | Wiki* switch (board (129) a); the narrow layout gets
  a Wiki tab and the store page the wide tree. Pieces per the mockup
  (board (133) a): wiki picker with tip and unpushed mark, "New wiki" form
  with the live name rule, page tree (index first, folders fold, path
  search), crumb and page menu (copy path, copy `wiki:` link for an agent),
  the page as the daemon rendered it with `wiki:` links marked (missing
  pages dotted), in-place edit through the shared editor with preview and
  image paste into the wiki branch, chord save and Esc cancel, the
  refused-save view with both texts and the overlapping lines marked (LCS
  per side with folded context), and the empty state. `wiki:` links in item
  bodies, item windows, and pages open the page; a missing page or a wiki
  the store does not have opens as a new page (new wiki) at that path. The
  mockup's `.wk-*` / `a.wlink` CSS was lifted into `tracker.css`; the
  mockup-only first-save refusal was not carried over. The adapter exposes
  `wiki.list/pages/read/save` and `render` takes `req.wiki`.
- State is per store in `ui.wiki`, read on demand; a save re-reads list,
  pages, and page; a tracker change event re-reads the wiki shown (the page
  is left alone while edited or while a refused save shows). 77 `js.tk.wk.*`
  keys in all eleven i18n tables (eight plural); the mockup `i18n-en.js` is
  regenerated.
- Verification: `TestPagesCarryTheTracker` pins the new markup hooks, the
  adapter's wiki calls, and the tab key; `node --check` on `tracker.js` and
  `dashboard.js`; `go vet`; `go test` on `./internal/wsdashboard/`,
  `./internal/wsi18n/`, `./internal/wswhiteboard/`, `./cmd/ws-mcp/` (the
  full suite was green before the develop merge). The headless Chromium probe
  against an isolated dev daemon (`WS_CONFIG_HOME`/`WS_CACHE_HOME` in the
  scratchpad, port 0, the user's daemons untouched) passes 60 checks over
  ten scenes: empty state and new wiki, page read with tree and marked
  links, more menu, edit and save, refused save with both texts and marks,
  image paste in preview and after save (served from the wiki image route),
  `wiki:` link from an item body and from the item window, narrow Wiki tab,
  store page, Esc and chord in the editor, an external save appearing in
  the open page, and a new page at an existing path refused. Ten
  screenshots were reviewed.
- Deviations: the tree search matches page paths only (no content search
  in the panel; `wiki.query search` covers content); the page shows no
  author or date line (history stays with the MCP tool); the refused view
  marks lines by LCS regions rather than the server's hunk positions.
- Review (lite): two Critical (stuck busy flags, stuck `saving`) and two
  Important (silent overwrite of an existing path by a new page, no refresh
  on tracker change) findings, all fixed in 389b387, which also folded in
  three Minors (link into an absent wiki, LCS prefix/suffix trim, uploads
  kept across a store switch). Minor left open: following a `wiki:` link
  from the editor's own preview or from an item window discards an unsaved
  edit without asking.
