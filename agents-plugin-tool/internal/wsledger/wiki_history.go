// Wiki history, recovery, and image reads inspect wiki commits.
package wsledger

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
)

// WikiHistory lists the commits of wiki name that touched page path (every
// commit when path is ""), newest first, at most limit when positive,
// each with its signature verdict against the store's member keys.
func (l *Ledger) WikiHistory(ctx context.Context, name, path string, limit int) ([]WikiCommit, error) {
	var commits []WikiCommit
	err := l.withLock(ctx, func() error {
		if path != "" {
			var ok bool
			if path, ok = CanonicalWikiPath(path); !ok {
				return ErrWikiBadPath
			}
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
		var ok bool
		if path, ok = CanonicalWikiPath(path); !ok {
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
