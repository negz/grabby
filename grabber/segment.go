package grabber

import (
	"errors"
	"io"
	"sync"
	"time"

	"github.com/negz/grabby/nzb"
)

var (
	NoMoreServersError = errors.New("segment failed on all servers")
	NoMoreGroupsError  = errors.New("segment failed on all groups")
)

type Segmenter interface {
	FSM
	ID() string
	Number() int
	Posted() time.Time
	Groups() []string
	WriteTo(w io.WriteCloser)
	WritingTo() io.WriteCloser
	FailGroup(g string)
	FailServer(s Serverer)
	SelectGroup(groups []string) (string, error)
	SelectServer(servers []Serverer) (Serverer, error)
	RetryServer(max int) bool
}

type Segment struct {
	ns           *nzb.Segment
	f            Filer
	w            io.WriteCloser
	state        State
	writeState   sync.Locker
	readState    sync.Locker
	err          error
	failedServer map[Serverer]bool
	failedGroup  map[string]bool
	retries      int
}

func NewSegment(ns *nzb.Segment, f Filer) Segmenter {
	mx := new(sync.RWMutex)
	return &Segment{
		ns:           ns,
		f:            f,
		writeState:   mx,
		readState:    mx.RLocker(),
		failedServer: make(map[Serverer]bool),
		failedGroup:  make(map[string]bool),
	}
}

func (s *Segment) Working() error {
	s.readState.Lock()
	switch s.state {
	case Working:
		s.readState.Unlock()
		return nil
	case Pending, Pausing:
		s.readState.Unlock()
	default:
		s.readState.Unlock()
		return StateError
	}

	s.writeState.Lock()
	if s.state == Pausing {
		s.state = Paused
		s.writeState.Unlock()
		return StateError
	}

	s.state = Working
	s.writeState.Unlock()
	if err := s.f.Working(); err != nil {
		return err
	}
	return nil
}

func (s *Segment) Pause() error {
	s.readState.Lock()
	switch s.state {
	case Pending, Working:
		s.readState.Unlock()
	default:
		s.readState.Unlock()
		return nil
	}

	s.writeState.Lock()
	switch s.state {
	case Pending:
		s.state = Paused
	case Working:
		s.state = Pausing
	}
	s.writeState.Unlock()
	return nil
}

func (s *Segment) Resume() error {
	s.readState.Lock()
	switch s.state {
	case Paused, Pausing:
		s.readState.Unlock()
	default:
		s.readState.Unlock()
		return nil
	}

	s.writeState.Lock()
	s.state = Pending
	s.writeState.Unlock()
	return nil
}

func (s *Segment) Done(err error) error {
	s.readState.Lock()
	// Segments may only transition to done one time to avoid us re-closing a
	// closed file.
	if s.state == Done {
		s.readState.Unlock()
		return nil
	}
	s.readState.Unlock()

	s.writeState.Lock()
	// TODO(negz): Mutex here in case we're going from create -> close really
	// fast
	if s.w != nil {
		s.w.Close()
	}
	s.state = Done
	s.err = err
	s.writeState.Unlock()

	s.f.SegmentDone()
	return err
}

func (s *Segment) Err() error {
	s.readState.Lock()
	defer s.readState.Unlock()

	return s.err
}

func (s *Segment) State() State {
	s.readState.Lock()
	defer s.readState.Unlock()

	return s.state
}

func (s *Segment) ID() string {
	return s.ns.ArticleID
}

func (s *Segment) Number() int {
	return s.ns.Number
}

func (s *Segment) Posted() time.Time {
	return s.f.Posted()
}

func (s *Segment) Groups() []string {
	return s.f.Groups()
}

func (s *Segment) WriteTo(w io.WriteCloser) {
	s.w = w
}

func (s *Segment) WritingTo() io.WriteCloser {
	return s.w
}

func (s *Segment) FailGroup(g string) {
	s.failedGroup[g] = true
}

func (s *Segment) FailServer(srv Serverer) {
	s.failedServer[srv] = true
	s.failedGroup = make(map[string]bool)
	s.retries = 0
}

func (s *Segment) SelectGroup(groups []string) (string, error) {
	for _, g := range groups {
		if s.failedGroup[g] {
			continue
		}
		return g, nil
	}
	return "", NoMoreGroupsError
}

func (s *Segment) SelectServer(servers []Serverer) (Serverer, error) {
	for _, srv := range servers {
		if s.failedServer[srv] {
			continue
		}
		return srv, nil
	}
	return nil, NoMoreServersError
}

func (s *Segment) RetryServer(max int) bool {
	if s.retries >= max {
		return false
	}
	s.retries++
	return true
}
