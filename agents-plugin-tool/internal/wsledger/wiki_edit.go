// Wiki edits validate changes and merge wiki trees.
package wsledger

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

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
	// Paths are canonical from here on: the tree file, the commit subject,
	// and a conflict's path all derive from them.
	var ok bool
	if e.Path, ok = CanonicalWikiPath(e.Path); !ok {
		return ErrWikiBadPath
	}
	if e.Op == WikiOpRename {
		if e.To, ok = CanonicalWikiPath(e.To); !ok {
			return fmt.Errorf("to: %w", ErrWikiBadPath)
		}
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
	if WikiPageKind(e.Path) == WikiKindHTML && len(e.Images) > 0 {
		return fmt.Errorf("%w: an HTML page cannot show images; write it without them", ErrWikiBadEdit)
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
	// A folder an older daemon wrote under a name that is now a page file
	// (x.html/) is not that page: a write or restore would replace the whole
	// folder with the page's blob, and a delete would remove it.
	switch e.Op {
	case WikiOpWrite, WikiOpDelete, WikiOpRestore:
		if cur, ok, err := g.entryAt(ctx, tree, pageFile(e.Path)); err != nil {
			return "", err
		} else if ok && cur.isTree() {
			if e.Op == WikiOpDelete {
				return "", ErrWikiPageNotFound
			}
			return "", fmt.Errorf("%w: %s is a folder in the wiki, not a page", ErrWikiBadEdit, pageFile(e.Path))
		}
	}
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
