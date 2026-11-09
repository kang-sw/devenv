package wswhiteboard

import "html/template"

// TextTargets is the file-target rule a RenderText caller serves its text
// under, and whether that text may hold HTML; FileText, ImageSrc,
// FileActions, and OutsideImage have the View meaning of the same name.
type TextTargets struct {
	// FileText shows a file path a link or image names as text. Nil shows
	// the path as written, never as a file: or relative link.
	FileText func(path string) string
	// ImageSrc serves an image the caller's server can fetch; empty leaves
	// the image as path text.
	ImageSrc func(path string) string
	// FileActions turns a file target into the served page's action link;
	// nil keeps FileText's path text and leaves http(s) links unmarked.
	FileActions func(path string) FileAction
	// OutsideImage serves an image outside the session root to the viewers
	// that may load it (View.OutsideImage); nil leaves it to the rules above.
	OutsideImage func(path string) FileAction
	// AttachmentSrc serves an image whose destination is attachment:<oid>
	// (a store attachment named by its content hash): the src it loads
	// from, a path on the serving daemon, or empty to leave the image as
	// its alt text. Nil leaves every such image as alt text, and a link to
	// one is never an anchor.
	AttachmentSrc func(oid string) string
	// WikiLink serves a link whose destination is wiki:<name>/<path> (a page
	// of a store wiki): the href the anchor opens, a fragment or a path on
	// the serving daemon, or empty to leave the link as its label text. The
	// anchor also carries data-wiki="<name>" and data-page="<path>" for the
	// host page to key on. Nil leaves every such link as text, and a wiki:
	// image is always its alt text.
	WikiLink func(name, path string) string
	// Mermaid renders a mermaid fence as <pre class="mermaid"> for the host
	// page to draw. Off, it is a plain code block: for text that re-renders
	// as it streams, where a diagram would redraw on every tick, and for a
	// host that draws no mermaid.
	Mermaid bool
	// HTML renders the HTML the text holds through the reply sanitizer
	// (sanitize.go) instead of escaping it. Set it only for a finished lead
	// reply: other authors are a different trust boundary, and a reply still
	// streaming would shake its layout on every half-closed tag.
	HTML bool
}

// RenderText renders s as block markdown outside a whiteboard page, through
// the same pipeline and escaping boundary as an item's block text: a tag
// the author types is escaped text, and every destination goes through
// resolveTarget's allow-list. With t.HTML the tag is kept instead, inside
// the sanitizer's allow-list and with its URLs resolved by the same rule.
// Item references stay literal text.
func RenderText(s string, t TextTargets) template.HTML {
	fileText := t.FileText
	if fileText == nil {
		fileText = func(string) string { return "" }
	}
	p := &pageData{View: View{FileText: fileText, ImageSrc: t.ImageSrc, FileActions: t.FileActions, OutsideImage: t.OutsideImage}, noMermaid: !t.Mermaid, noInlineTags: true, attachmentSrc: t.AttachmentSrc, wikiLink: t.WikiLink}
	if !t.HTML {
		return p.Block(s)
	}
	p.trusted = &[]trustedTarget{}
	return p.sanitizeReply(string(p.convert(p.rawMarkdown(), s)))
}
