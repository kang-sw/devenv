package wswhiteboard

import (
	"slices"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// FileImagePaths finds file-image dependencies in the complete board, including
// deferred topics and historical entries, without rendering their templates or
// frames. It uses the renderer's parser/target rules, so code and remote URLs
// do not become file watches; raw HTML images take the same path as markdown.
// Missing files are dependencies too. The caller may cache by board revision.
func FileImagePaths(state *State) []string {
	if state == nil {
		return nil
	}
	paths := map[string]bool{}
	p := &pageData{framing: true, inDoc: true}
	p.FileText = func(path string) string { return path }
	p.DocImageSrc = func(path string) string { paths[path] = true; return "" }
	scan := func(s string) {
		if !strings.Contains(s, "![") && !(strings.Contains(s, "<") && strings.Contains(strings.ToLower(s), "<img")) {
			return
		}
		src := []byte(s)
		_ = ast.Walk(p.rawMarkdown().Parser().Parse(text.NewReader(src)), func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			switch n := n.(type) {
			case *ast.Image:
				p.resolveTarget(string(n.Destination), true)
			case *ast.RawHTML:
				var raw []byte
				for i := 0; i < n.Segments.Len(); i++ {
					seg := n.Segments.At(i)
					raw = append(raw, seg.Value(src)...)
				}
				p.rawImages(raw)
			case *ast.HTMLBlock:
				p.rawImages(n.Lines().Value(src))
			}
			return ast.WalkContinue, nil
		})
	}
	for _, topic := range state.Topics {
		scan(topic.Body)
		scan(topic.Recap)
		for _, entry := range topic.Entries {
			scan(entry.Text)
			scan(entry.Recap)
		}
	}
	for _, item := range state.Items {
		for _, s := range append([]string{item.Title, item.Rationale, item.Recommendation, item.Answer, item.AnswerQuote}, item.Options...) {
			scan(s)
		}
	}
	for _, result := range state.CloseResults {
		scan(result)
	}
	out := make([]string, 0, len(paths))
	for path := range paths {
		out = append(out, path)
	}
	slices.Sort(out)
	return out
}
