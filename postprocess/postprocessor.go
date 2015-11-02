package postprocess

import "github.com/negz/grabby/grabber"

// TODO(negz): Read this stuff from config. :)
const par2bin string = "/usr/local/bin/par2repair"
const unrarbin string = "/usr/local/bin/unrar"

type Repairer interface {
	Repair() error
	Repaired() bool
	RenamedFiles() map[string]string
	BlocksNeeded() int
}

type Extracter interface {
	Extract() error
	ExtractedFiles() []string
}

type PostProcessorer interface {
	AddFiles([]grabber.Filer)
	Assemble() error
	Repairer() Repairer
	// Extracter() Extracter
}

type PostProcessor struct {
	wd       string
	files    []Filer
	hasFile  map[grabber.Filer]bool
	par2File Filer
}

func New(workdir string) PostProcessorer {
	return &PostProcessor{
		wd:      workdir,
		files:   make([]Filer, 0),
		hasFile: make(map[grabber.Filer]bool),
	}
}

func (pp *PostProcessor) AddFiles(files []grabber.Filer) {
	for _, gf := range files {
		if pp.hasFile[gf] {
			continue
		}
		f := NewFile(pp.wd, gf)
		pp.files = append(pp.files, f)
		if pp.par2File == nil && f.IsPar2() {
			pp.par2File = f
		}
	}
}

func (pp *PostProcessor) Assemble() error {
	for _, f := range pp.files {
		if err := f.Assemble(); err != nil {
			return err
		}
	}
	return nil
}

// TODO(negz): Handle stuff what isn't par2?
func (pp *PostProcessor) Repairer() Repairer {
	if pp.par2File == nil {
		return &NoopRepairer{}
	}
	return NewPar2Cmdline(par2bin, Par2(pp.par2File), Files(pp.files))
}

/*
func (pp *PostProcessor) Extracter() Extracter {
	return NewUnrar(unrarbin)
}
*/
