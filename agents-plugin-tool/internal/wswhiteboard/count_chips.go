package wswhiteboard

import (
	"html/template"
	"regexp"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

// bulkCountChip is a topic bar's bulk-answer count chip, as the regions
// render it (BulkChips).
var bulkCountChip = regexp.MustCompile(`<button type="button" class="chip (policy|ok) bulk" data-bulk="\w+" title="[^"]*">(.*?)</button>`)

// topicHashAttr is a served topic section's data-th (TopicHTML).
var topicHashAttr = regexp.MustCompile(` data-th="[0-9a-f]{16}"`)

// StaticCountChips turns the bulk-answer count chips of served board HTML
// back into the plain count chips the static page renders in the default
// language, and drops the topic sections' data-th hashes, which the static
// page does not carry. These are the differences between a served board
// region and the static page that holds no topic body, so a test in this or
// another package compares the two through it, and every such comparison
// follows one rule. It is test support only: no production path calls it.
func StaticCountChips(board string) string {
	board = topicHashAttr.ReplaceAllString(board, "")
	return bulkCountChip.ReplaceAllStringFunc(board, func(m string) string {
		g := bulkCountChip.FindStringSubmatch(m)
		key := map[string]string{"policy": "chip.answer", "ok": "chip.ack"}[g[1]]
		return `<span class="chip ` + g[1] + `" title="` + template.HTMLEscapeString(wsi18n.T(wsi18n.Default, key)) + `">` + g[2] + `</span>`
	})
}
