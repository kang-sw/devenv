package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
)

// The wiki tools: wiki.query reads a tracker store's wikis and wiki.update
// saves one edit to one, both through wstrackerhost. They pass the tracker
// gate (a top-level lead, or a store's support conversation whose profile
// names them); a support conversation reaches its own store only, for reads
// as well as writes, since a wiki carries no per-store read pin of its own.

const (
	wikiQueryTool  = "wiki.query"
	wikiUpdateTool = "wiki.update"
	// wikiHistoryLimit is the history length a query returns without limit.
	wikiHistoryLimit = 30
)

var (
	wikiQueryOps  = []string{"list", "pages", "read", "search", "history", "diff", "recovery"}
	wikiUpdateOps = []string{wsledger.WikiOpWrite, wsledger.WikiOpRename, wsledger.WikiOpDelete, wsledger.WikiOpRestore}
)

func isWikiTool(name string) bool {
	return name == wikiQueryTool || name == wikiUpdateTool
}

// wikiArgs reads the string arguments names of args; a present argument
// that is not a string is refused.
func wikiArgs(tool string, args map[string]any, names ...string) (map[string]string, error) {
	out := make(map[string]string, len(names))
	for _, name := range names {
		raw, present := args[name]
		if !present || raw == nil {
			continue
		}
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("%s: %s must be a string", tool, name)
		}
		out[name] = strings.TrimSpace(s)
	}
	return out, nil
}

// wikiStore opens the call's store: the caller's key passes the tracker
// gate, a support conversation names its own store, and the store is
// registered on this machine.
func (s *Server) wikiStore(tool string, args map[string]any, store string) (*wstrackerhost.Host, error) {
	_, support, err := s.trackerSessionKey(tool, args)
	if err != nil {
		return nil, err
	}
	if store == "" {
		return nil, fmt.Errorf("%s: store is required: the ID of a registered tracker store, as tracker.query lists them", tool)
	}
	if support != nil && store != support.store {
		return nil, fmt.Errorf("%s: store %q; this conversation reaches the wikis of its own store %q only", tool, store, support.store)
	}
	h, err := s.trackerHost()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", tool, err)
	}
	registered, err := h.Stores()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", tool, err)
	}
	ids := make([]string, 0, len(registered))
	for _, e := range registered {
		if e.ID == store {
			return h, nil
		}
		ids = append(ids, e.ID)
	}
	reason := "no store is registered"
	if len(ids) > 0 {
		reason = "registered: " + strings.Join(ids, ", ")
	}
	return nil, fmt.Errorf("%s: store %q is not a registered store (%s)", tool, store, reason)
}

// wikiFailure is the tool error of a failed call, credentials redacted.
func wikiFailure(id json.RawMessage, tool string, err error) response {
	return toolErrorTextResponse(id, fmt.Sprintf("%s: %s", tool, wstrackerhost.RedactCredentials(err.Error())))
}

func (s *Server) handleWikiQuery(ctx context.Context, id json.RawMessage, args map[string]any) response {
	const tool = wikiQueryTool
	a, err := wikiArgs(tool, args, "store", "op", "wiki", "path", "commit", "query", "from", "to")
	if err != nil {
		return toolErrorTextResponse(id, err.Error())
	}
	h, err := s.wikiStore(tool, args, a["store"])
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	op, wiki, path := a["op"], a["wiki"], a["path"]
	asJSON := wantsJSON(args)
	need := func(names ...string) error {
		var missing []string
		for _, name := range names {
			if a[name] == "" {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("op %s needs %s", op, strings.Join(missing, " and "))
		}
		return nil
	}
	switch op {
	case "list", "recovery":
	case "pages", "history":
		err = need("wiki")
	case "read", "diff":
		err = need("wiki", "path")
	case "search":
		err = need("wiki", "query")
	default:
		err = fmt.Errorf("op %q is not one of %s", op, strings.Join(wikiQueryOps, ", "))
	}
	if op == "diff" && err == nil {
		err = need("to")
	}
	if err != nil {
		return toolErrorTextResponse(id, fmt.Sprintf("%s: %s", tool, err))
	}
	if err := h.PreRead(ctx, a["store"]); err != nil {
		return wikiFailure(id, tool, err)
	}
	store := a["store"]
	var sb strings.Builder
	var value any
	switch op {
	case "list":
		wikis, err := h.WikiList(ctx, store)
		if err != nil {
			return wikiFailure(id, tool, err)
		}
		type row struct {
			Name     string `json:"name"`
			Tip      string `json:"tip"`
			Unpushed bool   `json:"unpushed"`
		}
		rows := make([]row, 0, len(wikis))
		if len(wikis) == 0 {
			fmt.Fprintf(&sb, "store %s has no wikis; create one with %s new_wiki: true\n", store, wikiUpdateTool)
		} else {
			fmt.Fprintf(&sb, "wikis of store %s:\n", store)
		}
		for _, w := range wikis {
			rows = append(rows, row{w.Name, w.Tip, w.Unpushed})
			fmt.Fprintf(&sb, "%s @ %s", w.Name, w.Tip)
			if w.Unpushed {
				sb.WriteString(" (not yet pushed)")
			}
			sb.WriteString("\n")
		}
		value = map[string]any{"store": store, "wikis": rows}
	case "pages":
		pages, at, err := h.WikiPages(ctx, store, wiki, a["commit"])
		if err != nil {
			return wikiFailure(id, tool, err)
		}
		fmt.Fprintf(&sb, "wiki %s @ %s: %s\n", wiki, at, trackerCount(len(pages), "page"))
		for _, p := range pages {
			sb.WriteString(p + "\n")
		}
		if pages == nil {
			pages = []string{}
		}
		value = map[string]any{"wiki": wiki, "commit": at, "pages": pages}
	case "read":
		content, at, err := h.WikiRead(ctx, store, wiki, path, a["commit"])
		if err != nil {
			return wikiFailure(id, tool, err)
		}
		fmt.Fprintf(&sb, "wiki %s page %s @ %s\n", wiki, path, at)
		fmt.Fprintf(&sb, "base for %s: %s\n---\n%s", wikiUpdateTool, at, content)
		value = map[string]any{"wiki": wiki, "path": path, "commit": at, "content": content}
	case "search":
		hits, at, err := h.WikiSearch(ctx, store, wiki, a["query"])
		if err != nil {
			return wikiFailure(id, tool, err)
		}
		total := len(hits)
		if limit := intFromArgument(args["limit"], 0); limit > 0 && len(hits) > limit {
			hits = hits[:limit]
		}
		type match struct {
			Path string `json:"path"`
			Line int    `json:"line"`
			Text string `json:"text"`
		}
		rows := make([]match, 0, len(hits))
		fmt.Fprintf(&sb, "wiki %s @ %s: %s for %q", wiki, at, trackerCount(total, "hit"), a["query"])
		if len(hits) < total {
			fmt.Fprintf(&sb, ", first %d shown", len(hits))
		}
		sb.WriteString("\n")
		for _, m := range hits {
			rows = append(rows, match{m.Path, m.Line, m.Text})
			fmt.Fprintf(&sb, "%s:%d: %s\n", m.Path, m.Line, m.Text)
		}
		value = map[string]any{"wiki": wiki, "commit": at, "total": total, "matches": rows}
	case "history":
		limit := intFromArgument(args["limit"], wikiHistoryLimit)
		commits, err := h.WikiHistory(ctx, store, wiki, path, limit)
		if err != nil {
			return wikiFailure(id, tool, err)
		}
		type commit struct {
			OID        string   `json:"oid"`
			Short      string   `json:"short"`
			Time       string   `json:"time"`
			Author     string   `json:"author"`
			Verified   bool     `json:"verified"`
			Signer     string   `json:"signer,omitempty"`
			Unverified string   `json:"unverified,omitempty"`
			Subject    string   `json:"subject"`
			Message    string   `json:"message"`
			Parents    []string `json:"parents"`
		}
		rows := make([]commit, 0, len(commits))
		what := "wiki " + wiki
		if path != "" {
			what += " page " + path
		}
		fmt.Fprintf(&sb, "%s: %s, newest first\n", what, trackerCount(len(commits), "commit"))
		for _, c := range commits {
			subject, _, _ := strings.Cut(c.Message, "\n")
			verdict := "verified, signed by " + c.Signer
			if !c.Verified {
				verdict = "unverified: " + c.Unverified
			}
			parents := c.Parents
			if parents == nil {
				parents = []string{}
			}
			rows = append(rows, commit{c.OID, wikiShort(c.OID), c.Time.Format(time.RFC3339), c.Author, c.Verified, c.Signer, c.Unverified, subject, c.Message, parents})
			fmt.Fprintf(&sb, "%s %s %s (%s; %s) %s\n", c.OID, c.Time.Format(time.RFC3339), c.Author, verdict, trackerCount(len(c.Parents), "parent"), subject)
		}
		if limit > 0 && len(commits) == limit {
			fmt.Fprintf(&sb, "(limit %d reached; pass a larger limit for older commits)\n", limit)
		}
		value = map[string]any{"wiki": wiki, "path": path, "commits": rows}
	case "diff":
		patch, err := h.WikiDiff(ctx, store, wiki, path, a["from"], a["to"])
		if err != nil {
			return wikiFailure(id, tool, err)
		}
		from := a["from"]
		label := from
		if label == "" {
			label = "its parent"
		}
		fmt.Fprintf(&sb, "wiki %s page %s: %s against %s\n", wiki, path, a["to"], label)
		if strings.TrimSpace(patch) == "" {
			sb.WriteString("(no change to this page)\n")
		} else {
			sb.WriteString(patch)
		}
		value = map[string]any{"wiki": wiki, "path": path, "from": from, "to": a["to"], "patch": patch}
	case "recovery":
		points, err := h.WikiRecovery(ctx, store, wiki)
		if err != nil {
			return wikiFailure(id, tool, err)
		}
		type point struct {
			Wiki    string `json:"wiki"`
			OID     string `json:"oid"`
			Time    string `json:"time"`
			Subject string `json:"subject"`
		}
		rows := make([]point, 0, len(points))
		if len(points) == 0 {
			sb.WriteString("no recovery points: no followed rewind or deletion replaced a wiki tip on this machine\n")
		} else {
			fmt.Fprintf(&sb, "%s, newest first:\n", trackerCount(len(points), "recovery point"))
		}
		for _, p := range points {
			rows = append(rows, point{p.Wiki, p.OID, p.Time.Format(time.RFC3339), p.Subject})
			fmt.Fprintf(&sb, "%s %s %s %s\n", p.Wiki, p.OID, p.Time.Format(time.RFC3339), p.Subject)
		}
		if len(points) > 0 {
			fmt.Fprintf(&sb, "Read a point's pages with op pages or read and commit=<oid>. Restore a page from one with %s op=restore commit=<oid> path=<page> and the wiki's current base, or new_wiki: true when the wiki no longer exists.\n", wikiUpdateTool)
		}
		value = map[string]any{"points": rows}
	}
	if asJSON {
		return toolJSONResponse(id, value, nil)
	}
	return toolTextResponse(id, sb.String(), nil)
}

func (s *Server) handleWikiUpdate(ctx context.Context, id json.RawMessage, args map[string]any) response {
	const tool = wikiUpdateTool
	a, err := wikiArgs(tool, args, "store", "wiki", "op", "base", "path", "to", "commit", "message")
	if err != nil {
		return toolErrorTextResponse(id, err.Error())
	}
	// content is page text: kept as given, whitespace included.
	content := ""
	if raw, present := args["content"]; present && raw != nil {
		str, ok := raw.(string)
		if !ok {
			return toolErrorTextResponse(id, tool+": content must be a string")
		}
		content = str
	}
	if raw, present := args["new_wiki"]; present && raw != nil {
		if _, ok := raw.(bool); !ok {
			return toolErrorTextResponse(id, tool+": new_wiki must be a boolean")
		}
	}
	h, err := s.wikiStore(tool, args, a["store"])
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	op := a["op"]
	var problems []string
	switch op {
	case wsledger.WikiOpWrite:
		if _, present := args["content"]; !present {
			problems = append(problems, "op write needs content (the whole page text)")
		}
	case wsledger.WikiOpRename:
		if a["to"] == "" {
			problems = append(problems, "op rename needs to (the new page path)")
		}
	case wsledger.WikiOpRestore:
		if a["commit"] == "" {
			problems = append(problems, "op restore needs commit (the commit to restore the page from)")
		}
	case wsledger.WikiOpDelete:
	default:
		problems = append(problems, fmt.Sprintf("op %q is not one of %s", op, strings.Join(wikiUpdateOps, ", ")))
	}
	images, names, imageProblems := wikiImages(args["images"])
	problems = append(problems, imageProblems...)
	if len(images) > 0 && op != wsledger.WikiOpWrite {
		problems = append(problems, "images go with op write only")
	}
	if len(problems) > 0 {
		return toolErrorTextResponse(id, fmt.Sprintf("%s: %s", tool, strings.Join(problems, "; ")))
	}
	edit := wsledger.WikiEdit{
		Wiki:    a["wiki"],
		Base:    a["base"],
		NewWiki: boolArgument(args["new_wiki"]),
		Op:      op,
		Path:    a["path"],
		To:      a["to"],
		Content: content,
		Commit:  a["commit"],
		Message: a["message"],
	}
	res, err := h.WikiSave(ctx, a["store"], wstrackerhost.WikiSave{Edit: edit, Images: images})
	asJSON := wantsJSON(args)
	var conflict *wsledger.WikiConflict
	if errors.As(err, &conflict) {
		return wikiConflictResponse(id, conflict, asJSON)
	}
	if err != nil {
		return wikiFailure(id, tool, err)
	}
	type image struct {
		Name string `json:"name,omitempty"`
		OID  string `json:"oid"`
	}
	written := make([]image, 0, len(images))
	for i, img := range images {
		written = append(written, image{names[i], wikiImageOID(img.Data)})
	}
	type warning struct {
		Ref    string `json:"ref"`
		Kind   string `json:"kind"`
		Detail string `json:"detail"`
	}
	warnings := make([]warning, 0, len(res.Warnings))
	for _, w := range res.Warnings {
		warnings = append(warnings, warning{w.Ref, w.Kind, wstrackerhost.RedactCredentials(w.Detail)})
	}
	if asJSON {
		return toolJSONResponse(id, map[string]any{
			"wiki": edit.Wiki, "op": op, "path": edit.Path, "commit": res.Commit,
			"merged": res.Merged, "created": res.Created, "images": written, "warnings": warnings,
		}, nil)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "wiki %s: saved %s %s\n", edit.Wiki, op, edit.Path)
	fmt.Fprintf(&sb, "commit: %s (the base for your next save)\n", res.Commit)
	if res.Created {
		sb.WriteString("created the wiki\n")
	}
	if res.Merged {
		sb.WriteString("merged: the wiki had moved since your base, and your edit merged cleanly with it\n")
	}
	for i, img := range written {
		label := img.Name
		if label == "" {
			label = "#" + strconv.Itoa(i+1)
		}
		fmt.Fprintf(&sb, "image %s: attachment:%s\n", label, img.OID)
	}
	if len(written) > 0 {
		sb.WriteString("Reference an image in a page as ![alt](attachment:<sha256>).\n")
	}
	for _, w := range warnings {
		fmt.Fprintf(&sb, "warning: %s %s: %s\n", w.Kind, w.Ref, w.Detail)
	}
	return toolTextResponse(id, sb.String(), nil)
}

// wikiConflictResponse is a refused save's tool error: what conflicted,
// the tip to save on next, and both texts to merge.
func wikiConflictResponse(id json.RawMessage, c *wsledger.WikiConflict, asJSON bool) response {
	if asJSON {
		raw, err := json.Marshal(map[string]any{"conflict": map[string]any{
			"wiki": c.Wiki, "path": c.Path, "kind": c.Kind, "reason": c.Reason,
			"base": c.Base, "theirs": c.Theirs, "ours_text": c.OursText, "theirs_text": c.TheirsText,
		}})
		if err != nil {
			return toolErrorTextResponse(id, err.Error())
		}
		return toolErrorTextResponse(id, string(raw)+"\n")
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s: conflict, nothing saved\n", wikiUpdateTool)
	fmt.Fprintf(&sb, "wiki: %s\npath: %s\nkind: %s\nreason: %s\nbase: %s\n", c.Wiki, c.Path, c.Kind, c.Reason, c.Base)
	if c.Theirs != "" {
		fmt.Fprintf(&sb, "theirs: %s\n", c.Theirs)
		sb.WriteString("Merge the two texts below and save again with base set to theirs.\n")
	} else {
		sb.WriteString("theirs: (the wiki no longer exists)\n")
		sb.WriteString("Merge the two texts below and save again with new_wiki: true.\n")
	}
	fmt.Fprintf(&sb, "\n===== ours_text (your page) =====\n%s", c.OursText)
	if c.OursText != "" && !strings.HasSuffix(c.OursText, "\n") {
		sb.WriteString("\n")
	}
	fmt.Fprintf(&sb, "===== theirs_text (the page at theirs) =====\n%s", c.TheirsText)
	if c.TheirsText != "" && !strings.HasSuffix(c.TheirsText, "\n") {
		sb.WriteString("\n")
	}
	sb.WriteString("===== end =====\n")
	return toolErrorTextResponse(id, sb.String())
}

// wikiImages decodes the images argument: an array of {data, name} whose
// data is base64 (standard or URL alphabet, padded or not, an optional
// data: URL prefix and whitespace tolerated).
func wikiImages(raw any) ([]wstrackerhost.WikiImageInput, []string, []string) {
	if raw == nil {
		return nil, nil, nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, nil, []string{"images must be an array of {data, name} objects"}
	}
	var images []wstrackerhost.WikiImageInput
	var names, problems []string
	for i, item := range list {
		obj, ok := item.(map[string]any)
		if !ok {
			problems = append(problems, fmt.Sprintf("images[%d] must be an object {data, name}", i))
			continue
		}
		for key := range obj {
			if key != "data" && key != "name" {
				problems = append(problems, fmt.Sprintf("images[%d].%s is not an image field (data, name)", i, key))
			}
		}
		name, _ := obj["name"].(string)
		data, _ := obj["data"].(string)
		if strings.TrimSpace(data) == "" {
			problems = append(problems, fmt.Sprintf("images[%d].data must be the image's base64 bytes", i))
			continue
		}
		decoded, err := decodeWikiBase64(data)
		if err != nil {
			problems = append(problems, fmt.Sprintf("images[%d].data is not base64: %v", i, err))
			continue
		}
		images = append(images, wstrackerhost.WikiImageInput{Data: decoded})
		names = append(names, strings.TrimSpace(name))
	}
	return images, names, problems
}

func decodeWikiBase64(s string) ([]byte, error) {
	if rest, ok := strings.CutPrefix(strings.TrimSpace(s), "data:"); ok {
		if _, payload, ok := strings.Cut(rest, ";base64,"); ok {
			s = payload
		}
	}
	s = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\r', '\n':
			return -1
		}
		return r
	}, s)
	s = strings.TrimRight(s, "=")
	if strings.ContainsAny(s, "-_") {
		return base64.RawURLEncoding.DecodeString(s)
	}
	return base64.RawStdEncoding.DecodeString(s)
}

// wikiImageOID is the sha256 an image is stored and referenced under.
func wikiImageOID(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func wikiShort(oid string) string {
	if len(oid) > 12 {
		return oid[:12]
	}
	return oid
}
