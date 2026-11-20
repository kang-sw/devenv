//go:build !darwin

package wsdashboard

// LaunchExecutable returns exe unchanged off macOS; Windows and Linux keep
// launching the content-addressed staged executable directly.
func LaunchExecutable(exe, _ string) (string, error) {
	return exe, nil
}
