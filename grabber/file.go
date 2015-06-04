package grabber

import (
	"strings"
	"sync"

	"github.com/negz/grabby/nzb"
)

type fileState int

const (
	filePending fileState = iota
	filePaused
	fileWorking
	fileDone
)

type File struct {
	*nzb.File
	g        *Grabber
	state    fileState
	stateMx  sync.Locker
	IsPar2   bool // TODO(negz): May need to store the number of blocks.
	Segments []*Segment
}

func NewFile(nf *nzb.File, g *Grabber) *File {
	return &File{
		File:     nf,
		g:        g,
		stateMx:  new(sync.Mutex),
		IsPar2:   strings.Contains(nf.Subject, ".par2"),
		Segments: make([]*Segment, 0, len(nf.Segments)),
	}
}

func (f *File) Working() error {
	f.stateMx.Lock()
	defer f.stateMx.Unlock()

	switch f.state {
	case fileWorking:
		return nil
	case filePending:
		f.state = fileWorking
		return nil
	default:
		return StateError
	}
}

func (f *File) Pause() error {
	f.stateMx.Lock()
	defer f.stateMx.Unlock()

	switch f.state {
	case filePaused:
		return nil
	case filePending:
		f.state = filePaused
		for _, s := range f.Segments {
			if err := s.Pause(); err != nil {
				return err
			}
		}
		return nil
	default:
		return StateError
	}
}

func (f *File) Resume() error {
	f.stateMx.Lock()
	defer f.stateMx.Unlock()

	switch f.state {
	case filePaused:
		for _, s := range f.Segments {
			if err := s.Resume(); err != nil {
				return err
			}
		}
		return nil
	default:
		return StateError
	}
}

func (f *File) Done() error {
	f.stateMx.Lock()
	defer f.stateMx.Unlock()

	f.state = fileDone
	return nil
}
