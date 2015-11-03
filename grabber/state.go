package grabber

import "errors"

// A StateError records an invalid segment, file, or grabber state transition.
var StateError = errors.New("invalid state transition")

type State int

const (
	Pending State = iota
	Pausing
	Paused
	Resuming
	Working
	Done
)

var stateName = map[State]string{
	Pending:  "pending",
	Pausing:  "pausing",
	Paused:   "paused",
	Resuming: "resuming",
	Working:  "working",
	Done:     "done",
}

func (s State) String() string {
	return stateName[s]
}

type FSM interface {
	Working() error
	Pause() error
	Resume() error
	Done(err error) error
	State() State
	Err() error
}
