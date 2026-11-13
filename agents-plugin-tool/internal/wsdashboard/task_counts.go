package wsdashboard

import "github.com/kang-sw/devenv/internal/wslive"

func taskBadgeCounts(s *wslive.Snapshot) (background, monitors int) {
	if s.Usage != nil {
		if s.Usage.Background != nil {
			background = max(0, *s.Usage.Background)
		}
		if s.Usage.Monitors != nil {
			monitors = max(0, *s.Usage.Monitors)
		}
	}
	if h := s.Host; h != nil {
		// Preserve the badge for old hosted snapshots that only knew a total.
		background = max(0, h.BackgroundTasks)
		if h.Background != nil {
			background = max(0, *h.Background)
		}
		if h.Monitors != nil {
			monitors = max(0, *h.Monitors)
		}
	}
	return
}
