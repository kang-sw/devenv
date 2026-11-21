// Wiki reads list and search pages in wiki branches.
package wsledger

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

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
// Markdown and HTML pages sorted together by page path, and returns the
// commit read.
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
			if p, ok := pageOf(path); ok && e.mode == "100644" {
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
		var ok bool
		if path, ok = CanonicalWikiPath(path); !ok {
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
// query (fixed string, case-insensitive) and returns the tip searched. An
// HTML page is searched as its raw source, tags included.
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
			"--fixed-strings", "-e", query, at, "--", "*"+pageExt, "*"+htmlExt, ":(exclude)"+WikiAttachDir+"/*")
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
			p, isPage := pageOf(file)
			if err != nil || !isPage {
				continue
			}
			matches = append(matches, WikiMatch{Path: p, Line: n, Text: text})
		}
		return nil
	})
	return matches, at, err
}
