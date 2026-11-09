package wsdashboard

import (
	"bytes"
	"container/list"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// The commit window's diff side reads one file's diff at a time:
//
//	GET /s/<reply-id>/git/commit/<oid>/diff?path=<p>
//	GET /s/<reply-id>/git/worktree/diff?side=staged|unstaged|untracked&path=<p>
//
// The first diffs a commit against its first parent (a root commit against
// the empty tree), path one of the commit's changed files; the second diffs
// the index against HEAD (staged), the working tree against the index
// (unstaged), or shows an untracked file whole, path one of the files the
// git/status read lists in that group. Neither fetches or takes git's
// optional locks. Both answer GitFileDiff JSON:
//
//	{"path", "binary", "notice", "truncated",
//	 "hunks": [{"header", "old", "new",
//	            "lines": [{"k", "t": [{"s", "c", "x"}]}]}]}
//
// A line's kind k is " ", "+", "-", or "\" (git's no-newline note); its
// pieces t are its text in order, each with a highlight class c (one of
// hlClass's codes, empty for plain text) and x set on the words an
// intra-line mark covers. The page numbers lines from each hunk's old and
// new start. The daemon highlights a file's whole old and new contents with
// chroma and maps the highlighted lines onto the hunk lines, so a comment
// or string that opens above a hunk still colours the lines inside it.

const (
	// gitDiffMax cuts one file's diff; a cut diff says so (truncated).
	gitDiffMax = 1 << 20
	// hlMaxBytes is the largest file content the daemon highlights; a
	// larger side is shown plain.
	hlMaxBytes = 256 << 10
	// hlCacheEntries bounds the highlight cache (blob id and path to
	// highlighted lines).
	hlCacheEntries = 32
)

// GitFileDiff is one file's diff for the commit window.
type GitFileDiff struct {
	Path   string `json:"path"`
	Binary bool   `json:"binary,omitempty"`
	// Notice names why the file has no text diff: "dir" (an untracked
	// directory), "gitlink" (a submodule entry), "unmerged", "special"
	// (neither a regular file nor a symlink), or "outside" (an untracked
	// file outside the session root, which is not read).
	Notice    string    `json:"notice,omitempty"`
	Truncated bool      `json:"truncated,omitempty"`
	Hunks     []GitHunk `json:"hunks"`
}

// GitHunk is one hunk: git's header line and the old and new line numbers
// its first line starts at.
type GitHunk struct {
	Header string        `json:"header"`
	Old    int           `json:"old"`
	New    int           `json:"new"`
	Lines  []GitDiffLine `json:"lines"`
}

// GitDiffLine is one hunk line.
type GitDiffLine struct {
	Kind   string     `json:"k"`
	Pieces []GitPiece `json:"t"`
}

// GitPiece is a run of one line's text with one highlight class.
type GitPiece struct {
	Text  string `json:"s"`
	Class string `json:"c,omitempty"`
	Mark  bool   `json:"x,omitempty"`
}

var errNoPath = errors.New("path not in this diff")

// runGitCapped runs git as gitBadgeRunner does, keeping at most max bytes
// of its output; cut reports more, and git is stopped there.
func runGitCapped(ctx context.Context, root string, max int, args ...string) (out []byte, cut bool, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, false, err
	}
	if err := cmd.Start(); err != nil {
		return nil, false, err
	}
	out, rerr := io.ReadAll(io.LimitReader(pipe, int64(max)+1))
	if len(out) > max {
		cancel()
		_ = cmd.Wait()
		return out[:max], true, nil
	}
	werr := cmd.Wait()
	if rerr != nil {
		return nil, false, rerr
	}
	if werr != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, false, fmt.Errorf("timed out after %s", gitReadTimeout)
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, false, fmt.Errorf("%s", gitErrText(errors.New(msg)))
		}
		return nil, false, werr
	}
	return out, false, nil
}

// diffArgs are the options every per-file patch read shares: full blob ids
// on the index line (the highlighter reads the blobs), no colour, external
// diff, or textconv.
var diffArgs = []string{"-p", "--full-index", "--no-color", "--no-ext-diff", "--no-textconv"}

// topPathspec names one listed file to git exactly. The lists (diff-tree,
// status) give paths from the repository top, while git reads a pathspec
// from -C root, a subdirectory when the session runs in one; and a plain
// pathspec globs, so a file named *.txt would match its siblings too.
func topPathspec(path string) string { return ":(top,literal)" + path }

// gitPrefix is root's path below the repository top, slash-terminated, or
// "" at the top (git rev-parse --show-prefix).
func gitPrefix(ctx context.Context, root string) (string, error) {
	out, _, err := runGitCapped(ctx, root, 1<<12, "rev-parse", "--show-prefix")
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

// underPrefix turns a path from the repository top into one from the
// session root, false when the file lies outside the root: the working
// tree is read under the file view's rule, inside the session root only.
func underPrefix(prefix, path string) (string, bool) {
	if !strings.HasPrefix(path, prefix) || len(path) == len(prefix) {
		return "", false
	}
	return path[len(prefix):], true
}

// readCommitFileDiff reads path's diff in commit oid.
func readCommitFileDiff(root, oid, path string) (GitFileDiff, error) {
	c, err := readGitCommit(root, oid)
	if err != nil {
		return GitFileDiff{}, err
	}
	var file *GitCommitFile
	for i := range c.Files {
		if c.Files[i].Path == path {
			file = &c.Files[i]
			break
		}
	}
	if file == nil {
		return GitFileDiff{}, errNoPath
	}
	d := GitFileDiff{Path: path, Hunks: []GitHunk{}}
	if file.Add < 0 {
		d.Binary = true
		return d, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitReadTimeout)
	defer cancel()
	args := append([]string{"--no-optional-locks", "diff-tree", "-r", "-M", "--no-commit-id"}, diffArgs...)
	if c.Base != "" {
		args = append(args, c.Base, c.OID)
	} else {
		args = append(args, "--root", c.OID)
	}
	args = append(args, "--")
	if file.From != "" {
		args = append(args, topPathspec(file.From))
	}
	args = append(args, topPathspec(path))
	out, cut, err := runGitCapped(ctx, root, gitDiffMax, args...)
	if err != nil {
		return GitFileDiff{}, err
	}
	blob := func(oid string) hlLines { return blobHighlight(ctx, root, oid, path) }
	return buildFileDiff(path, out, cut, blob, blob), nil
}

// readWorktreeFileDiff reads path's diff on one side of the working tree.
func readWorktreeFileDiff(root, side, path string) (GitFileDiff, error) {
	changes, err := readGitChanges(root)
	if err != nil {
		return GitFileDiff{}, err
	}
	var group []GitFile
	switch side {
	case "staged":
		group = changes.Staged
	case "unstaged":
		group = changes.Unstaged
	case "untracked":
		group = changes.Untracked
	}
	var file *GitFile
	for i := range group {
		if group[i].Path == path {
			file = &group[i]
			break
		}
	}
	if file == nil {
		return GitFileDiff{}, errNoPath
	}
	d := GitFileDiff{Path: path, Hunks: []GitHunk{}}
	switch {
	case file.Status == "U":
		d.Notice = "unmerged"
		return d, nil
	case side != "untracked" && file.Add < 0:
		d.Binary = true
		return d, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitReadTimeout)
	defer cancel()
	// Working-tree reads take the path from the session root.
	var local string
	localOK := false
	if side != "staged" {
		prefix, err := gitPrefix(ctx, root)
		if err != nil {
			return GitFileDiff{}, err
		}
		local, localOK = underPrefix(prefix, path)
	}
	if side == "untracked" {
		if !localOK {
			d.Notice = "outside"
			return d, nil
		}
		d = untrackedFileDiff(root, local)
		d.Path = path
		return d, nil
	}
	args := []string{"--no-optional-locks", "diff", "--no-renames"}
	if side == "staged" {
		args = append(args, "--cached")
	}
	args = append(append(args, diffArgs...), "--", topPathspec(path))
	out, cut, err := runGitCapped(ctx, root, gitDiffMax, args...)
	if err != nil {
		return GitFileDiff{}, err
	}
	blob := func(oid string) hlLines { return blobHighlight(ctx, root, oid, path) }
	if side == "staged" {
		return buildFileDiff(path, out, cut, blob, blob), nil
	}
	// The new side is the working tree, which has no blob to read or cache;
	// outside the session root it goes uncoloured.
	work := func(string) hlLines {
		if !localOK {
			return nil
		}
		return fileHighlight(root, local)
	}
	return buildFileDiff(path, out, cut, blob, work), nil
}

// untrackedFileDiff shows an untracked entry whole, as added lines: a
// regular file's text, a symlink's target (as git diffs a symlink), or a
// notice for a directory or any other kind. No symlink is followed.
func untrackedFileDiff(root, path string) GitFileDiff {
	d := GitFileDiff{Path: path, Hunks: []GitHunk{}}
	if strings.HasSuffix(path, "/") {
		d.Notice = "dir"
		return d
	}
	rel, ok := relativeInside(root, path)
	if !ok {
		d.Notice = "special"
		return d
	}
	info, err := os.Lstat(filepath.Join(root, rel))
	if err != nil {
		d.Notice = "special"
		return d
	}
	var content []byte
	highlight := true
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		if !parentInside(root, rel) {
			d.Notice = "special"
			return d
		}
		target, err := os.Readlink(filepath.Join(root, rel))
		if err != nil {
			d.Notice = "special"
			return d
		}
		content, highlight = []byte(target), false
	case info.Mode().IsRegular():
		f, _, _, ok := openContained(root, path)
		if !ok {
			d.Notice = "special"
			return d
		}
		content, _ = io.ReadAll(io.LimitReader(f, gitDiffMax+1))
		_ = f.Close()
		if len(content) > gitDiffMax {
			content, d.Truncated = content[:gitDiffMax], true
		}
	case info.IsDir():
		d.Notice = "dir"
		return d
	default:
		d.Notice = "special"
		return d
	}
	if bytes.IndexByte(content[:min(len(content), 8000)], 0) >= 0 {
		d.Binary = true
		return d
	}
	text := string(content)
	if d.Truncated {
		if i := strings.LastIndexByte(text, '\n'); i >= 0 {
			text = text[:i+1]
		}
	}
	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return d
	}
	h := diffHunk{header: fmt.Sprintf("@@ -0,0 +1,%d @@", len(lines)), newStart: 1}
	for _, l := range lines {
		h.lines = append(h.lines, diffLine{kind: '+', text: strings.TrimSuffix(l, "\n")})
	}
	var newHL hlLines
	if highlight && len(content) <= hlMaxBytes && !d.Truncated {
		newHL = highlightLines(path, content)
	}
	d.Hunks = renderHunks([]diffHunk{h}, nil, newHL)
	return d
}

// parentInside reports whether rel's directory resolves inside root, so a
// symlinked directory on the way never leads a read outside it.
func parentInside(root, rel string) bool {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	dir, err := filepath.EvalSymlinks(filepath.Dir(filepath.Join(root, rel)))
	if err != nil {
		return false
	}
	r, err := filepath.Rel(realRoot, dir)
	return err == nil && (r == "." || insideRel(r))
}

// diffHunk and diffLine are a parsed patch before highlighting.
type diffHunk struct {
	header             string
	oldStart, newStart int
	lines              []diffLine
}

type diffLine struct {
	kind byte
	text string
}

// patchInfo is what a patch's file header says.
type patchInfo struct {
	oldBlob, newBlob string
	binary, gitlink  bool
}

// parsePatch reads a one-file patch (`diff -p --full-index`). A cut patch
// loses its last, possibly partial, line.
func parsePatch(out []byte, cut bool) ([]diffHunk, patchInfo) {
	text := string(out)
	if cut {
		if i := strings.LastIndexByte(text, '\n'); i >= 0 {
			text = text[:i+1]
		} else {
			text = ""
		}
	}
	var hunks []diffHunk
	var info patchInfo
	inHunk := false
	for _, l := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		if strings.HasPrefix(l, "@@ ") {
			h := diffHunk{header: l}
			h.oldStart, h.newStart = parseHunkHeader(l)
			hunks = append(hunks, h)
			inHunk = true
			continue
		}
		if inHunk && l != "" && strings.IndexByte(" +-\\", l[0]) >= 0 {
			h := &hunks[len(hunks)-1]
			h.lines = append(h.lines, diffLine{kind: l[0], text: l[1:]})
			continue
		}
		if inHunk && l == "" {
			// A context line of an empty line some tools strip to nothing.
			h := &hunks[len(hunks)-1]
			h.lines = append(h.lines, diffLine{kind: ' '})
			continue
		}
		inHunk = false
		switch {
		case strings.HasPrefix(l, "index "):
			f := strings.Fields(l)
			if len(f) >= 2 {
				if a, b, ok := strings.Cut(f[1], ".."); ok {
					info.oldBlob, info.newBlob = a, b
				}
			}
			if len(f) >= 3 && f[2] == "160000" {
				info.gitlink = true
			}
		case strings.HasSuffix(l, " 160000") && (strings.HasPrefix(l, "new file mode") || strings.HasPrefix(l, "deleted file mode") || strings.HasPrefix(l, "old mode") || strings.HasPrefix(l, "new mode")):
			info.gitlink = true
		case strings.HasPrefix(l, "Binary files ") || l == "GIT binary patch":
			info.binary = true
		}
	}
	return hunks, info
}

// parseHunkHeader reads "@@ -a[,b] +c[,d] @@" into a and c.
func parseHunkHeader(h string) (old, new int) {
	f := strings.Fields(h)
	if len(f) < 3 {
		return 0, 0
	}
	num := func(s string) int {
		s, _, _ = strings.Cut(s[1:], ",")
		n, _ := strconv.Atoi(s)
		return n
	}
	return num(f[1]), num(f[2])
}

// buildFileDiff turns a patch into the answer, highlighting the old side
// from oldHL(old blob id) and the new side from newHL(new blob id); each is
// asked only when the patch has lines on its side.
func buildFileDiff(path string, out []byte, cut bool, oldHL, newHL func(oid string) hlLines) GitFileDiff {
	d := GitFileDiff{Path: path, Truncated: cut, Hunks: []GitHunk{}}
	hunks, info := parsePatch(out, cut)
	switch {
	case info.gitlink:
		d.Notice = "gitlink"
		return d
	case info.binary:
		d.Binary = true
		return d
	}
	var hasOld, hasNew bool
	for _, h := range hunks {
		for _, l := range h.lines {
			hasOld = hasOld || l.kind == '-'
			hasNew = hasNew || l.kind == '+' || l.kind == ' '
		}
	}
	var o, n hlLines
	if hasOld {
		o = oldHL(info.oldBlob)
	}
	if hasNew {
		n = newHL(info.newBlob)
	}
	d.Hunks = renderHunks(hunks, o, n)
	return d
}

// renderHunks maps highlighted lines onto the hunk lines (a removed line
// from the old side, a context or added line from the new side) and marks
// changed words. A line whose highlighted text differs from the patch's
// shows plain.
func renderHunks(hunks []diffHunk, oldHL, newHL hlLines) []GitHunk {
	out := make([]GitHunk, 0, len(hunks))
	for _, h := range hunks {
		g := GitHunk{Header: h.header, Old: h.oldStart, New: h.newStart, Lines: make([]GitDiffLine, 0, len(h.lines))}
		marks := pairMarks(h.lines)
		o, n := h.oldStart, h.newStart
		for i, l := range h.lines {
			var toks []hlTok
			switch l.kind {
			case '-':
				toks = oldHL.line(o, l.text)
				o++
			case '+':
				toks = newHL.line(n, l.text)
				n++
			case ' ':
				toks = newHL.line(n, l.text)
				o++
				n++
			}
			if toks == nil {
				toks = []hlTok{{Text: l.text}}
			}
			m := marks[i]
			g.Lines = append(g.Lines, GitDiffLine{Kind: string(l.kind), Pieces: splitPieces(toks, m[0], m[1])})
		}
		out = append(out, g)
	}
	return out
}

// splitPieces turns a line's tokens into pieces, the bytes [from, to)
// marked; from == to marks nothing.
func splitPieces(toks []hlTok, from, to int) []GitPiece {
	pieces := make([]GitPiece, 0, len(toks))
	pos := 0
	for _, t := range toks {
		s := t.Text
		for s != "" {
			end := len(s)
			mark := from < to && pos >= from && pos < to
			switch {
			case mark && pos+end > to:
				end = to - pos
			case !mark && from < to && pos < from && pos+end > from:
				end = from - pos
			}
			if n := len(pieces); n > 0 && pieces[n-1].Class == t.Class && pieces[n-1].Mark == mark {
				pieces[n-1].Text += s[:end]
			} else {
				pieces = append(pieces, GitPiece{Text: s[:end], Class: t.Class, Mark: mark})
			}
			pos += end
			s = s[end:]
		}
	}
	if len(pieces) == 0 {
		pieces = append(pieces, GitPiece{})
	}
	return pieces
}

// pairMarks finds each removed run directly followed by an added run of the
// same length and pairs their lines in order; a pair's changed words are
// marked, as byte ranges per line. Every other line has none.
func pairMarks(lines []diffLine) [][2]int {
	marks := make([][2]int, len(lines))
	for i := 0; i < len(lines); {
		if lines[i].kind != '-' {
			i++
			continue
		}
		del := i
		for i < len(lines) && lines[i].kind == '-' {
			i++
		}
		add := i
		for i < len(lines) && lines[i].kind == '+' {
			i++
		}
		if add-del != i-add {
			continue
		}
		for k := 0; k < add-del; k++ {
			a, b, ok := wordChange(lines[del+k].text, lines[add+k].text)
			if ok {
				marks[del+k], marks[add+k] = a, b
			}
		}
	}
	return marks
}

// wordChange compares two lines word by word (a run of letters, digits, or
// '_'; a run of spaces; any other single character) and returns the byte
// range of each that lies between their common leading and trailing words.
// It reports false for equal lines and for lines with nothing in common.
func wordChange(a, b string) ([2]int, [2]int, bool) {
	wa, wb := splitWords(a), splitWords(b)
	p := 0
	for p < len(wa) && p < len(wb) && wa[p].s == wb[p].s {
		p++
	}
	q := 0
	for q < len(wa)-p && q < len(wb)-p && wa[len(wa)-1-q].s == wb[len(wb)-1-q].s {
		q++
	}
	if p == 0 && q == 0 {
		return [2]int{}, [2]int{}, false
	}
	if p == len(wa) && p == len(wb) {
		return [2]int{}, [2]int{}, false
	}
	span := func(w []word, line string) [2]int {
		from, to := len(line), len(line)
		if p < len(w) {
			from = w[p].at
		}
		if len(w)-q < len(w) {
			to = w[len(w)-q].at
		}
		if to < from {
			to = from
		}
		return [2]int{from, to}
	}
	return span(wa, a), span(wb, b), true
}

type word struct {
	s  string
	at int
}

func splitWords(s string) []word {
	var out []word
	class := func(r rune) int {
		switch {
		case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
			return 1
		case r == ' ' || r == '\t':
			return 2
		}
		return 0
	}
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		c, j := class(r), i+size
		if c != 0 {
			for j < len(s) {
				r2, s2 := utf8.DecodeRuneInString(s[j:])
				if class(r2) != c {
					break
				}
				j += s2
			}
		}
		out = append(out, word{s: s[i:j], at: i})
		i = j
	}
	return out
}

// hlTok is one highlighted run; hlLines is a file's highlighted lines.
type hlTok struct {
	Text  string
	Class string
}

type hlLines [][]hlTok

// line is the highlighted line number n (1-based) when its text is text;
// nil otherwise (no highlight, a short file, or a mismatch, such as a line
// git shows with a carriage return the lexer dropped).
func (h hlLines) line(n int, text string) []hlTok {
	if n < 1 || n > len(h) {
		return nil
	}
	toks := h[n-1]
	var sb strings.Builder
	for _, t := range toks {
		sb.WriteString(t.Text)
	}
	if sb.String() != text {
		return nil
	}
	return toks
}

// highlightLines highlights a file's content by its name; nil when no
// lexer knows the name, the content is binary, or it is over hlMaxBytes.
func highlightLines(path string, content []byte) hlLines {
	if len(content) > hlMaxBytes || bytes.IndexByte(content, 0) >= 0 || !utf8.Valid(content) {
		return nil
	}
	lexer := lexers.Match(filepath.Base(path))
	if lexer == nil || lexer.Config().Name == "plaintext" {
		return nil
	}
	it, err := chroma.Coalesce(lexer).Tokenise(&chroma.TokeniseOptions{State: "root"}, string(content))
	if err != nil {
		return nil
	}
	lines := hlLines{nil}
	for t := it(); t != chroma.EOF; t = it() {
		class := hlClass(t.Type)
		parts := strings.Split(t.Value, "\n")
		for i, p := range parts {
			if i > 0 {
				lines = append(lines, nil)
			}
			if p == "" {
				continue
			}
			last := &lines[len(lines)-1]
			if k := len(*last); k > 0 && (*last)[k-1].Class == class {
				(*last)[k-1].Text += p
			} else {
				*last = append(*last, hlTok{Text: p, Class: class})
			}
		}
	}
	return lines
}

// hlClass folds a chroma token type into the page's few highlight codes:
// k keyword, kt type, kc constant or builtin, nf function or class name,
// nt tag, na attribute, s string, m number, c comment, cp preprocessor;
// empty for the rest.
func hlClass(t chroma.TokenType) string {
	switch t {
	case chroma.KeywordType:
		return "kt"
	case chroma.KeywordConstant, chroma.NameBuiltin, chroma.NameBuiltinPseudo, chroma.NameConstant:
		return "kc"
	case chroma.NameFunction, chroma.NameFunctionMagic, chroma.NameClass, chroma.NameDecorator:
		return "nf"
	case chroma.NameTag:
		return "nt"
	case chroma.NameAttribute:
		return "na"
	case chroma.CommentPreproc, chroma.CommentPreprocFile:
		return "cp"
	}
	switch {
	case t.Category() == chroma.Keyword:
		return "k"
	case t.SubCategory() == chroma.LiteralString:
		return "s"
	case t.SubCategory() == chroma.LiteralNumber:
		return "m"
	case t.Category() == chroma.Comment:
		return "c"
	}
	return ""
}

// hlCache is an LRU of highlighted blobs by (blob id, path); only content
// with a blob id is cached, never the working tree's.
type hlCache struct {
	mu  sync.Mutex
	max int
	ll  *list.List
	m   map[string]*list.Element
}

type hlEntry struct {
	key   string
	lines hlLines
}

func newHLCache(max int) *hlCache {
	return &hlCache{max: max, ll: list.New(), m: map[string]*list.Element{}}
}

func (c *hlCache) get(key string) (hlLines, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok {
		return nil, false
	}
	c.ll.MoveToFront(e)
	return e.Value.(*hlEntry).lines, true
}

func (c *hlCache) put(key string, lines hlLines) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.m[key]; ok {
		e.Value.(*hlEntry).lines = lines
		c.ll.MoveToFront(e)
		return
	}
	c.m[key] = c.ll.PushFront(&hlEntry{key: key, lines: lines})
	for c.ll.Len() > c.max {
		last := c.ll.Back()
		c.ll.Remove(last)
		delete(c.m, last.Value.(*hlEntry).key)
	}
}

// gitHL is the daemon's highlight cache.
var gitHL = newHLCache(hlCacheEntries)

// blobHighlight highlights blob oid under path's name, through gitHL; nil
// for no blob (an all-zero id), a read failure, or a blob over hlMaxBytes.
func blobHighlight(ctx context.Context, root, oid, path string) hlLines {
	if !isHexOID(oid) || strings.Trim(oid, "0") == "" {
		return nil
	}
	key := oid + "\x00" + path
	if lines, ok := gitHL.get(key); ok {
		return lines
	}
	out, cut, err := runGitCapped(ctx, root, hlMaxBytes, "--no-optional-locks", "cat-file", "blob", oid)
	if err != nil {
		return nil
	}
	var lines hlLines
	if !cut {
		lines = highlightLines(path, out)
	}
	gitHL.put(key, lines)
	return lines
}

// fileHighlight highlights the working tree's copy of path, read under the
// file view's rule (no symlink followed, inside root); never cached.
func fileHighlight(root, path string) hlLines {
	f, _, _, ok := openContained(root, path)
	if !ok {
		return nil
	}
	defer f.Close()
	content, err := io.ReadAll(io.LimitReader(f, hlMaxBytes+1))
	if err != nil {
		return nil
	}
	return highlightLines(path, content)
}

// handleGitCommitDiff serves GET /s/<reply-id>/git/commit/<oid>/diff?path=:
// one changed file's diff in the commit; 404 for an unknown commit or a
// path the commit does not change, 502 with git's reason when a read fails.
func (d *Daemon) handleGitCommitDiff(w http.ResponseWriter, r *http.Request, replyID, oid string) {
	root, ok := d.sessionRoot(w, replyID)
	if !ok {
		return
	}
	if !isHexOID(oid) {
		http.Error(w, "no such commit", http.StatusNotFound)
		return
	}
	diff, err := readCommitFileDiff(root, oid, r.URL.Query().Get("path"))
	writeFileDiff(w, diff, err)
}

// handleGitWorktreeDiff serves GET /s/<reply-id>/git/worktree/diff?side=&path=:
// one changed file's diff on a side of the working tree; 400 for an unknown
// side, 404 for a path git/status does not list on it.
func (d *Daemon) handleGitWorktreeDiff(w http.ResponseWriter, r *http.Request, replyID string) {
	root, ok := d.sessionRoot(w, replyID)
	if !ok {
		return
	}
	side := r.URL.Query().Get("side")
	if side != "staged" && side != "unstaged" && side != "untracked" {
		http.Error(w, "bad side", http.StatusBadRequest)
		return
	}
	diff, err := readWorktreeFileDiff(root, side, r.URL.Query().Get("path"))
	writeFileDiff(w, diff, err)
}

func writeFileDiff(w http.ResponseWriter, diff GitFileDiff, err error) {
	switch {
	case err == errNoCommit:
		http.Error(w, "no such commit", http.StatusNotFound)
	case err == errNoPath:
		http.Error(w, err.Error(), http.StatusNotFound)
	case err != nil:
		http.Error(w, err.Error(), http.StatusBadGateway)
	default:
		writeGitJSON(w, diff)
	}
}
