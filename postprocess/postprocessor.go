package postprocess

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/juju/deputy"
	"github.com/negz/grabby/grabber"
)

// TODO(negz): Read this stuff from config. :)
const par2bin string = "/usr/local/bin/par2repair"
const unrarbin string = "/usr/local/bin/unrar"

type PostProcessorer interface {
	Assemble(files []grabber.Filer) error
	Repair() error
	Extract() error
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
	if f.Filename() != "" {
		return os.Create(filepath.Join(wd, f.Filename()))
	}
	if f.IsPar2() {
		// par2cmdline requires that par2 files have a valid extension.
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

func (pp *PostProcessor) Repair() error {
	par2files := filepath.Join(pp.wd, "*.par2")
	allfiles := filepath.Join(pp.wd, "*")
	d := deputy.Deputy{
		Errors:    deputy.FromStderr,
		StdoutLog: func(b []byte) { log.Printf("Repair: %v", string(b)) },
		Timeout:   time.Minute * 3,
	}
	if err := d.Run(exec.Command(par2bin, "-q", "--", par2files, allfiles)); err != nil {
		return err
	}
	return nil
}

func (pp *PostProcessor) Extract() error {
	rarfiles := filepath.Join(pp.wd, "*.rar")
	d := deputy.Deputy{
		Errors:    deputy.FromStderr,
		StdoutLog: func(b []byte) { log.Printf("Extract: %v", string(b)) },
		Timeout:   time.Minute * 3,
	}
	if err := d.Run(exec.Command(unrarbin, "x", "--", rarfiles)); err != nil {
		return err
	}
	return nil
}
