package postprocess

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/negz/grabby/grabber"
)

type PostProcessorer interface {
	Assemble(files []grabber.Filer) error
	//Repair() error
	//Extract() error
}

type PostProcessor struct {
	wd        string
	assembled map[grabber.Filer]bool
	repaired  bool
	extracted bool
}

func New(workdir string) PostProcessorer {
	return &PostProcessor{
		wd:        workdir,
		assembled: make(map[grabber.Filer]bool),
	}
}

func assembleTo(wd string, f grabber.Filer) (io.WriteCloser, error) {
	if f.IsPar2() || SmellsLikePar2(wd, f) {
		return os.Create(filepath.Join(wd, fmt.Sprintf("%v.par2", f.Hash())))
	}
	return os.Create(filepath.Join(wd, f.Hash()))
}

func appendSegment(wd string, s grabber.Segmenter, af io.Writer) error {
	sp := filepath.Join(wd, s.WorkingFilename())
	sf, err := os.Open(sp)
	if err != nil {
		return err
	}
	io.Copy(af, sf)
	sf.Close()
	os.Remove(sp)
	return nil
}

func (pp *PostProcessor) Assemble(files []grabber.Filer) error {
	for _, f := range files {
		if pp.assembled[f] {
			continue
		}
		if f.State() != grabber.Done {
			continue
		}

		f.SortSegments()
		af, err := assembleTo(pp.wd, f)
		if err != nil {
			return err
		}
		for _, s := range f.Segments() {
			if err := appendSegment(pp.wd, s, af); err != nil {
				return err
			}
		}

		af.Close()
	}
	return nil
}
