package wswhiteboard

import (
	"strings"
	"testing"
)

func TestSelectAllPageAssetsExcludeSandboxFrames(t *testing.T) {
	a, err := loadAssets()
	if err != nil {
		t.Fatal(err)
	}
	shared, err := PageAssets()
	if err != nil {
		t.Fatal(err)
	}
	if a.selectAll == "" || string(shared.SelectAllScript) != a.selectAll {
		t.Fatal("dashboard must receive the standalone page's selection handler")
	}
	for _, layout := range layouts() {
		page := render(t, sampleView(layout))
		if !strings.Contains(page, a.selectAll) {
			t.Errorf("%v page omits the selection handler", layout)
		}
	}
	if strings.Contains(a.docFrame, a.selectAll) || strings.Contains(a.docFrame, "var dialogs =") {
		t.Fatal("sandboxed topic frames must keep their own selection behavior")
	}
}
