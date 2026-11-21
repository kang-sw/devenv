// Wiki saves commit edits and report conflicts.
package wsledger

import (
	"context"
	"fmt"
)

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
	save := func() error {
		remote := l.hasRemote(ctx)
		if remote {
			if err := l.fetchWiki(ctx); err != nil {
				return err
			}
			// A wiki written before the store had a remote is published
			// first, so the save below pushes onto a ref the remote holds.
			// Another wiki the remote refuses is only reported; this one
			// refuses the save.
			failed, err := l.pushLocalWikis(ctx, held)
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
	}
	// One step, network included: a save's fetch, merge, and push are one
	// decision about one wiki, so the store lock stays held across them.
	err := l.withNet(ctx, func() error {
		return l.gated(func() error { return l.withLock(ctx, save) })
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
		if renamed || entry.oid != base.oid {
			return nil
		}
		if _, isPage := pageOf(path); !isPage {
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
	path := c.Path
	if p, isPage := pageOf(c.Path); isPage {
		path = p
	}
	out := &WikiConflict{Wiki: e.Wiki, Path: path, Kind: c.Kind, Base: e.Base, Theirs: tip}
	switch {
	case oursTree != "":
		if data, ok, err := l.git.readBlob(ctx, oursTree+":"+c.Path); err == nil && ok {
			out.OursText = string(data)
		}
	case e.Op == WikiOpWrite && c.Path == pageFile(e.Path):
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
