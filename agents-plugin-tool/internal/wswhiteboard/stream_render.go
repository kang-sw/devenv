package wswhiteboard

// entryData pairs an entry with the rendering page without changing its model.
type entryData struct {
	EntryView
	Page *pageData
}

func (p *pageData) EntryOf(e EntryView) entryData { return entryData{EntryView: e, Page: p} }

func (d entryData) IsPanel() bool { return d.Kind == EntryPanel }

func (d entryData) PayloadClass() string {
	if d.Page.DocFramed {
		return "framed"
	}
	return "md"
}

type entryPageData struct {
	Entries EntryPage
	Page    *pageData
}

func (p *pageData) EntryPageOf(entries EntryPage) entryPageData {
	return entryPageData{Entries: entries, Page: p}
}
