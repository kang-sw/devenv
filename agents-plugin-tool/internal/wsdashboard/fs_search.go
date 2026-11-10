package wsdashboard

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// The file browser's two searches, both over the browsing root's file list
// (the list the files route caches per root: git's tracked and untracked
// but not ignored files, or a capped walk outside a repository):
//
//	GET /s/<reply-id>/fs/find?q=<text>[&root=<abs>]
//	GET /s/<reply-id>/fs/search?q=<text>[&case=1][&regex=1][&root=<abs>]
//
// find is the name search: the composer's file mention ranking (rankRoot)
// of the list, files and the directories they sit in.
//
// search is the content search. The text is a literal matched without case
// unless case=1; regex=1 reads it as a regular expression instead (RE2, so
// its running time is linear in the file). It reads each listed file inside
// the root as the preview does (openContained), skips a file over the text
// cap or one that is not UTF-8 text, and answers the matching lines by
// file. It stops at fsSearchFiles files, at fsSearchMatches lines, or at
// fsSearchTimeout, and then says more.
//
// An ignored file is in neither answer, since the list leaves it out, even
// while the explorer shows ignored entries.

const (
	fsSearchFiles    = 200
	fsSearchMatches  = 2000
	fsSearchQueryMax = 1024
	fsSearchTimeout  = 10 * time.Second
	// fsSearchPre and fsSearchSide bound a matching line's excerpt, in
	// runes: the text before the match, and the match and the text after it.
	fsSearchPre  = 48
	fsSearchSide = 160
)

// fsMatch is one matching line: its 1-based number and the line cut around
// its first match.
type fsMatch struct {
	Line int    `json:"line"`
	Pre  string `json:"pre"`
	Hit  string `json:"hit"`
	Post string `json:"post"`
}

type fsSearchFile struct {
	Path    string    `json:"path"`
	Matches []fsMatch `json:"matches"`
}

type fsSearchAnswer struct {
	Files []fsSearchFile `json:"files"`
	More  bool           `json:"more,omitempty"`
}

type fsFindAnswer struct {
	Items []fileItem `json:"items"`
}

// searchPattern compiles a content search: q as a literal, or as a regular
// expression, without case unless matchCase.
func searchPattern(q string, matchCase, regex bool) (*regexp.Regexp, error) {
	if !regex {
		q = regexp.QuoteMeta(q)
	}
	if !matchCase {
		q = "(?i)" + q
	}
	return regexp.Compile(q)
}

// firstRunes and lastRunes cut s to at most n runes, marking the cut.
func firstRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

func lastRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return "…" + string(r[len(r)-n:])
}

// matchLines appends the lines of text the pattern matches, up to room of
// them. A match of no text is no match.
func matchLines(re *regexp.Regexp, text string, room int) (out []fsMatch, full bool) {
	for n := 1; text != ""; n++ {
		line, rest, _ := strings.Cut(text, "\n")
		text = rest
		loc := re.FindStringIndex(line)
		if loc == nil || loc[0] == loc[1] {
			continue
		}
		if len(out) >= room {
			return out, true
		}
		line = strings.TrimSuffix(line, "\r")
		if loc[1] > len(line) {
			loc[1] = len(line)
		}
		out = append(out, fsMatch{Line: n,
			Pre:  lastRunes(strings.TrimLeft(line[:loc[0]], " \t"), fsSearchPre),
			Hit:  firstRunes(line[loc[0]:loc[1]], fsSearchSide),
			Post: firstRunes(line[loc[1]:], fsSearchSide)})
	}
	return out, false
}

// searchFiles runs a content search over the listed files of root.
func searchFiles(ctx context.Context, root string, list []fileCandidate, re *regexp.Regexp) fsSearchAnswer {
	a := fsSearchAnswer{Files: []fsSearchFile{}}
	total := 0
	for _, c := range list {
		if c.dir {
			continue
		}
		if ctx.Err() != nil {
			a.More = true
			break
		}
		text, ok := searchText(root, c.path)
		if !ok {
			continue
		}
		matches, full := matchLines(re, text, fsSearchMatches-total)
		if len(matches) > 0 {
			if len(a.Files) >= fsSearchFiles {
				a.More = true
				break
			}
			a.Files = append(a.Files, fsSearchFile{Path: c.path, Matches: matches})
			total += len(matches)
		}
		if full {
			a.More = true
			break
		}
	}
	return a
}

// searchText reads the listed file rel under root for a search: false for
// a file the preview would not show as text (outside the root, over the
// text cap, or not UTF-8 text).
func searchText(root, rel string) (string, bool) {
	f, _, info, ok := openContained(root, rel)
	if !ok {
		return "", false
	}
	defer f.Close()
	if info.Size() > fileViewMax {
		return "", false
	}
	data, err := io.ReadAll(io.LimitReader(f, fileViewMax+1))
	if err != nil || len(data) > fileViewMax || !utf8.Valid(data) || strings.IndexByte(string(data), 0) >= 0 {
		return "", false
	}
	return string(data), true
}

// handleFSFind serves the name search.
func (d *Daemon) handleFSFind(w http.ResponseWriter, r *http.Request, replyID string) {
	root, session, ok := d.browseRoot(r, replyID)
	q := r.URL.Query().Get("q")
	if !ok || len(q) > filesQueryMax || strings.ContainsRune(q, 0) {
		fsNotFound(w)
		return
	}
	items := rankRoot(d.files.get(root), q)
	if items == nil {
		items = []fileItem{}
	}
	writeFSJSON(w, !session, fsFindAnswer{Items: items})
}

// handleFSSearch serves the content search. A text that does not compile
// as a regular expression answers 400 with the reason.
func (d *Daemon) handleFSSearch(w http.ResponseWriter, r *http.Request, replyID string) {
	root, session, ok := d.browseRoot(r, replyID)
	query := r.URL.Query()
	q := query.Get("q")
	if !ok || len(q) > fsSearchQueryMax || strings.ContainsRune(q, 0) {
		fsNotFound(w)
		return
	}
	if q == "" {
		writeFSJSON(w, !session, fsSearchAnswer{Files: []fsSearchFile{}})
		return
	}
	re, err := searchPattern(q, query.Get("case") == "1", query.Get("regex") == "1")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), fsSearchTimeout)
	defer cancel()
	writeFSJSON(w, !session, searchFiles(ctx, root, d.files.get(root), re))
}
