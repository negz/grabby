package grabber

import (
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/negz/grabby/magic"
	"github.com/negz/grabby/nzb"
	"github.com/negz/grabby/util"
)

type ByNumber []Segmenter

func (b ByNumber) Len() int           { return len(b) }
func (b ByNumber) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b ByNumber) Less(i, j int) bool { return b[i].Number() < b[j].Number() }

type Filer interface {
	FSM
	Grabber() Grabberer
	Subject() string
	Hash() string
	Poster() string
	Posted() time.Time
	Groups() []string
	Segments() []Segmenter
	SortSegments()
	IsRequired() bool
	IsPar2() bool
	IsFiltered() bool
	SegmentDone()
	Filename() string
	FileType() magic.FileType
	SetFileType(t magic.FileType)
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
	filename   string
	filetype   magic.FileType
	required   bool
	filtered   bool
}

func NewFile(nf *nzb.File, g Grabberer, filter ...*regexp.Regexp) Filer {
	mx := new(sync.RWMutex)
	f := &File{
		nf:         nf,
		g:          g,
		hash:       util.HashString(nf.Subject),
		segments:   make([]Segmenter, len(nf.Segments)),
		writeState: mx,
		readState:  mx.RLocker(),
		doneMx:     new(sync.Mutex),
		filename:   magic.GetSubjectFilename(nf.Subject),
		filetype:   magic.GetSubjectType(nf.Subject),
		required:   true,
	}

	for i, ns := range nf.Segments {
		f.segments[i] = NewSegment(ns, f)
	}

	if f.filetype == magic.Par2 {
		f.g.FileIsPar2(f)
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
	f.state = Pausing
	f.writeState.Unlock()
	for _, s := range f.segments {
		s.Pause()
	}
	f.writeState.Lock()
	f.state = Paused
	f.writeState.Unlock()
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
	f.state = Resuming
	f.writeState.Unlock()
	for _, s := range f.segments {
		s.Resume()
	}
	f.writeState.Lock()
	f.state = Pending
	if !f.required {
		f.required = true
		f.g.FileRequired()
	}
	f.writeState.Unlock()
	return nil
}

func (f *File) Done(err error) error {
	f.writeState.Lock()
	f.state = Done
	f.err = err
	f.writeState.Unlock()
	f.g.FileDone(f)
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

func (f *File) Grabber() Grabberer {
	return f.g
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

func (f *File) SortSegments() {
	// Segments are almost always already sorted.
	if sort.IsSorted(ByNumber(f.segments)) {
		return
	}
	sort.Sort(ByNumber(f.segments))
}

func (f *File) IsRequired() bool {
	return f.required
}

func (f *File) IsPar2() bool {
	return f.filetype == magic.Par2
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

func (f *File) Filename() string {
	return f.filename
}

func (f *File) FileType() magic.FileType {
	return f.filetype
}

func (f *File) SetFileType(t magic.FileType) {
	if f.filetype == t {
		return
	}
	f.filetype = t

	if f.IsPar2() {
		f.g.FileIsPar2(f)
	}
}
