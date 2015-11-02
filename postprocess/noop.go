package postprocess

// A Noop repairer is used when there are no par2 files to repair with.
type NoopRepairer struct {
}

func (no *NoopRepairer) Repair() error {
	return nil
}

func (no *NoopRepairer) Repaired() bool {
	return true
}

func (no *NoopRepairer) RenamedFiles() map[string]string {
	return make(map[string]string)
}

func (no *NoopRepairer) BlocksNeeded() int {
	return 0
}
