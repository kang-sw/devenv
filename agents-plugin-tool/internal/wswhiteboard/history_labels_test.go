package wswhiteboard

import (
	"testing"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

func TestHistoryLabelsKeepLocalizedClassAndStatusMappings(t *testing.T) {
	classes := []struct {
		item Item
		key  string
	}{
		{Item{Class: ClassPolicy}, "wb.history.policy"},
		{Item{Class: ClassTechnical}, "wb.history.technical"},
		{Item{Class: ClassAnnounced}, "wb.history.announced"},
		{Item{Class: ClassCheck}, "wb.history.check"},
	}
	statuses := []struct {
		item Item
		key  string
	}{
		{Item{Status: StatusAssumed}, "wb.history.assumed"},
		{Item{Status: StatusWithdrawn}, "wb.history.withdrawn"},
		{Item{Status: StatusSuperseded}, "wb.history.superseded"},
		{Item{Status: StatusAnswered}, "wb.history.answered"},
	}
	for _, lang := range []string{"en", "ko"} {
		page := &pageData{View: View{Lang: lang}}
		for _, tc := range classes {
			d := itemData{ItemView: ItemView{Class: tc.item.Class}, Page: page}
			if got, want := d.HistoryClassLabel(), wsi18n.T(lang, tc.key); got != want {
				t.Errorf("%s class %s: got %q, want %q", lang, tc.item.Class, got, want)
			}
		}
		for _, tc := range statuses {
			d := itemData{ItemView: ItemView{Status: tc.item.Status}, Page: page}
			if got, want := d.HistoryStatusLabel(), wsi18n.T(lang, tc.key); got != want {
				t.Errorf("%s status %s: got %q, want %q", lang, tc.item.Status, got, want)
			}
		}
	}
}
