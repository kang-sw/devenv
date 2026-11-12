package wswhiteboard

import (
	"html/template"
	"regexp"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

// bulkCountChip is a topic bar's bulk-answer count chip, as the regions
// render it (BulkChips).
var bulkCountChip = regexp.MustCompile(`<button type="button" class="chip (policy|ok) bulk" data-bulk="\w+" title="[^"]*">(.*?)</button>`)

// StaticCountChips turns the bulk-answer count chips of served board HTML
// back into the plain count chips the static page renders in the default
// language. The count chips are the one difference between a served board
// region and the static page that holds no topic body, so a test in this or
// another package compares the two through it, and every such comparison
// follows one rule.
func StaticCountChips(board string) string {
	return bulkCountChip.ReplaceAllStringFunc(board, func(m string) string {
		g := bulkCountChip.FindStringSubmatch(m)
		key := map[string]string{"policy": "chip.answer", "ok": "chip.ack"}[g[1]]
		return `<span class="chip ` + g[1] + `" title="` + template.HTMLEscapeString(wsi18n.T(wsi18n.Default, key)) + `">` + g[2] + `</span>`
	})
}
