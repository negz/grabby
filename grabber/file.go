package grabber

import (
	"regexp"
	"sync"
	"time"

	"github.com/negz/grabby/nzb"
	"github.com/negz/grabby/util"
)

var par2RE *regexp.Regexp = regexp.MustCompile(`(?i).+\.par2.*`)

type Filer interface {
	FSM
	Subject() string
	Hash() string
	Poster() string
	Posted() time.Time
	Groups() []string
	Segments() []Segmenter
	IsRequired() bool
	IsPar2() bool
	IsFiltered() bool
	SegmentDone()
}

type File struct {
	nf         *nzb.File
	g          Grabberer
	hash       string
	segments   []Segmenter
	state      State
	writeState sync.Locker
	readState  sync.Locker
	err        error
	done       int
	doneMx     sync.Locker
	required   bool
	par2       bool // TODO(negz): May need to store the number of blocks.
	filtered   bool
}

func NewFile(nf *nzb.File, g Grabberer, filter ...*regexp.Regexp) Filer {
	mx := new(sync.RWMutex)
	f := &File{
		nf:         nf,
		g:          g,
		hash:       util.HashString(nf.Subject),
		segments:   make([]Segmenter, 0, len(nf.Segments)),
		writeState: mx,
		readState:  mx.RLocker(),
		doneMx:     new(sync.Mutex),
		required:   true,
	}

	for _, ns := range nf.Segments {
		f.segments = append(f.segments, NewSegment(ns, f))
	}

	if par2RE.MatchString(nf.Subject) {
		f.par2 = true
		f.required = false
		f.Pause()
	}

	for _, r := range filter {
		if r.MatchString(nf.Subject) {
			f.filtered = true
			f.required = false
			f.Pause()
		}
	}

	if f.required {
		f.g.FileRequired()
	}

	return f
}

func (f *File) Working() error {
	f.readState.Lock()
	switch f.state {
	case Working:
		f.readState.Unlock()
		return nil
	case Pending:
		f.readState.Unlock()
	default:
		f.readState.Unlock()
		return StateError
	}

	f.writeState.Lock()
	f.state = Working
	f.writeState.Unlock()
	if err := f.g.Working(); err != nil {
		return err
	}
	return nil
}

func (f *File) Pause() error {
	f.readState.Lock()
	switch f.state {
	case Pending, Working:
		f.readState.Unlock()
	default:
		f.readState.Unlock()
		return nil
	}

	f.writeState.Lock()
	f.state = Paused
	f.writeState.Unlock()
	for _, s := range f.segments {
		s.Pause()
	}
	return nil
}

func (f *File) Resume() error {
	f.readState.Lock()
	switch f.state {
	case Paused:
		f.readState.Unlock()
	default:
		f.readState.Unlock()
		return nil
	}

	f.writeState.Lock()
	f.state = Pending
	if !f.required {
		f.required = true
		f.g.FileRequired()
	}
	f.writeState.Unlock()
	for _, s := range f.segments {
		s.Resume()
	}
	return nil
}

func (f *File) Done(err error) error {
	f.writeState.Lock()
	f.state = Done
	f.err = err
	f.writeState.Unlock()
	f.g.FileDone()
	return err
}

func (f *File) Err() error {
	f.readState.Lock()
	defer f.readState.Unlock()

	return f.err
}

func (f *File) State() State {
	f.readState.Lock()
	defer f.readState.Unlock()

	return f.state
}

func (f *File) Subject() string {
	return f.nf.Subject
}

func (f *File) Hash() string {
	// TODO(negz): UUID instead of hash - no guarantee of unique subjects.
	return f.hash
}

func (f *File) Poster() string {
	return f.nf.Poster
}

func (f *File) Posted() time.Time {
	return time.Unix(f.nf.Date, 0)
}

func (f *File) Groups() []string {
	return f.nf.Groups
}

func (f *File) Segments() []Segmenter {
	return f.segments
}

func (f *File) IsRequired() bool {
	return f.required
}

func (f *File) IsPar2() bool {
	return f.par2
}

func (f *File) IsFiltered() bool {
	return f.filtered
}

func (f *File) SegmentDone() {
	f.doneMx.Lock()
	defer f.doneMx.Unlock()

	f.done++

	if f.done >= len(f.segments) {
		f.Done(nil)
	}
}
