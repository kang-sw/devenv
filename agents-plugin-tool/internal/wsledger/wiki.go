package wsledger

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Wiki branches. A wiki is a plain git branch of the store repository,
// refs/heads/wiki/<name>, whose tree holds pages at any depth - Markdown
// pages (<path>.md, addressed as <path>) and HTML pages (<path>.html,
// addressed as is) - and the images Markdown pages paste
// (_attachments/<sha256>.<ext>). It
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
	htmlExt       = ".html"
)

// Wiki page kinds: the page path's extension decides (WikiPageKind).
const (
	WikiKindMarkdown = "markdown"
	WikiKindHTML     = "html"
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
	ErrWikiBadPath       = errors.New("ledger: a page path is folder segments and a page name joined by /: a Markdown page is name or name.md (the same page), an HTML page is name.html; no folder ends in .md or .html, no name ends in .md or .html otherwise (a.html.md, a.HTML), and there is no ., .., leading _attachments, or any of the characters : \\ * ? \" < > |")
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

// ValidWikiPath reports whether s is a page path (CanonicalWikiPath).
func ValidWikiPath(s string) bool {
	_, ok := CanonicalWikiPath(s)
	return ok
}

// CanonicalWikiPath returns the canonical form of page path s and whether
// s is a page path at all: one or more non-empty segments joined by "/",
// none "." or "..", the first not the attachments folder, with no control
// characters and none of the characters a Windows checkout refuses. The
// last segment names the page and its extension the kind:
//
//   - name.html (exactly lowercase, non-empty name) is an HTML page, stored
//     at that path as is;
//   - name.md (exactly lowercase, non-empty name) is the Markdown page name,
//     canonicalized by dropping .md, so a.md and a address one file, a.md;
//   - any other name is a Markdown page, refused when it ends in .md or
//     .html in any case (a.html.md, a.md.md, a.HTML, a.MD).
//
// A folder segment ending in .md or .html in any case is refused, so a
// folder never shares a tree name with a page file. The exact-lowercase
// extensions keep a.HTML beside a.html from colliding on a
// case-insensitive checkout.
func CanonicalWikiPath(s string) (string, bool) {
	if s == "" || len(s) > wikiPathMax || strings.ContainsAny(s, ":\\*?\"<>|") {
		return "", false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return "", false
		}
	}
	segs := strings.Split(s, "/")
	if segs[0] == WikiAttachDir {
		return "", false
	}
	last := len(segs) - 1
	for i, seg := range segs {
		if seg == "" || seg == "." || seg == ".." || strings.TrimSpace(seg) != seg {
			return "", false
		}
		if i < last && pageLikeName(seg) {
			return "", false
		}
	}
	name := segs[last]
	// stemOK holds a page name's stem to the segment rules.
	stemOK := func(stem string) bool { return stem != "." && stem != ".." && strings.TrimSpace(stem) == stem }
	switch {
	case len(name) > len(htmlExt) && strings.HasSuffix(name, htmlExt):
		return s, stemOK(strings.TrimSuffix(name, htmlExt))
	case len(name) > len(pageExt) && strings.HasSuffix(name, pageExt):
		name = strings.TrimSuffix(name, pageExt)
		if !stemOK(name) {
			return "", false
		}
	}
	if pageLikeName(name) {
		return "", false
	}
	segs[last] = name
	return strings.Join(segs, "/"), true
}

// pageLikeName reports whether seg ends in a page file extension, in any
// case.
func pageLikeName(seg string) bool {
	low := strings.ToLower(seg)
	return strings.HasSuffix(low, pageExt) || strings.HasSuffix(low, htmlExt)
}

// WikiPageKind is the kind of canonical page path path: WikiKindHTML for
// a .html page, WikiKindMarkdown otherwise.
func WikiPageKind(path string) string {
	if strings.HasSuffix(path, htmlExt) {
		return WikiKindHTML
	}
	return WikiKindMarkdown
}

func wikiRef(name string) string              { return wikiPrefix + name }
func wikiRecoveryRef(name, oid string) string { return wikiRecoveryPrefix + name + "/" + oid }
func pageParts(path string) []string          { return strings.Split(pageFile(path), "/") }
func wikiImageParts(oid, ext string) []string { return []string{WikiAttachDir, oid + "." + ext} }
func wikiNameOf(ref string) (string, bool) {
	n, ok := strings.CutPrefix(ref, wikiPrefix)
	return n, ok && ValidWikiName(n)
}

// pageFile is the tree file of canonical page path path: an HTML page's
// own path, a Markdown page's path with .md. Every operation addresses a
// page through it, and maps a tree file back through pageOf.
func pageFile(path string) string {
	if WikiPageKind(path) == WikiKindHTML {
		return path
	}
	return path + pageExt
}

// pageOf is the page path tree file file holds, false when file is no
// page: an attachment, a file of another extension, or a blob no valid
// path maps to (x.html.md, a.md.md, anything under an a.html/ folder),
// which listing and search skip and no read or save can address.
func pageOf(file string) (string, bool) {
	if strings.HasPrefix(file, WikiAttachDir+"/") {
		return "", false
	}
	var p string
	switch {
	case strings.HasSuffix(file, htmlExt):
		p = file
	case strings.HasSuffix(file, pageExt):
		p = strings.TrimSuffix(file, pageExt)
	default:
		return "", false
	}
	c, ok := CanonicalWikiPath(p)
	if !ok || c != p || pageFile(c) != file {
		return "", false
	}
	return c, true
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
