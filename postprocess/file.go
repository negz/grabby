package postprocess

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/negz/grabby/grabber"
)

type State int

const (
	Unprocessed State = iota
	Assembled
	Repaired
	Extracted
)

type Filer interface {
	Path() string
	IsPar2() bool
	State() State
	Assemble() error
}

type File struct {
	segments []string
	filename string
	filepath string
	isPar2   bool
	state    State
}

func NewFile(wd string, gf grabber.Filer) Filer {
	f := &File{segments: make([]string, len(gf.Segments())), isPar2: gf.IsPar2()}

	gf.SortSegments()
	for i, s := range gf.Segments() {
		f.segments[i] = filepath.Join(wd, s.WorkingFilename())
	}

	switch {
	case gf.Filename() != "":
		f.filename = gf.Filename()
	case f.IsPar2():
		// par2cmdline requires that par2 files have a valid extension.
		f.filename = fmt.Sprintf("%v.par2", gf.Hash())
	default:
		f.filename = gf.Hash()
	}
	f.filepath = filepath.Join(wd, f.filename)

	return f
}

func (f *File) Path() string {
	return f.filepath
}

func (f *File) IsPar2() bool {
	return f.isPar2
}

func (f *File) State() State {
	return f.state
}

func appendSegment(af io.Writer, path string) error {
	sf, err := os.Open(path)
	if err != nil {
		return err
	}
	io.Copy(af, sf)
	if err := sf.Close(); err != nil {
		return err
	}
	os.Remove(path)
	return nil
}

func (f *File) Assemble() error {
	if f.state != Unprocessed {
		return nil
	}

	af, err := os.Create(f.filepath)
	if err != nil {
		return err
	}
	defer af.Close()

	for _, s := range f.segments {
		if err := appendSegment(af, s); err != nil {
			return err
		}
	}

	f.state = Assembled
	return nil
}
