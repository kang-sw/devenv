package wsdashboard

import "testing"

// Read the assembled stylesheet so split ownership does not hide hover styles.
func TestSessionHoverTranslatedLabelsWrap(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	contains(t, "full translated stat labels", css, `.shp-stat > span { font-size: 10.5px; line-height: 1.25; color: var(--wb-muted-fg); white-space: normal; overflow-wrap: anywhere; }`)
	contains(t, "model history spacing", css, `.modelhistory { margin: 8px 12px; font-size: 12px; color: var(--wb-muted-fg); }`)
	contains(t, "model history disclosure", css, `.modelhistory > summary { cursor: pointer; }`)
}
