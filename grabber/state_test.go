package grabber

type fakeFSM struct {
	s State
	e error
}

func (fsm *fakeFSM) Working() error {
	fsm.s = Working
	return nil
}

func (fsm *fakeFSM) Pause() error {
	fsm.s = Paused
	return nil
}

func (fsm *fakeFSM) Resume() error {
	fsm.s = Pending
	return nil
}

func (fsm *fakeFSM) Done(err error) error {
	fsm.s = Done
	fsm.e = err
	return nil
}

func (fsm *fakeFSM) State() State {
	return fsm.s
}

func (fsm *fakeFSM) Err() error {
	return fsm.e
}
