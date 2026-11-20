package wswhiteboard

// entryData pairs an entry with the rendering page without changing its model.
type entryData struct {
	EntryView
	Page *pageData
}

func (p *pageData) EntryOf(e EntryView) entryData { return entryData{EntryView: e, Page: p} }

func (d entryData) IsPanel() bool { return d.Kind == EntryPanel }

func (d entryData) HistoryItem() itemData {
	return itemData{ItemView: *d.Item, Page: d.Page, Static: true, History: &d.EntryView}
}

func (d entryData) PayloadClass() string {
	return "framed"
}

// entryPageData is one history page to render.
type entryPageData struct {
	Entries EntryPage
	Page    *pageData
}

func (p *pageData) EntryPageOf(entries EntryPage) entryPageData {
	return entryPageData{Entries: entries, Page: p}
}

// rowData pairs a log run's row with the rendering page.
type rowData struct {
	StreamRow
	Page *pageData
}

func (p *pageData) RowOf(r StreamRow) rowData { return rowData{StreamRow: r, Page: p} }
