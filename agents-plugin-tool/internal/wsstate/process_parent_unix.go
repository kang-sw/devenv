//go:build !windows

package wsstate

func processParent(pid int) (int, string, bool) { return 0, "", false }
