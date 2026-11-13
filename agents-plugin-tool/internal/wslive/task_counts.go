package wslive

// TaskCountsCapability gates optional status count fields separately from
// status: a daemon may accept usage status but predate task counts.
const TaskCountsCapability = "task_counts"

func taskCount(n *int) *int {
	if n == nil || *n < 0 {
		return nil
	}
	v := *n
	return &v
}
