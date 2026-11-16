package wsdashboard

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

func TestRailFavoriteMarkup(t *testing.T) {
	for _, lang := range wsi18n.Languages() {
		for _, state := range []struct {
			name                     string
			starred, hidden, archive bool
		}{
			{name: "plain"},
			{name: "favorite", starred: true},
			{name: "child", starred: true, hidden: true},
			{name: "archived", starred: true, archive: true},
		} {
			t.Run(lang.Code+"/"+state.name, func(t *testing.T) {
				view := RailView{Workspaces: []RailWorkspace{{Name: "project", Path: "/project", Sessions: []RailRow{{
					ReplyID: "row", Harness: "pi", Live: true, Starred: state.starred, StarHidden: state.hidden, Archived: state.archive,
				}}}}}
				region, err := renderRailRegion(view, lang.Code)
				if err != nil {
					t.Fatal(err)
				}
				html := string(region.HTML)
				eligible := !state.hidden && !state.archive
				if strings.Contains(html, `class="star"`) || strings.Contains(html, `data-star=`) != eligible {
					t.Fatal("row has an inline star button or wrong favorite eligibility")
				}
				badge := eligible && state.starred
				if strings.Contains(html, `class="hstar"`) != badge {
					t.Fatal("wrong favorite badge visibility")
				}
				if badge && !strings.Contains(html, `class="hstar" role="img" aria-label="`+wsi18n.T(lang.Code, "js.rail.starred")+`"`) {
					t.Fatal("favorite badge lacks its localized accessible label")
				}
			})
		}
	}
}
