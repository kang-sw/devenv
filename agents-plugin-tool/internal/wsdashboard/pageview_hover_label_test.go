package wsdashboard

import "testing"

// Read the assembled stylesheet so split ownership does not hide hover styles.
func TestSessionHoverTranslatedLabelsWrap(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	contains(t, "full translated stat labels", css, `.shp-stat > span { font-size: 10.5px; line-height: 1.25; color: var(--wb-muted-fg); white-space: normal; overflow-wrap: anywhere; }`)
	contains(t, "model history trigger", css, `.modelhistory-trigger { width: 24px;`, `.modelhistory-trigger:hover, .modelhistory-trigger[aria-expanded="true"]`)
	contains(t, "model history popover", css, `.modelhistory-popover { width: 360px; }`, `.modelhistory { display: grid; gap: 9px;`)
}
