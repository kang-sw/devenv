package wsdashboard

import "testing"

// This regression travels with the deferred frozen-asset label adjustment,
// not the independently mergeable response-telemetry heads.
func TestSessionHoverTranslatedLabelsWrap(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	contains(t, "full translated stat labels", css, `.shp-stat > span { font-size: 10.5px; line-height: 1.25; color: var(--wb-muted-fg); white-space: normal; overflow-wrap: anywhere; }`)
}
