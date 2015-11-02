package postprocess

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/negz/grabby/grabber"
	"github.com/negz/grabby/magic"
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
	FileType() magic.FileType
	State() State
	Assemble() error
}

type File struct {
	segPaths []string
	filename string
	filepath string
	filetype magic.FileType
	state    State
}

func NewFile(wd string, gf grabber.Filer) Filer {
	f := &File{segPaths: make([]string, len(gf.Segments())), filetype: gf.FileType()}

	gf.SortSegments()
	for i, s := range gf.Segments() {
		f.segPaths[i] = filepath.Join(wd, s.WorkingFilename())
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
	return f.filetype == magic.Par2
}

func (f *File) FileType() magic.FileType {
	return f.filetype
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

	for _, s := range f.segPaths {
		if err := appendSegment(af, s); err != nil {
			return err
		}
	}

	f.state = Assembled
	return nil
}
