package wsdashboard

// PlanUsageActivity implements the daemon-host capability used by
// user-started lead tool calls. Both proxy and hosted-turn activity share the
// same asynchronous agy refresh admission path.
func (d *Daemon) PlanUsageActivity() {
	d.AgyActivity()
}
