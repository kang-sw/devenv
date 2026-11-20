package wsdashboard

// PlanUsageActivity implements the daemon-host capability used by user-started
// agy lead tool calls. Both proxy and hosted-turn agy activity share the same
// asynchronous refresh admission path.
func (d *Daemon) PlanUsageActivity() {
	d.AgyActivity()
}
