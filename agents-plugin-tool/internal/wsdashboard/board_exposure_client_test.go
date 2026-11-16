package wsdashboard

import "testing"

// The same offline contract runs directly under node --test on Windows; it
// never starts a daemon or a Go-built fixture. Node-less remote runs skip it.
func TestBoardExposureNoHiddenLayoutUnderNode(t *testing.T) {
	runNode(t, "hidden-safe board exposure", `require("./testdata/board-exposure.test.cjs")();`)
}
