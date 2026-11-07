package wsledger

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// gitRepo drives the user's git over one bare repository through the wsgit
// runner. The runner has no stdin or environment hook and merges stderr into
// stdout, so content goes through files and settings through -c options,
// and every parser skips lines it does not recognize.
type gitRepo struct {
	runner wsgit.Runner
	dir    string // the bare repository
	tmp    string // scratch directory for files handed to git
}

func (g *gitRepo) run(ctx context.Context, args ...string) ([]byte, error) {
	return g.runner.RunGit(ctx, g.dir, args...)
}

// minGit is the oldest git with SSH commit signing.
var minGit = [2]int{2, 34}

var gitVersionRe = regexp.MustCompile(`git version (\d+)\.(\d+)`)

// checkTools fails with a message naming the tool when git is missing or
// older than 2.34, or ssh-keygen is missing.
func checkTools(ctx context.Context, runner wsgit.Runner, dir string, lookPath func(string) (string, error)) error {
	out, err := runner.RunGit(ctx, dir, "version")
	if err != nil {
		return fmt.Errorf("ledger needs git %d.%d or later: %w", minGit[0], minGit[1], err)
	}
	m := gitVersionRe.FindSubmatch(out)
	if m == nil {
		return fmt.Errorf("ledger needs git %d.%d or later: cannot read %q", minGit[0], minGit[1], strings.TrimSpace(string(out)))
	}
	major, _ := strconv.Atoi(string(m[1]))
	minor, _ := strconv.Atoi(string(m[2]))
	if major < minGit[0] || (major == minGit[0] && minor < minGit[1]) {
		return fmt.Errorf("ledger needs git %d.%d or later for SSH commit signing; found %d.%d", minGit[0], minGit[1], major, minor)
	}
	if _, err := lookPath("ssh-keygen"); err != nil {
		return fmt.Errorf("ledger needs ssh-keygen for SSH commit signing: %w", err)
	}
	return nil
}

func (g *gitRepo) initBare(ctx context.Context) error {
	if err := os.MkdirAll(g.dir, 0o755); err != nil {
		return err
	}
	_, err := g.run(ctx, "init", "--quiet", "--bare")
	return err
}

var oidRe = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// lastOID returns the last line of out that is an object id.
func lastOID(out []byte) (string, error) {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); oidRe.MatchString(l) {
			return l, nil
		}
	}
	return "", fmt.Errorf("git printed no object id: %q", strings.TrimSpace(string(out)))
}

// listRefs returns ref name -> object id for refs under prefix.
func (g *gitRepo) listRefs(ctx context.Context, prefix string) (map[string]string, error) {
	out, err := g.run(ctx, "for-each-ref", "--format=%(objectname) %(refname)", prefix)
	if err != nil {
		return nil, err
	}
	refs := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		oid, name, ok := strings.Cut(strings.TrimSpace(line), " ")
		if ok && oidRe.MatchString(oid) && strings.HasPrefix(name, prefix) {
			refs[name] = oid
		}
	}
	return refs, nil
}

func (g *gitRepo) tempFile(data []byte) (string, error) {
	if err := os.MkdirAll(g.tmp, 0o700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(g.tmp, "obj-*")
	if err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// hashObject writes an object of type typ from data.
func (g *gitRepo) hashObject(ctx context.Context, typ string, data []byte) (string, error) {
	path, err := g.tempFile(data)
	if err != nil {
		return "", err
	}
	defer os.Remove(path)
	out, err := g.run(ctx, "hash-object", "-w", "-t", typ, "--", path)
	if err != nil {
		return "", err
	}
	return lastOID(out)
}

// readBlob returns the blob at rev ("<commit>:<path>"); ok is false when it
// does not exist.
func (g *gitRepo) readBlob(ctx context.Context, rev string) (data []byte, ok bool, err error) {
	if _, err := g.run(ctx, "cat-file", "-e", rev); err != nil {
		return nil, false, nil
	}
	out, err := g.run(ctx, "cat-file", "blob", rev)
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}

type treeEntry struct {
	mode string // "100644" or "40000"
	name string
	oid  string
}

func (e treeEntry) isTree() bool { return e.mode == "40000" || e.mode == "040000" }

// lsTree lists one tree level.
func (g *gitRepo) lsTree(ctx context.Context, tree string) ([]treeEntry, error) {
	out, err := g.run(ctx, "ls-tree", "-z", "--full-tree", tree)
	if err != nil {
		return nil, err
	}
	var entries []treeEntry
	for _, rec := range strings.Split(string(out), "\x00") {
		meta, name, ok := strings.Cut(rec, "\t")
		if !ok {
			continue
		}
		f := strings.Fields(meta)
		if len(f) != 3 || !oidRe.MatchString(f[2]) {
			continue
		}
		entries = append(entries, treeEntry{mode: strings.TrimPrefix(f[0], "0"), name: name, oid: f[2]})
	}
	return entries, nil
}

// writeTree writes a tree object in git's binary format.
func (g *gitRepo) writeTree(ctx context.Context, entries []treeEntry) (string, error) {
	sorted := append([]treeEntry(nil), entries...)
	key := func(e treeEntry) string {
		if e.isTree() {
			return e.name + "/"
		}
		return e.name
	}
	sort.Slice(sorted, func(i, j int) bool { return key(sorted[i]) < key(sorted[j]) })
	var buf bytes.Buffer
	for _, e := range sorted {
		raw, err := hex.DecodeString(e.oid)
		if err != nil {
			return "", err
		}
		mode := e.mode
		if e.isTree() {
			mode = "40000"
		}
		buf.WriteString(mode + " " + e.name + "\x00")
		buf.Write(raw)
	}
	return g.hashObject(ctx, "tree", buf.Bytes())
}

// putPath returns a tree equal to base (or empty when base is "") with the
// blob placed at parts, creating intermediate trees.
func (g *gitRepo) putPath(ctx context.Context, base string, parts []string, blob string) (string, error) {
	var entries []treeEntry
	if base != "" {
		var err error
		if entries, err = g.lsTree(ctx, base); err != nil {
			return "", err
		}
	}
	name := parts[0]
	idx := -1
	for i, e := range entries {
		if e.name == name {
			idx = i
		}
	}
	var entry treeEntry
	if len(parts) == 1 {
		entry = treeEntry{mode: "100644", name: name, oid: blob}
	} else {
		sub := ""
		if idx >= 0 && entries[idx].isTree() {
			sub = entries[idx].oid
		}
		oid, err := g.putPath(ctx, sub, parts[1:], blob)
		if err != nil {
			return "", err
		}
		entry = treeEntry{mode: "40000", name: name, oid: oid}
	}
	if idx >= 0 {
		entries[idx] = entry
	} else {
		entries = append(entries, entry)
	}
	return g.writeTree(ctx, entries)
}

// treeOf returns a commit's tree id.
func (g *gitRepo) treeOf(ctx context.Context, commit string) (string, error) {
	out, err := g.run(ctx, "rev-parse", "--verify", "--quiet", commit+"^{tree}")
	if err != nil {
		return "", err
	}
	return lastOID(out)
}

// commitTree writes a commit signed with the SSH private key at keyPath.
func (g *gitRepo) commitTree(ctx context.Context, tree, parent string, msg []byte, author, keyPath string) (string, error) {
	path, err := g.tempFile(msg)
	if err != nil {
		return "", err
	}
	defer os.Remove(path)
	args := []string{
		"-c", "user.name=" + author,
		"-c", "user.email=" + author + "@ws-ledger.invalid",
		"-c", "gpg.format=ssh",
		"-c", "user.signingkey=" + keyPath,
		"commit-tree", "-S", "-F", path,
	}
	if parent != "" {
		args = append(args, "-p", parent)
	}
	args = append(args, tree)
	out, err := g.run(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("signed commit (needs ssh-keygen and a readable key at %s): %w", keyPath, err)
	}
	return lastOID(out)
}

// updateRef moves ref from old to new; old "" requires that ref not exist.
func (g *gitRepo) updateRef(ctx context.Context, ref, newOID, old string) error {
	_, err := g.run(ctx, "update-ref", "-m", "ledger", ref, newOID, old)
	return err
}

func (g *gitRepo) deleteRef(ctx context.Context, ref, old string) error {
	_, err := g.run(ctx, "update-ref", "-d", ref, old)
	return err
}

// isAncestor reports whether a is an ancestor of (or equal to) b.
func (g *gitRepo) isAncestor(ctx context.Context, a, b string) bool {
	_, err := g.run(ctx, "merge-base", "--is-ancestor", a, b)
	return err == nil
}

func nonce() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// rawCommit is one commit as log -p shows it. The patch fields describe the
// single pure append a ledger commit must be; PatchBad says why not.
type rawCommit struct {
	OID      string
	Parents  []string
	Message  []byte
	Path     string
	NewFile  bool
	OldStart int // line count of the file before the append
	Added    []string
	PatchBad string
}

// logCommits lists the first-parent commits of tip not reachable from
// exclude (all when exclude is ""), oldest first; max limits to the newest
// max commits when positive.
func (g *gitRepo) logCommits(ctx context.Context, tip, exclude string, max int) ([]rawCommit, error) {
	n := nonce()
	args := []string{
		"-c", "log.showRoot=true",
		"-c", "i18n.logOutputEncoding=UTF-8",
		"-c", "core.quotePath=true",
		"-c", "diff.noprefix=false",
		"-c", "diff.mnemonicPrefix=false",
		"log", "--first-parent", "--no-show-signature", "--no-decorate",
		"--format=%x00" + n + " %H %P%n%B%x00" + n + "B",
		"-p", "--unified=0", "--no-ext-diff", "--no-textconv", "--no-color",
		"--no-renames", "--no-relative", "--diff-algorithm=myers",
		"--src-prefix=a/", "--dst-prefix=b/",
	}
	if max > 0 {
		args = append(args, "--max-count="+strconv.Itoa(max))
	}
	args = append(args, tip)
	if exclude != "" {
		args = append(args, "^"+exclude)
	}
	args = append(args, "--")
	out, err := g.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	var commits []rawCommit
	for _, rec := range strings.Split(string(out), "\x00"+n+" ")[1:] {
		head, rest, ok := strings.Cut(rec, "\n")
		if !ok {
			continue
		}
		fields := strings.Fields(head)
		if len(fields) == 0 || !oidRe.MatchString(fields[0]) {
			continue
		}
		msg, patch, ok := strings.Cut(rest, "\x00"+n+"B")
		if !ok {
			continue
		}
		c := rawCommit{OID: fields[0], Parents: fields[1:], Message: []byte(msg)}
		parsePatch(&c, patch)
		commits = append(commits, c)
	}
	// log lists newest first; reverse after any --max-count cut.
	for i, j := 0, len(commits)-1; i < j; i, j = i+1, j-1 {
		commits[i], commits[j] = commits[j], commits[i]
	}
	return commits, nil
}

var hunkRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// parsePatch accepts exactly one file diff holding one hunk that appends
// whole lines and removes none.
func parsePatch(c *rawCommit, patch string) {
	lines := strings.Split(strings.TrimLeft(patch, "\n"), "\n")
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	bad := func(reason string) { c.PatchBad = reason; c.Added = nil }
	if len(lines) == 0 {
		bad("commit changes no event file")
		return
	}
	if !strings.HasPrefix(lines[0], "diff --git ") {
		bad("unexpected patch header")
		return
	}
	i := 1
	inHunk := false
	newCount := 0
	for ; i < len(lines); i++ {
		l := lines[i]
		if inHunk {
			if !strings.HasPrefix(l, "+") {
				bad("commit does more than append whole lines to one event file")
				return
			}
			c.Added = append(c.Added, l[1:])
			continue
		}
		switch {
		case strings.HasPrefix(l, "diff --git "):
			bad("commit touches more than one file")
			return
		case strings.HasPrefix(l, "new file mode "):
			if strings.TrimPrefix(l, "new file mode ") != "100644" {
				bad("event file has a non-regular mode")
				return
			}
			c.NewFile = true
		case strings.HasPrefix(l, "index "):
			if f := strings.Fields(l); len(f) == 3 && f[2] != "100644" {
				bad("event file has a non-regular mode")
				return
			}
		case strings.HasPrefix(l, "--- "):
			if (l == "--- /dev/null") != c.NewFile {
				bad("unexpected patch header")
				return
			}
		case strings.HasPrefix(l, "+++ "):
			p, ok := strings.CutPrefix(l, "+++ b/")
			if !ok {
				bad("unexpected patch header")
				return
			}
			c.Path = p
		case strings.HasPrefix(l, "@@ "):
			m := hunkRe.FindStringSubmatch(l)
			if m == nil || c.Path == "" {
				bad("unexpected hunk header")
				return
			}
			oldCount, newC := 1, 1
			if m[2] != "" {
				oldCount, _ = strconv.Atoi(m[2])
			}
			if m[4] != "" {
				newC, _ = strconv.Atoi(m[4])
			}
			oldStart, _ := strconv.Atoi(m[1])
			newStart, _ := strconv.Atoi(m[3])
			if oldCount != 0 || newStart != oldStart+1 || newC < 1 {
				bad("commit does more than append whole lines to one event file")
				return
			}
			c.OldStart, newCount, inHunk = oldStart, newC, true
		default:
			bad("unexpected patch line")
			return
		}
	}
	if !inHunk {
		bad("commit appends no event line")
		return
	}
	if len(c.Added) != newCount {
		bad("commit does more than append whole lines to one event file")
	}
}

type sigInfo struct {
	state sigState
	fp    string
}

// verifySigs checks commit signatures through the user's git with the given
// allowed-signers content. Verdicts use only the cryptographic result and
// the signer fingerprint git reports ("U", a good signature from a key the
// file does not list, still names its key), so the result is a pure
// function of the commit.
func (g *gitRepo) verifySigs(ctx context.Context, oids []string, allowed []byte) (map[string]sigInfo, error) {
	res := map[string]sigInfo{}
	if len(oids) == 0 {
		return res, nil
	}
	path, err := g.tempFile(allowed)
	if err != nil {
		return nil, err
	}
	defer os.Remove(path)
	const batch = 256
	for start := 0; start < len(oids); start += batch {
		end := min(start+batch, len(oids))
		args := []string{
			"-c", "gpg.ssh.allowedSignersFile=" + path,
			"-c", "gpg.minTrustLevel=undefined",
			"log", "--no-walk=unsorted", "--no-show-signature", "--no-decorate",
			"--format=%H %G? %GF",
		}
		args = append(args, oids[start:end]...)
		args = append(args, "--")
		out, err := g.run(ctx, args...)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(string(out), "\n") {
			f := strings.Fields(line)
			if len(f) < 2 || !oidRe.MatchString(f[0]) || len(f[1]) != 1 {
				continue
			}
			info := sigInfo{state: sigBad}
			switch f[1] {
			case "N":
				info.state = sigUnsigned
			case "G", "U":
				if len(f) == 3 && strings.HasPrefix(f[2], "SHA256:") {
					info = sigInfo{state: sigGood, fp: f[2]}
				}
			}
			res[f[0]] = info
		}
	}
	for _, oid := range oids {
		if _, ok := res[oid]; !ok {
			return nil, errors.New("signature check printed no result for " + oid)
		}
	}
	return res, nil
}
