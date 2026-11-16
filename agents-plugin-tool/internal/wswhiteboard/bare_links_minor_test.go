package wswhiteboard

import (
	"strings"
	"testing"
)

func TestBarePathsPreserveEmphasisAndHomeLinks(t *testing.T) {
	page := &pageData{View: View{FileLink: func(p string) string { return "L/" + p }}}
	targets := TextTargets{FileActions: func(p string) FileAction {
		return FileAction{Href: "L/" + p, Abs: p, Rel: p}
	}}
	finished := targets
	finished.HTML = true
	for name, render := range map[string]func(string) string{
		"board":    func(s string) string { return string(page.Block(s)) },
		"text":     func(s string) string { return string(RenderText(s, targets)) },
		"finished": func(s string) string { return string(RenderText(s, finished)) },
	} {
		if got := render("_docs/a.md_"); !strings.Contains(got, "<em>docs/a.md</em>") {
			t.Errorf("%s: underscore emphasis lost: %s", name, got)
		}
		for _, pair := range [][2]string{
			{"*docs/a.md*", "*[docs/a.md](docs/a.md)*"},
			{"**docs/a.md**", "**[docs/a.md](docs/a.md)**"},
			{"~/docs/a.md", "[~/docs/a.md](~/docs/a.md)"},
			{"see ~/docs/a.md", "see [~/docs/a.md](~/docs/a.md)"},
			{"*~/docs/a.md*", "*[~/docs/a.md](~/docs/a.md)*"},
		} {
			got, want := render(pair[0]), render(pair[1])
			if got != want || !strings.Contains(got, "<a ") {
				t.Errorf("%s %q: got %s; want %s", name, pair[0], got, want)
			}
		}
	}
}
