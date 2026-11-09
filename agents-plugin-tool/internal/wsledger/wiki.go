package wsledger

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Wiki branches. A wiki is a plain git branch of the store repository,
// refs/heads/wiki/<name>, whose tree holds markdown pages (<path>.md at
// any depth) and the images they paste (_attachments/<sha256>.<ext>). It
// uses none of the event protocol: history, rename, delete, diff, and
// search are git's own. What the ledger lends it is the repository, the
// remote, the clone's signing key, and the identity state its commits are
// verified against.
//
// Sync. The store fetch also fetches refs/heads/wiki/* into the snapshot
// namespace, whole (--no-filter: the remote may be a promisor and every
// ledger git run forbids lazy fetches, so a filtered wiki fetch would leave
// pages and images unreadable) and negotiating only against wiki refs (a
// wiki image can share its blob with an item attachment whose commits this
// clone fetched filtered; a "have" on those would hide the blob). The local
// copy follows the remote: a fast-forward is taken; a remote tip that no
// longer descends from the local copy, or a deleted remote ref the local
// copy was once known to share, is followed too, with the replaced tip kept
// under refs/ledger/wiki-recovery/<name>/<oid> (local-only, never pushed,
// never pruned) and a warning. Deletion is judged against the never-pruned
// known value (refs/ledger/known/wiki/<name>), so a wiki this clone wrote
// before the store had a remote is not read as deleted: Sync pushes it.
//
// Saves. A save names the commit the edit started from (base). The clone
// commits the edit on base, fetches, and when the remote tip moved merges
// the two trees three-way (git merge-file per file; every structural
// conflict, modify/delete included, refuses), pushes with a lease on the
// tip it saw, and on a lost race fetches and merges again, never forcing.
// A conflict is refused as a *WikiConflict carrying both texts; nothing
// enters the branch. A store with a remote never commits a save it could
// not push: a transport failure refuses the save and the local ref stays.
// Without a remote the save moves the local ref alone.

const (
	wikiNamespace = "wiki/"
	wikiPrefix    = "refs/heads/" + wikiNamespace
	// wikiRecoveryPrefix holds the replaced tips of followed rewinds and
	// deletions: refs/ledger/wiki-recovery/<name>/<oid>.
	wikiRecoveryPrefix = "refs/ledger/wiki-recovery/"
	// WikiAttachDir is the wiki tree folder holding pasted images.
	WikiAttachDir = "_attachments"
	// WikiNameMax bounds a wiki name.
	WikiNameMax = 64
	wikiPathMax = 512
	// wikiPushTries bounds the fetch-merge-push loop of one save.
	wikiPushTries = 5
	pageExt       = ".md"
)

// Wiki edit operations.
const (
	WikiOpWrite   = "write"
	WikiOpRename  = "rename"
	WikiOpDelete  = "delete"
	WikiOpRestore = "restore"
)

// WikiConflict kinds.
const (
	// WikiConflictStaleBase: the edit's base is not an ancestor of the
	// current tip (the ref is absent, rewound, or the base is unknown).
	WikiConflictStaleBase = "stale-base"
	// WikiConflictText: both sides changed overlapping lines of one page.
	WikiConflictText = "text"
	// WikiConflictModifyDelete: one side changed what the other deleted.
	WikiConflictModifyDelete = "modify-delete"
	// WikiConflictAddAdd: both sides added the same path with different
	// content.
	WikiConflictAddAdd = "add-add"
	// WikiConflictTreeFile: a folder on one side is a file on the other.
	WikiConflictTreeFile = "tree-file"
	// WikiConflictRename: the page a rename or delete names was renamed
	// or removed on the other side.
	WikiConflictRename = "rename"
)

var (
	ErrWikiBadName       = errors.New("ledger: a wiki name is [a-z0-9][a-z0-9-]*, at most 64 characters")
	ErrWikiBadPath       = errors.New("ledger: a page path is folder segments and a name, without ., .., a leading _attachments, a .md suffix, or the characters : \\ * ? \" < > |")
	ErrWikiNotFound      = errors.New("ledger: no such wiki")
	ErrWikiExists        = errors.New("ledger: the wiki already exists; save with its base instead of new_wiki")
	ErrWikiNoBase        = errors.New("ledger: a save needs the base commit the edit started from (new_wiki creates a wiki without one)")
	ErrWikiPageNotFound  = errors.New("ledger: no such page")
	ErrWikiPageExists    = errors.New("ledger: the target page already exists")
	ErrWikiNoChange      = errors.New("ledger: the edit changes nothing")
	ErrWikiImageNotFound = errors.New("ledger: no such wiki image")
	ErrWikiBadOp         = errors.New("ledger: op is write, rename, delete, or restore")
	// ErrWikiBadEdit wraps the other malformed-edit refusals of a save.
	ErrWikiBadEdit = errors.New("ledger: malformed wiki edit")
)

var (
	wikiNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	wikiExtRe  = regexp.MustCompile(`^[a-z0-9]{1,8}$`)
	// wikiAttachmentRefRe finds the attachment:<sha256> references a page
	// makes, the syntax item bodies use.
	wikiAttachmentRefRe = regexp.MustCompile(`attachment:([0-9a-f]{64})`)
)

// ValidWikiName reports whether s is a wiki name.
func ValidWikiName(s string) bool { return len(s) <= WikiNameMax && wikiNameRe.MatchString(s) }

// ValidWikiPath reports whether s is a page path: one or more non-empty
// segments joined by "/", none "." or "..", none ending in ".md" (the
// file of page "a" is a.md, so a folder a.md would collide with it), the
// first not the attachments folder, with no control characters and none
// of the characters a Windows checkout refuses.
func ValidWikiPath(s string) bool {
	if s == "" || len(s) > wikiPathMax || strings.ContainsAny(s, ":\\*?\"<>|") {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	segs := strings.Split(s, "/")
	if segs[0] == WikiAttachDir {
		return false
	}
	for _, seg := range segs {
		if seg == "" || seg == "." || seg == ".." || strings.HasSuffix(strings.ToLower(seg), pageExt) || strings.TrimSpace(seg) != seg {
			return false
		}
	}
	return true
}

func wikiRef(name string) string              { return wikiPrefix + name }
func wikiRecoveryRef(name, oid string) string { return wikiRecoveryPrefix + name + "/" + oid }
func pageFile(path string) string             { return path + pageExt }
func pageParts(path string) []string          { return strings.Split(pageFile(path), "/") }
func wikiImageParts(oid, ext string) []string { return []string{WikiAttachDir, oid + "." + ext} }
func wikiNameOf(ref string) (string, bool) {
	n, ok := strings.CutPrefix(ref, wikiPrefix)
	return n, ok && ValidWikiName(n)
}

// WikiImage is one image a write commits beside its page, at
// _attachments/<OID>.<Ext>. Data must hash to OID (the sha256 of the
// content, lowercase hex); Ext is the caller's type decision.
type WikiImage struct {
	OID  string
	Ext  string
	Data []byte
}

// WikiEdit is one save. Base is the commit the edit started from (a page
// read returns it); NewWiki creates the wiki with this save instead, and
// is refused when the ref exists. Op is one of the WikiOp constants: write
// puts Content at Path (with Images); rename moves Path to To; delete
// removes Path; restore puts the page at Path as commit Commit held it,
// with the images it references that the base lacks.
type WikiEdit struct {
	Wiki    string
	Base    string
	NewWiki bool
	Op      string
	Path    string
	To      string
	Content string
	Commit  string
	Images  []WikiImage
	// Message is the optional commit message body under the generated
	// subject.
	Message string
}

// WikiSaveResult is a landed save.
type WikiSaveResult struct {
	// Commit is the new tip (the merge commit when the save merged).
	Commit string
	// Merged is true when the remote had moved and the save merged.
	Merged bool
	// Created is true when the save created the wiki.
	Created bool
	// Warnings are the anomalies the save's fetch found.
	Warnings []Warning
}

// WikiConflict is a refused save: the edit cannot be applied cleanly on
// the current tip. Theirs is that tip, Base the edit's base, and the two
// texts are the saver's page and the page as Theirs holds it ("" when
// absent); the saver merges them and saves again with Theirs as base.
type WikiConflict struct {
	Wiki, Path string
	Kind       string
	Reason     string
	Base       string
	Theirs     string
	OursText   string
	TheirsText string
}

func (c *WikiConflict) Error() string {
	return fmt.Sprintf("ledger: wiki %s: %s refused on %s: %s", c.Wiki, c.Kind, c.Path, c.Reason)
}

// WikiInfo is one wiki of the store.
type WikiInfo struct {
	Name string
	Tip  string
	// Unpushed is true when the store has a remote the tip is not known to
	// hold (a wiki written before the store had a remote).
	Unpushed bool
}

// WikiCommit is one commit of a wiki's history.
type WikiCommit struct {
	OID     string
	Parents []string
	// Author is the committing member's id as the commit records it,
	// which only Verified vouches for.
	Author  string
	Time    time.Time
	Message string
	// Verified is true when the signature is good and its key is a key a
	// store member holds or held; Signer is that member. Unverified says
	// why not.
	Verified   bool
	Signer     string
	Unverified string
}

// WikiMatch is one search hit.
type WikiMatch struct {
	Path string
	Line int
	Text string
}

// WikiRecoveryPoint is a replaced tip kept by a followed rewind or
// deletion.
type WikiRecoveryPoint struct {
	Wiki    string
	OID     string
	Time    time.Time
	Subject string
}

// Fetch and absorb.

// fetchWikiRefs sets the wiki snapshot namespace to the remote's wiki
// branches, whole; the file comment says why the filter is off and the
// negotiation tips are the wiki refs alone.
func (g *gitRepo) fetchWikiRefs(ctx context.Context) error {
	_, err := g.fetchNamespace(ctx, wikiNamespace, "--no-filter",
		"--negotiation-tip="+wikiPrefix+"*", "--negotiation-tip="+snapPrefix+wikiNamespace+"*")
	return err
}

// fetchWiki fetches and absorbs the wiki refs alone (a save's fetch).
func (l *Ledger) fetchWiki(ctx context.Context) error {
	if err := l.git.fetchWikiRefs(ctx); err != nil {
		return fmt.Errorf("ledger: wiki fetch: %w", err)
	}
	return l.absorbWiki(ctx)
}

func (l *Ledger) wikiRefs(ctx context.Context) (local, snap, known map[string]string, err error) {
	if local, err = l.git.listRefs(ctx, wikiPrefix); err != nil {
		return nil, nil, nil, err
	}
	snapRaw, err := l.git.listRefs(ctx, snapPrefix+wikiNamespace)
	if err != nil {
		return nil, nil, nil, err
	}
	knownRaw, err := l.git.listRefs(ctx, knownPrefix+wikiNamespace)
	if err != nil {
		return nil, nil, nil, err
	}
	return local, rekey(snapRaw, snapPrefix), rekey(knownRaw, knownPrefix), nil
}

// absorbWiki applies the wiki snapshot to the local copies under the
// follow-the-remote rules and records the anomalies in l.wikiWarns.
func (l *Ledger) absorbWiki(ctx context.Context) error {
	local, snap, known, err := l.wikiRefs(ctx)
	if err != nil {
		return err
	}
	names := map[string]bool{}
	for n := range local {
		names[n] = true
	}
	for n := range snap {
		names[n] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)
	l.wikiWarns = nil
	warn := func(ref, kind, detail string) {
		l.wikiWarns = append(l.wikiWarns, Warning{Ref: ref, Kind: kind, Detail: detail})
	}
	recover := func(name, ref, oid string) error {
		// The recovery ref may exist from an earlier follow of the same tip.
		if err := l.git.updateRef(ctx, wikiRecoveryRef(name, oid), oid, ""); err != nil {
			if refs, lerr := l.git.listRefs(ctx, wikiRecoveryRef(name, oid)); lerr != nil || refs[wikiRecoveryRef(name, oid)] != oid {
				return err
			}
		}
		return nil
	}
	for _, ref := range sorted {
		L, S, K := local[ref], snap[ref], known[ref]
		name, ok := wikiNameOf(ref)
		if !ok {
			if S != "" && L == "" {
				warn(ref, WarnBadRef, "a remote ref under the wiki namespace that is not a wiki name; not taken")
			}
			continue
		}
		switch {
		case S == "":
			if L != "" && K != "" {
				if err := recover(name, ref, L); err != nil {
					return err
				}
				if err := l.git.deleteRef(ctx, ref, L); err != nil {
					return err
				}
				warn(ref, WarnDeleted, "deleted on the remote; followed, and the local tip is kept under "+wikiRecoveryRef(name, L))
			}
			// L != "" && K == "": a wiki this clone wrote before the store
			// had a remote; pushLocalWikis publishes it.
		case L == "":
			if err := l.git.updateRef(ctx, ref, S, ""); err != nil {
				return err
			}
			if err := l.setKnown(ctx, ref, S); err != nil {
				return err
			}
		case L == S:
			if K != S {
				if err := l.setKnown(ctx, ref, S); err != nil {
					return err
				}
			}
		case l.git.isAncestor(ctx, L, S):
			if err := l.git.updateRef(ctx, ref, S, L); err != nil {
				return err
			}
			if err := l.setKnown(ctx, ref, S); err != nil {
				return err
			}
		case K == "" && l.git.isAncestor(ctx, S, L):
			// Never pushed, and ahead of the remote: pushLocalWikis
			// fast-forwards the remote under a lease on S.
		default:
			if err := recover(name, ref, L); err != nil {
				return err
			}
			if err := l.git.updateRef(ctx, ref, S, L); err != nil {
				return err
			}
			if err := l.setKnown(ctx, ref, S); err != nil {
				return err
			}
			warn(ref, WarnRewound, "moved on the remote to a non-descendant of the local copy; followed, and the local tip is kept under "+wikiRecoveryRef(name, L))
		}
	}
	return nil
}

// pushLocalWikis pushes every wiki the remote is not known to hold (one
// written before the store had a remote) under a lease on the last
// snapshot of the remote ref: create-only when the remote has none, a
// fast-forward when the local copy descends from it (absorbWiki left that
// case alone). A rejection (the remote moved meanwhile) is left to the
// fetch that follows, which follows the remote and keeps the local tip as
// a recovery point. A push that fails (a remote refusal, a transport
// failure) is returned in failed, by ref, and the other wikis are still
// pushed: one wiki the remote refuses must not hold up the ledger's sync
// or saves to other wikis. err is a local failure.
func (l *Ledger) pushLocalWikis(ctx context.Context) (failed map[string]error, err error) {
	local, snap, known, err := l.wikiRefs(ctx)
	if err != nil {
		return nil, err
	}
	refs := make([]string, 0, len(local))
	for ref := range local {
		if _, ok := wikiNameOf(ref); ok && known[ref] == "" {
			refs = append(refs, ref)
		}
	}
	sort.Strings(refs)
	for _, ref := range refs {
		lease := snap[ref]
		out, err := l.git.push(ctx, ref, &lease)
		if err != nil {
			if failed == nil {
				failed = map[string]error{}
			}
			failed[ref] = fmt.Errorf("ledger: wiki push: %w", err)
			continue
		}
		if out == pushOK {
			if err := l.setKnown(ctx, ref, local[ref]); err != nil {
				return failed, err
			}
		}
	}
	return failed, nil
}

// wikiPushWarnings reports the failed pushes of pushLocalWikis, by ref.
func wikiPushWarnings(failed map[string]error) []Warning {
	refs := make([]string, 0, len(failed))
	for ref := range failed {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	out := make([]Warning, 0, len(refs))
	for _, ref := range refs {
		out = append(out, Warning{Ref: ref, Kind: WarnWikiSyncFailed,
			Detail: failed[ref].Error() + "; the wiki stays on this clone and the next sync retries"})
	}
	return out
}

// Reads.

func (l *Ledger) wikiTip(ctx context.Context, name string) (string, error) {
	refs, err := l.git.listRefs(ctx, wikiRef(name))
	if err != nil {
		return "", err
	}
	return refs[wikiRef(name)], nil
}

// wikiCommitArg resolves the commit a read names: the tip when "", else
// an object id this clone holds.
func (l *Ledger) wikiCommitArg(ctx context.Context, name, commit string) (string, error) {
	if !ValidWikiName(name) {
		return "", ErrWikiBadName
	}
	if commit == "" {
		tip, err := l.wikiTip(ctx, name)
		if err != nil {
			return "", err
		}
		if tip == "" {
			return "", ErrWikiNotFound
		}
		return tip, nil
	}
	if !oidRe.MatchString(commit) {
		return "", fmt.Errorf("ledger: %q is not a commit id", commit)
	}
	if _, err := l.git.treeOf(ctx, commit); err != nil {
		return "", fmt.Errorf("ledger: commit %s is not in this clone", commit)
	}
	return commit, nil
}

// WikiList lists the store's wikis, sorted by name.
func (l *Ledger) WikiList(ctx context.Context) ([]WikiInfo, error) {
	var out []WikiInfo
	err := l.withLock(ctx, func() error {
		local, _, known, err := l.wikiRefs(ctx)
		if err != nil {
			return err
		}
		remote := l.hasRemote(ctx)
		for ref, tip := range local {
			name, ok := wikiNameOf(ref)
			if !ok {
				continue
			}
			out = append(out, WikiInfo{Name: name, Tip: tip, Unpushed: remote && known[ref] != tip})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return nil
	})
	return out, err
}

// WikiPages lists the pages of wiki name at commit (the tip when ""),
// sorted, and returns the commit read.
func (l *Ledger) WikiPages(ctx context.Context, name, commit string) ([]string, string, error) {
	var pages []string
	var at string
	err := l.withLock(ctx, func() error {
		var err error
		if at, err = l.wikiCommitArg(ctx, name, commit); err != nil {
			return err
		}
		tree, err := l.git.treeOf(ctx, at)
		if err != nil {
			return err
		}
		return l.git.walkTree(ctx, tree, "", func(path string, e treeEntry) error {
			if strings.HasPrefix(path, WikiAttachDir+"/") {
				return nil
			}
			if p, ok := strings.CutSuffix(path, pageExt); ok && e.mode == "100644" {
				pages = append(pages, p)
			}
			return nil
		})
	})
	sort.Strings(pages)
	return pages, at, err
}

// WikiRead returns page path of wiki name at commit (the tip when "") and
// the commit it read from, which a save on it names as its base.
// ErrWikiPageNotFound when the page is absent there.
func (l *Ledger) WikiRead(ctx context.Context, name, path, commit string) (string, string, error) {
	var content, at string
	err := l.withLock(ctx, func() error {
		if !ValidWikiPath(path) {
			return ErrWikiBadPath
		}
		var err error
		if at, err = l.wikiCommitArg(ctx, name, commit); err != nil {
			return err
		}
		data, ok, err := l.git.readBlob(ctx, at+":"+pageFile(path))
		if err != nil {
			return err
		}
		if !ok {
			return ErrWikiPageNotFound
		}
		content = string(data)
		return nil
	})
	return content, at, err
}

// WikiSearch finds the lines of wiki name's pages at the tip that contain
// query (fixed string, case-insensitive) and returns the tip searched.
func (l *Ledger) WikiSearch(ctx context.Context, name, query string) ([]WikiMatch, string, error) {
	var matches []WikiMatch
	var at string
	err := l.withLock(ctx, func() error {
		if strings.TrimSpace(query) == "" {
			return errors.New("ledger: an empty search")
		}
		var err error
		if at, err = l.wikiCommitArg(ctx, name, ""); err != nil {
			return err
		}
		out, err := l.git.run(ctx, "-c", "core.quotePath=false", "grep", "-n", "-i", "-I", "--no-color",
			"--fixed-strings", "-e", query, at, "--", "*"+pageExt, ":(exclude)"+WikiAttachDir+"/*")
		if err != nil {
			// Exit status 1 is no match.
			if strings.Contains(err.Error(), "exit status 1") {
				return nil
			}
			return err
		}
		for _, line := range strings.Split(string(out), "\n") {
			rest, ok := strings.CutPrefix(line, at+":")
			if !ok {
				continue
			}
			// Page paths hold no ":", so the first two cuts are exact.
			file, rest, ok := strings.Cut(rest, ":")
			if !ok {
				continue
			}
			ln, text, ok := strings.Cut(rest, ":")
			if !ok {
				continue
			}
			n, err := strconv.Atoi(ln)
			p, isPage := strings.CutSuffix(file, pageExt)
			if err != nil || !isPage {
				continue
			}
			matches = append(matches, WikiMatch{Path: p, Line: n, Text: text})
		}
		return nil
	})
	return matches, at, err
}

// WikiHistory lists the commits of wiki name that touched page path (every
// commit when path is ""), newest first, at most limit when positive,
// each with its signature verdict against the store's member keys.
func (l *Ledger) WikiHistory(ctx context.Context, name, path string, limit int) ([]WikiCommit, error) {
	var commits []WikiCommit
	err := l.withLock(ctx, func() error {
		if path != "" && !ValidWikiPath(path) {
			return ErrWikiBadPath
		}
		tip, err := l.wikiCommitArg(ctx, name, "")
		if err != nil {
			return err
		}
		// The identity state the signatures are judged against.
		if err := l.refresh(ctx); err != nil {
			return err
		}
		n := nonce()
		args := []string{"-c", "i18n.logOutputEncoding=UTF-8", "log", "--no-color", "--no-decorate", "--no-show-signature",
			"--format=%x00" + n + " %H %P%n%an%n%at%n%B"}
		if limit > 0 {
			args = append(args, "--max-count="+strconv.Itoa(limit))
		}
		args = append(args, tip, "--")
		if path != "" {
			args = append(args, pageFile(path))
		}
		out, err := l.git.run(ctx, args...)
		if err != nil {
			return err
		}
		var oids []string
		for _, rec := range strings.Split(string(out), "\x00"+n+" ")[1:] {
			head, rest, ok := strings.Cut(rec, "\n")
			if !ok {
				continue
			}
			fields := strings.Fields(head)
			if len(fields) == 0 || !oidRe.MatchString(fields[0]) {
				continue
			}
			author, rest, _ := strings.Cut(rest, "\n")
			ts, msg, _ := strings.Cut(rest, "\n")
			sec, _ := strconv.ParseInt(strings.TrimSpace(ts), 10, 64)
			commits = append(commits, WikiCommit{OID: fields[0], Parents: fields[1:], Author: author,
				Time: time.Unix(sec, 0).UTC(), Message: strings.TrimRight(msg, "\n")})
			oids = append(oids, fields[0])
		}
		return l.wikiVerify(ctx, commits, oids)
	})
	return commits, err
}

// wikiVerify fills the signature verdicts of commits: a good signature by
// a key some member holds, or held before a re-key (as attachment commits
// are judged), is verified and names its member.
func (l *Ledger) wikiVerify(ctx context.Context, commits []WikiCommit, oids []string) error {
	if len(oids) == 0 {
		return nil
	}
	res, err := l.git.verifySigs(ctx, oids, allowedSigners(l.genesis.roles, l.refCommitList(), l.claims))
	if err != nil {
		return err
	}
	byFP := map[string]string{}
	if l.view != nil && l.view.ident != nil {
		for id, m := range l.view.ident.members {
			for _, k := range m.keys {
				byFP[k.fp] = id
			}
			for _, r := range m.revoked {
				byFP[r.key.fp] = id
			}
		}
	}
	for i := range commits {
		c := &commits[i]
		info := res[c.OID]
		switch {
		case info.state == sigUnsigned:
			c.Unverified = "unsigned"
		case info.state != sigGood || info.fp == "":
			c.Unverified = "signature does not verify"
		case byFP[info.fp] == "":
			c.Unverified = "signed with " + info.fp + ", not a key of any member"
		default:
			c.Verified, c.Signer = true, byFP[info.fp]
		}
	}
	return nil
}

// WikiDiff returns the patch of page path between commits from and to of
// wiki name; from "" diffs to against its first parent (a root commit
// against nothing).
func (l *Ledger) WikiDiff(ctx context.Context, name, path, from, to string) (string, error) {
	var patch string
	err := l.withLock(ctx, func() error {
		if !ValidWikiPath(path) {
			return ErrWikiBadPath
		}
		if to == "" {
			return errors.New("ledger: a diff needs the commit to diff (to)")
		}
		var err error
		if to, err = l.wikiCommitArg(ctx, name, to); err != nil {
			return err
		}
		args := []string{"-c", "core.quotePath=false", "diff-tree", "-p", "-r", "--no-color", "--no-ext-diff", "--no-textconv"}
		switch {
		case from != "":
			if from, err = l.wikiCommitArg(ctx, name, from); err != nil {
				return err
			}
			args = append(args, from, to)
		default:
			out, err := l.git.run(ctx, "rev-list", "--parents", "-n", "1", to)
			if err != nil {
				return err
			}
			if f := strings.Fields(string(out)); len(f) > 1 {
				args = append(args, f[1], to)
			} else {
				args = append(args, "--root", to)
			}
		}
		args = append(args, "--", pageFile(path))
		out, err := l.git.run(ctx, args...)
		if err != nil {
			return err
		}
		patch = string(out)
		return nil
	})
	return patch, err
}

// WikiRecovery lists the recovery points of wiki name (every wiki's when
// ""), newest first.
func (l *Ledger) WikiRecovery(ctx context.Context, name string) ([]WikiRecoveryPoint, error) {
	var points []WikiRecoveryPoint
	err := l.withLock(ctx, func() error {
		prefix := wikiRecoveryPrefix
		if name != "" {
			if !ValidWikiName(name) {
				return ErrWikiBadName
			}
			prefix += name + "/"
		}
		refs, err := l.git.listRefs(ctx, prefix)
		if err != nil {
			return err
		}
		var oids []string
		seen := map[string]bool{}
		for ref, oid := range refs {
			rest := strings.TrimPrefix(ref, wikiRecoveryPrefix)
			wiki, kept, ok := strings.Cut(rest, "/")
			if !ok || kept != oid || !ValidWikiName(wiki) {
				continue
			}
			points = append(points, WikiRecoveryPoint{Wiki: wiki, OID: oid})
			if !seen[oid] {
				seen[oid] = true
				oids = append(oids, oid)
			}
		}
		if len(oids) == 0 {
			return nil
		}
		// The same commit may be a recovery point of two wikis.
		byOID := map[string][]int{}
		for i, p := range points {
			byOID[p.OID] = append(byOID[p.OID], i)
		}
		args := []string{"-c", "i18n.logOutputEncoding=UTF-8", "log", "--no-walk=unsorted", "--no-color", "--no-decorate", "--no-show-signature", "--format=%H %at %s"}
		args = append(args, oids...)
		args = append(args, "--")
		out, err := l.git.run(ctx, args...)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(out), "\n") {
			f := strings.SplitN(line, " ", 3)
			if len(f) < 2 {
				continue
			}
			sec, _ := strconv.ParseInt(f[1], 10, 64)
			for _, i := range byOID[f[0]] {
				points[i].Time = time.Unix(sec, 0).UTC()
				if len(f) == 3 {
					points[i].Subject = f[2]
				}
			}
		}
		sort.Slice(points, func(i, j int) bool {
			if !points[i].Time.Equal(points[j].Time) {
				return points[i].Time.After(points[j].Time)
			}
			return points[i].OID < points[j].OID
		})
		return nil
	})
	return points, err
}

// WikiImageRead returns the image of oid in wiki name's tip tree
// (_attachments/<oid>.<ext>), checking that the content hashes to oid.
func (l *Ledger) WikiImageRead(ctx context.Context, name, oid string) ([]byte, error) {
	var data []byte
	err := l.withLock(ctx, func() error {
		if !ValidAttachmentOID(oid) {
			return ErrWikiImageNotFound
		}
		tip, err := l.wikiCommitArg(ctx, name, "")
		if err != nil {
			return err
		}
		tree, err := l.git.treeOf(ctx, tip)
		if err != nil {
			return err
		}
		dir, ok, err := l.git.entryAt(ctx, tree, WikiAttachDir)
		if err != nil || !ok || !dir.isTree() {
			return firstErr(err, ErrWikiImageNotFound)
		}
		entries, err := l.git.lsTree(ctx, dir.oid)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if !strings.HasPrefix(e.name, oid+".") || e.mode != "100644" {
				continue
			}
			blob, ok, err := l.git.readBlob(ctx, e.oid)
			if err != nil {
				return err
			}
			if ok && blobOID(blob) == oid {
				data = blob
				return nil
			}
		}
		return ErrWikiImageNotFound
	})
	return data, err
}

// WikiImages lists the oids of the images in wiki name's tip tree
// (_attachments/<oid>.<ext>), the attachment: references a page of it may
// show. It reads no content; WikiImageRead checks the hash when one is
// served.
func (l *Ledger) WikiImages(ctx context.Context, name string) ([]string, error) {
	var oids []string
	err := l.withLock(ctx, func() error {
		tip, err := l.wikiCommitArg(ctx, name, "")
		if err != nil {
			return err
		}
		tree, err := l.git.treeOf(ctx, tip)
		if err != nil {
			return err
		}
		dir, ok, err := l.git.entryAt(ctx, tree, WikiAttachDir)
		if err != nil || !ok || !dir.isTree() {
			return err
		}
		entries, err := l.git.lsTree(ctx, dir.oid)
		if err != nil {
			return err
		}
		for _, e := range entries {
			oid, _, found := strings.Cut(e.name, ".")
			if found && e.mode == "100644" && ValidAttachmentOID(oid) {
				oids = append(oids, oid)
			}
		}
		return nil
	})
	return oids, err
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// Saves.

func checkWikiEdit(e *WikiEdit) error {
	if !ValidWikiName(e.Wiki) {
		return ErrWikiBadName
	}
	switch e.Op {
	case WikiOpWrite, WikiOpRename, WikiOpDelete, WikiOpRestore:
	default:
		return ErrWikiBadOp
	}
	if !ValidWikiPath(e.Path) {
		return ErrWikiBadPath
	}
	if e.Op == WikiOpRename && !ValidWikiPath(e.To) {
		return fmt.Errorf("to: %w", ErrWikiBadPath)
	}
	if e.Op == WikiOpRestore && !oidRe.MatchString(e.Commit) {
		return fmt.Errorf("%w: restore needs the commit to restore the page from", ErrWikiBadEdit)
	}
	switch {
	case e.NewWiki && e.Base != "":
		return fmt.Errorf("%w: new_wiki and base exclude each other", ErrWikiBadEdit)
	case !e.NewWiki && e.Base == "":
		return ErrWikiNoBase
	case !e.NewWiki && !oidRe.MatchString(e.Base):
		return fmt.Errorf("%w: base %q is not a commit id", ErrWikiBadEdit, e.Base)
	}
	if e.Op != WikiOpWrite && len(e.Images) > 0 {
		return fmt.Errorf("%w: images go with a write", ErrWikiBadEdit)
	}
	for _, img := range e.Images {
		if !ValidAttachmentOID(img.OID) || !wikiExtRe.MatchString(img.Ext) {
			return fmt.Errorf("%w: image %q.%q is not a sha256 oid with a short extension", ErrWikiBadEdit, img.OID, img.Ext)
		}
		if got := blobOID(img.Data); got != img.OID {
			return fmt.Errorf("%w: %s hashes to %s", ErrBlobMismatch, img.OID, got)
		}
	}
	return nil
}

// wikiApply returns base's tree with the edit applied.
func (l *Ledger) wikiApply(ctx context.Context, base string, e WikiEdit) (string, error) {
	g := l.git
	tree := base
	switch e.Op {
	case WikiOpWrite:
		blob, err := g.hashObject(ctx, "blob", []byte(e.Content))
		if err != nil {
			return "", err
		}
		if tree, err = g.putPath(ctx, tree, pageParts(e.Path), blob); err != nil {
			return "", err
		}
		for _, img := range e.Images {
			oid, err := g.hashObject(ctx, "blob", img.Data)
			if err != nil {
				return "", err
			}
			if tree, err = g.putPath(ctx, tree, wikiImageParts(img.OID, img.Ext), oid); err != nil {
				return "", err
			}
		}
	case WikiOpRename:
		from, ok, err := g.entryAt(ctx, tree, pageFile(e.Path))
		if err != nil {
			return "", err
		}
		if !ok || from.isTree() {
			return "", ErrWikiPageNotFound
		}
		if _, exists, err := g.entryAt(ctx, tree, pageFile(e.To)); err != nil {
			return "", err
		} else if exists {
			return "", ErrWikiPageExists
		}
		if tree, err = g.removePath(ctx, tree, pageParts(e.Path)); err != nil {
			return "", err
		}
		if tree, err = g.putPath(ctx, tree, pageParts(e.To), from.oid); err != nil {
			return "", err
		}
	case WikiOpDelete:
		_, ok, err := g.entryAt(ctx, tree, pageFile(e.Path))
		if err != nil {
			return "", err
		}
		if !ok {
			return "", ErrWikiPageNotFound
		}
		if tree, err = g.removePath(ctx, tree, pageParts(e.Path)); err != nil {
			return "", err
		}
	case WikiOpRestore:
		srcTree, err := g.treeOf(ctx, e.Commit)
		if err != nil {
			return "", fmt.Errorf("ledger: commit %s is not in this clone", e.Commit)
		}
		page, ok, err := g.entryAt(ctx, srcTree, pageFile(e.Path))
		if err != nil {
			return "", err
		}
		if !ok || page.isTree() {
			return "", fmt.Errorf("%w at %s", ErrWikiPageNotFound, e.Commit)
		}
		if tree, err = g.putPath(ctx, tree, pageParts(e.Path), page.oid); err != nil {
			return "", err
		}
		// The images the page references come back with it, when the
		// source still holds them and the base does not.
		content, _, err := g.readBlob(ctx, page.oid)
		if err != nil {
			return "", err
		}
		srcDir, ok, err := g.entryAt(ctx, srcTree, WikiAttachDir)
		if err != nil || !ok || !srcDir.isTree() {
			return tree, err
		}
		srcImages, err := g.lsTree(ctx, srcDir.oid)
		if err != nil {
			return "", err
		}
		for _, m := range wikiAttachmentRefRe.FindAllStringSubmatch(string(content), -1) {
			for _, img := range srcImages {
				if !strings.HasPrefix(img.name, m[1]+".") || img.isTree() {
					continue
				}
				if _, have, err := g.entryAt(ctx, tree, WikiAttachDir+"/"+img.name); err != nil {
					return "", err
				} else if !have {
					if tree, err = g.putPath(ctx, tree, []string{WikiAttachDir, img.name}, img.oid); err != nil {
						return "", err
					}
				}
			}
		}
	}
	return tree, nil
}

// wikiSubject is the commit subject of an edit.
func wikiSubject(e WikiEdit) string {
	s := fmt.Sprintf("wiki(%s): %s %s", e.Wiki, e.Op, e.Path)
	switch e.Op {
	case WikiOpRename:
		s += " -> " + e.To
	case WikiOpRestore:
		s += " from " + e.Commit[:12]
	}
	return s
}

func (l *Ledger) wikiCommit(ctx context.Context, tree string, parents []string, e WikiEdit, merge bool) (string, error) {
	msg := wikiSubject(e)
	if merge {
		msg += " (merged)"
	}
	if body := strings.TrimSpace(e.Message); body != "" {
		msg += "\n\n" + body
	}
	return l.git.commitTreeParents(ctx, tree, parents, []byte(msg+"\n"), l.member, l.keyPath)
}

// treeConflict is what mergeTrees could not resolve.
type treeConflict struct {
	Path string // the tree path (a page file or an image)
	Kind string
}

// mergeTrees merges ours and theirs against base, tree by tree: an entry
// changed on one side only is taken, one changed the same way on both is
// taken, a page both changed differently goes through git merge-file, and
// everything else (a change against a deletion, two different additions,
// a folder against a file, differing modes) is a conflict.
func (g *gitRepo) mergeTrees(ctx context.Context, base, ours, theirs, prefix string) (string, *treeConflict, error) {
	switch {
	case ours == theirs || base == theirs:
		return ours, nil, nil
	case base == ours:
		return theirs, nil, nil
	}
	list := func(tree string) (map[string]treeEntry, error) {
		out := map[string]treeEntry{}
		if tree == "" {
			return out, nil
		}
		entries, err := g.lsTree(ctx, tree)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			out[e.name] = e
		}
		return out, nil
	}
	B, err := list(base)
	if err != nil {
		return "", nil, err
	}
	O, err := list(ours)
	if err != nil {
		return "", nil, err
	}
	T, err := list(theirs)
	if err != nil {
		return "", nil, err
	}
	names := map[string]bool{}
	for n := range B {
		names[n] = true
	}
	for n := range O {
		names[n] = true
	}
	for n := range T {
		names[n] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)
	same := func(a, b treeEntry, aok, bok bool) bool {
		return aok == bok && (!aok || (a.oid == b.oid && a.isTree() == b.isTree() && (a.isTree() || a.mode == b.mode)))
	}
	var result []treeEntry
	for _, name := range sorted {
		b, bok := B[name]
		o, ook := O[name]
		t, tok := T[name]
		path := prefix + name
		switch {
		case same(o, t, ook, tok):
			if ook {
				result = append(result, o)
			}
		case same(o, b, ook, bok):
			if tok {
				result = append(result, t)
			}
		case same(t, b, tok, bok):
			if ook {
				result = append(result, o)
			}
		case !ook || !tok:
			return "", &treeConflict{Path: path, Kind: WikiConflictModifyDelete}, nil
		case o.isTree() != t.isTree():
			return "", &treeConflict{Path: path, Kind: WikiConflictTreeFile}, nil
		case o.isTree():
			sub := ""
			if bok && b.isTree() {
				sub = b.oid
			}
			merged, conflict, err := g.mergeTrees(ctx, sub, o.oid, t.oid, path+"/")
			if err != nil || conflict != nil {
				return "", conflict, err
			}
			if merged != "" {
				if entries, err := g.lsTree(ctx, merged); err != nil {
					return "", nil, err
				} else if len(entries) > 0 {
					result = append(result, treeEntry{mode: "40000", name: name, oid: merged})
				}
			}
		case o.mode != t.mode || o.mode != "100644":
			return "", &treeConflict{Path: path, Kind: WikiConflictTreeFile}, nil
		case !bok || b.isTree():
			return "", &treeConflict{Path: path, Kind: WikiConflictAddAdd}, nil
		default:
			blob, conflict, err := g.mergeFile(ctx, b.oid, o.oid, t.oid)
			if err != nil {
				return "", nil, err
			}
			if conflict {
				return "", &treeConflict{Path: path, Kind: WikiConflictText}, nil
			}
			result = append(result, treeEntry{mode: "100644", name: name, oid: blob})
		}
	}
	tree, err := g.writeTree(ctx, result)
	return tree, nil, err
}

// WikiSave applies one edit as the file comment describes and returns the
// new tip. It refuses with ErrWikiExists (new_wiki on an existing wiki), a
// *WikiConflict (a stale base, or an edit the current tip cannot take
// cleanly), ErrWikiNoChange, or the page errors of the operation.
func (l *Ledger) WikiSave(ctx context.Context, e WikiEdit) (WikiSaveResult, error) {
	var res WikiSaveResult
	if err := checkWikiEdit(&e); err != nil {
		return res, err
	}
	var pushWarns []Warning
	err := l.withLock(ctx, func() error {
		remote := l.hasRemote(ctx)
		if remote {
			if err := l.fetchWiki(ctx); err != nil {
				return err
			}
			// A wiki written before the store had a remote is published
			// first, so the save below pushes onto a ref the remote holds.
			// Another wiki the remote refuses is only reported; this one
			// refuses the save.
			failed, err := l.pushLocalWikis(ctx)
			if err != nil {
				return err
			}
			if err := failed[wikiRef(e.Wiki)]; err != nil {
				return err
			}
			pushWarns = wikiPushWarnings(failed)
		}
		ref := wikiRef(e.Wiki)
		tip, err := l.wikiTip(ctx, e.Wiki)
		if err != nil {
			return err
		}
		stale := func(tip string) error {
			return l.wikiConflict(ctx, e, tip, "", &treeConflict{Path: pageFile(e.Path), Kind: WikiConflictStaleBase})
		}
		baseTree := ""
		if e.NewWiki {
			if tip != "" {
				return ErrWikiExists
			}
		} else {
			if tip == "" || !l.git.isAncestor(ctx, e.Base, tip) {
				return stale(tip)
			}
			if baseTree, err = l.git.treeOf(ctx, e.Base); err != nil {
				return err
			}
		}
		tree, err := l.wikiApply(ctx, baseTree, e)
		if err != nil {
			return err
		}
		if tree == baseTree {
			return ErrWikiNoChange
		}
		var parents []string
		if !e.NewWiki {
			parents = []string{e.Base}
		}
		ours, err := l.wikiCommit(ctx, tree, parents, e, false)
		if err != nil {
			return err
		}
		for try := 1; ; try++ {
			commit, merged := ours, false
			if !e.NewWiki && tip != e.Base {
				theirsTree, err := l.git.treeOf(ctx, tip)
				if err != nil {
					return err
				}
				conflict, err := l.renameGuard(ctx, e, baseTree, theirsTree)
				if err != nil {
					return err
				}
				var mergedTree string
				if conflict == nil {
					if mergedTree, conflict, err = l.git.mergeTrees(ctx, baseTree, tree, theirsTree, ""); err != nil {
						return err
					}
				}
				if conflict != nil {
					return l.wikiConflict(ctx, e, tip, tree, conflict)
				}
				if mergedTree == theirsTree {
					// The tip already holds this edit.
					res.Commit = tip
					return nil
				}
				if commit, err = l.wikiCommit(ctx, mergedTree, []string{tip, ours}, e, true); err != nil {
					return err
				}
				merged = true
			}
			if !remote {
				if err := l.git.updateRef(ctx, ref, commit, tip); err != nil {
					return err
				}
				res.Commit, res.Merged, res.Created = commit, merged, e.NewWiki
				return nil
			}
			lease := tip
			out, err := l.git.pushCommit(ctx, ref, commit, &lease)
			if err != nil {
				return fmt.Errorf("ledger: wiki push: %w", err)
			}
			if out == pushOK {
				if err := l.git.updateRef(ctx, ref, commit, tip); err != nil {
					return err
				}
				if err := l.setKnown(ctx, ref, commit); err != nil {
					return err
				}
				res.Commit, res.Merged, res.Created = commit, merged, e.NewWiki
				return nil
			}
			// Lost a race: the remote moved. Fetch and merge again, never
			// force.
			if try >= wikiPushTries {
				return fmt.Errorf("ledger: wiki %s: the remote kept moving during %d save attempts; try again", e.Wiki, try)
			}
			if err := l.fetchWiki(ctx); err != nil {
				return err
			}
			if tip, err = l.wikiTip(ctx, e.Wiki); err != nil {
				return err
			}
			switch {
			case e.NewWiki && tip != "":
				return ErrWikiExists
			case !e.NewWiki && (tip == "" || !l.git.isAncestor(ctx, e.Base, tip)):
				return stale(tip)
			}
		}
	})
	l.mu.Lock()
	res.Warnings = append(append([]Warning(nil), l.wikiWarns...), pushWarns...)
	l.mu.Unlock()
	return res, err
}

// renameGuard refuses a rename or delete whose page the other side moved
// or removed: the tree merge alone would keep both names of a page both
// sides renamed, or resurrect under its new name a page the other side
// deleted. A page both sides deleted outright is not a conflict (the
// merge finds the edit already applied). nil when the edit is neither, or
// the tip still holds the page.
func (l *Ledger) renameGuard(ctx context.Context, e WikiEdit, baseTree, theirsTree string) (*treeConflict, error) {
	if e.Op != WikiOpRename && e.Op != WikiOpDelete {
		return nil, nil
	}
	file := pageFile(e.Path)
	if _, ok, err := l.git.entryAt(ctx, theirsTree, file); err != nil || ok {
		return nil, err
	}
	if e.Op == WikiOpRename {
		return &treeConflict{Path: file, Kind: WikiConflictRename}, nil
	}
	base, ok, err := l.git.entryAt(ctx, baseTree, file)
	if err != nil || !ok {
		return nil, err
	}
	// A rename is the page's content at a path the base did not hold: a
	// page the base already held with the same content is not where this
	// one went.
	renamed := false
	err = l.git.walkTree(ctx, theirsTree, "", func(path string, entry treeEntry) error {
		if renamed || entry.oid != base.oid || !strings.HasSuffix(path, pageExt) {
			return nil
		}
		_, inBase, err := l.git.entryAt(ctx, baseTree, path)
		renamed = err == nil && !inBase
		return err
	})
	if err != nil || !renamed {
		return nil, err
	}
	return &treeConflict{Path: file, Kind: WikiConflictRename}, nil
}

// wikiConflict builds the refusal of an edit against tip: the saver's text
// (the conflicting path as the edit's tree holds it, when the edit was
// applied) and the text at tip of the conflicting path.
func (l *Ledger) wikiConflict(ctx context.Context, e WikiEdit, tip, oursTree string, c *treeConflict) error {
	out := &WikiConflict{Wiki: e.Wiki, Path: strings.TrimSuffix(c.Path, pageExt), Kind: c.Kind, Base: e.Base, Theirs: tip}
	switch {
	case oursTree != "":
		if data, ok, err := l.git.readBlob(ctx, oursTree+":"+c.Path); err == nil && ok {
			out.OursText = string(data)
		}
	case e.Op == WikiOpWrite && strings.TrimSuffix(c.Path, pageExt) == e.Path:
		out.OursText = e.Content
	}
	if tip != "" {
		if data, ok, err := l.git.readBlob(ctx, tip+":"+c.Path); err == nil && ok {
			out.TheirsText = string(data)
		}
	}
	switch c.Kind {
	case WikiConflictStaleBase:
		switch {
		case tip == "":
			out.Reason = "the wiki no longer exists on the remote; recreate it with new_wiki, from a recovery point when one is listed"
		default:
			out.Reason = "base " + e.Base + " is not an ancestor of the current tip " + tip + "; read the page again and redo the edit on it"
		}
	case WikiConflictText:
		out.Reason = "another save changed overlapping lines; merge the two texts and save again with the current tip as base"
	case WikiConflictModifyDelete:
		out.Reason = "one side changed what the other deleted; decide and save again with the current tip as base"
	case WikiConflictAddAdd:
		out.Reason = "both sides added this path with different content; merge and save again with the current tip as base"
	case WikiConflictRename:
		out.Reason = "the page was renamed or removed on the other side; find it at the current tip and redo the edit there"
	default:
		out.Reason = "the edit and the current tip disagree on whether this path is a folder or a file"
	}
	return out
}
