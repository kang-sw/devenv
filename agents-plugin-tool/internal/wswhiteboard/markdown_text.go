package wswhiteboard

import "html/template"

// TextTargets is the file-target rule a RenderText caller serves its text
// under, and whether that text may hold HTML; FileText and ImageSrc have the
// View meaning of the same name.
type TextTargets struct {
	// FileText shows a file path a link or image names as text. Nil shows
	// the path as written, never as a file: or relative link.
	FileText func(path string) string
	// ImageSrc serves an image the caller's server can fetch; empty leaves
	// the image as path text.
	ImageSrc func(path string) string
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
// Item references stay literal text, and a mermaid fence renders as a plain
// code block, since the host page may draw mermaid on the board only.
func RenderText(s string, t TextTargets) template.HTML {
	fileText := t.FileText
	if fileText == nil {
		fileText = func(string) string { return "" }
	}
	p := &pageData{View: View{FileText: fileText, ImageSrc: t.ImageSrc}, noMermaid: true}
	if !t.HTML {
		return p.Block(s)
	}
	p.trusted = &[]trustedURL{}
	return p.sanitizeReply(string(convert(p.rawMarkdown(), s)))
}
