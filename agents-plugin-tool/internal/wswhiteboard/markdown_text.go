package wswhiteboard

import "html/template"

// TextTargets is the file-target rule a RenderText caller serves its text
// under; both fields have the View meaning of the same name.
type TextTargets struct {
	// FileText shows a file path a link or image names as text. Nil shows
	// the path as written, never as a file: or relative link.
	FileText func(path string) string
	// ImageSrc serves an image the caller's server can fetch; empty leaves
	// the image as path text.
	ImageSrc func(path string) string
}

// RenderText renders s as block markdown outside a whiteboard page, through
// the same pipeline and escaping boundary as an item's block text: a tag
// the author types is escaped text, and every destination goes through
// resolveTarget's allow-list. Item references stay literal text, and a
// mermaid fence renders as a plain code block, since the host page may draw
// mermaid on the board only.
func RenderText(s string, t TextTargets) template.HTML {
	fileText := t.FileText
	if fileText == nil {
		fileText = func(string) string { return "" }
	}
	p := &pageData{View: View{FileText: fileText, ImageSrc: t.ImageSrc}, noMermaid: true}
	return p.Block(s)
}
