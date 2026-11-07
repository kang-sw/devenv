package wswhiteboard

import "html/template"

// TextTargets is the file-target rule a RenderText caller serves its text
// under; FileText, ImageSrc, and FileActions have the View meaning of the
// same name.
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
	// Mermaid renders a mermaid fence as <pre class="mermaid"> for the host
	// page to draw. Off, it is a plain code block: for text that re-renders
	// as it streams, where a diagram would redraw on every tick, and for a
	// host that draws no mermaid.
	Mermaid bool
}

// RenderText renders s as block markdown outside a whiteboard page, through
// the same pipeline and escaping boundary as an item's block text: a tag
// the author types is escaped text, and every destination goes through
// resolveTarget's allow-list. Item references stay literal text.
func RenderText(s string, t TextTargets) template.HTML {
	fileText := t.FileText
	if fileText == nil {
		fileText = func(string) string { return "" }
	}
	p := &pageData{View: View{FileText: fileText, ImageSrc: t.ImageSrc, FileActions: t.FileActions}, noMermaid: !t.Mermaid}
	return p.Block(s)
}
