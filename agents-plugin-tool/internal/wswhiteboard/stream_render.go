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

// entryPageData is one history page to render; Tail marks the newest page,
// whose last log run keeps its newest rows visible (EntryPage.Segments).
type entryPageData struct {
	Entries EntryPage
	Tail    bool
	Page    *pageData
}

func (p *pageData) EntryPageOf(entries EntryPage, tail bool) entryPageData {
	return entryPageData{Entries: entries, Tail: tail, Page: p}
}

// rowData pairs a log run's row with the rendering page.
type rowData struct {
	StreamRow
	Page *pageData
}

func (p *pageData) RowOf(r StreamRow) rowData { return rowData{StreamRow: r, Page: p} }
