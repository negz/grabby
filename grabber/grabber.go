package grabber

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/tomb.v2"

	"github.com/negz/grabby/decode"
	"github.com/negz/grabby/decode/yenc"
	"github.com/negz/grabby/magic"
	"github.com/negz/grabby/nntp"
	"github.com/negz/grabby/nzb"
	"github.com/negz/grabby/util"
)

var (
	MissingNameError    = errors.New("at least one GrabberOption must specify a name")
	MissingWorkDirError = errors.New("you must specify a workdir")
	UnknownFileError    = errors.New("asked to grab an unknown file")
)

type BySegments []Filer

func (b BySegments) Len() int           { return len(b) }
func (b BySegments) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b BySegments) Less(i, j int) bool { return len(b[i].Segments()) < len(b[j].Segments()) }

type SegFileCreator func(g Grabberer, s Segmenter) (io.WriteCloser, error)

func createSegmentFile(g Grabberer, s Segmenter) (io.WriteCloser, error) {
	return os.Create(filepath.Join(g.WorkDir(), s.WorkingFilename()))
}

type Grabberer interface {
	FSM
	Name() string
	Hash() string
	WorkDir() string
	Strategy() Strategizer
	Metadata() []Metadataer
	Files() []Filer
	Par2Files() []Filer
	FileIsPar2(f Filer) error
	FileDone(f Filer) error
	FileRequired()
	PostProcessable() <-chan bool
	HandleGrabs()
	GrabAll() error
	GrabFile(f Filer) error
	GrabbedFiles() []Filer
	Shutdown(error) error
	Health() float64
}

type Grabber struct {
	name             string
	hash             string
	wd               string
	meta             []Metadataer
	files            []Filer
	par2Files        []Filer
	doneFiles        []Filer
	doneMx           sync.Locker
	s                Strategizer
	state            State
	writeState       sync.Locker
	readState        sync.Locker
	qIn              chan Segmenter
	qOut             chan Segmenter
	err              error
	required         int
	pp               chan bool
	maxRetry         int
	decoder          func(io.Writer) io.Writer
	sniffer          func(io.Writer, magic.FileTyper) io.Writer
	fileCreator      SegFileCreator
	grabT            *tomb.Tomb
	enqueueT         *tomb.Tomb
	segments         float64
	failedSegments   float64
	failedSegmentsMx sync.Locker
}

type GrabberOption func(*Grabber) error

func FromNZB(n *nzb.NZB, filter ...*regexp.Regexp) GrabberOption {
	return func(g *Grabber) error {
		if n.Filename != "" {
			g.name = strings.TrimSuffix(n.Filename, ".nzb")
		}

		g.meta = make([]Metadataer, 0, len(n.Metadata))
		for _, m := range n.Metadata {
			g.meta = append(g.meta, NewMetadata(m, g))
		}

		g.files = make([]Filer, len(n.Files))
		for i, nf := range n.Files {
			f := NewFile(nf, g, filter...)
			g.files[i] = f
			if f.IsPar2() {
				g.par2Files = append(g.par2Files, f)
			}
			g.segments += float64(len(f.Segments()))
		}
		sort.Sort(BySegments(g.par2Files))
		if len(g.par2Files) > 0 {
			g.par2Files[0].Resume()
		}

		return nil
	}
}

func Name(n string) GrabberOption {
	return func(g *Grabber) error {
		g.name = n
		return nil
	}
}

func Decoder(d func(io.Writer) io.Writer) GrabberOption {
	return func(g *Grabber) error {
		g.decoder = d
		return nil
	}
}

func Sniffer(s func(io.Writer, magic.FileTyper) io.Writer) GrabberOption {
	return func(g *Grabber) error {
		g.sniffer = s
		return nil
	}
}

func RetryOnError(r int) GrabberOption {
	return func(g *Grabber) error {
		g.maxRetry = r
		return nil
	}
}

func SegmentFileCreator(s SegFileCreator) GrabberOption {
	return func(g *Grabber) error {
		g.fileCreator = s
		return nil
	}
}

func New(wd string, ss Strategizer, gro ...GrabberOption) (*Grabber, error) {
	if wd == "" {
		return nil, MissingWorkDirError
	}
	mx := new(sync.RWMutex)
	g := &Grabber{
		wd:               wd,
		par2Files:        make([]Filer, 0),
		doneFiles:        make([]Filer, 0),
		s:                ss,
		writeState:       mx,
		readState:        mx.RLocker(),
		qIn:              make(chan Segmenter, 100), // TODO(negz): Determine best buffer len.
		qOut:             make(chan Segmenter, 100),
		maxRetry:         3,
		doneMx:           new(sync.Mutex),
		pp:               make(chan bool),
		decoder:          yenc.NewDecoder, // TODO(negz): Detect encoding.
		sniffer:          magic.NewSniffer,
		fileCreator:      createSegmentFile,
		grabT:            new(tomb.Tomb),
		enqueueT:         new(tomb.Tomb),
		failedSegmentsMx: new(sync.Mutex),
	}
	for _, o := range gro {
		if err := o(g); err != nil {
			return nil, err
		}
	}
	if g.name == "" {
		return nil, MissingNameError
	}
	g.hash = util.HashString(g.name)
	return g, nil
}

func (g *Grabber) Working() error {
	g.readState.Lock()
	switch g.state {
	case Working:
		g.readState.Unlock()
		return nil
	case Pending:
		g.readState.Unlock()
	default:
		g.readState.Unlock()
		return StateError
	}

	g.writeState.Lock()
	g.state = Working
	g.writeState.Unlock()
	return nil
}

func (g *Grabber) Pause() error {
	g.readState.Lock()
	switch g.state {
	case Pending, Working:
		g.readState.Unlock()
	default:
		g.readState.Unlock()
		return nil
	}

	g.writeState.Lock()
	for _, f := range g.files {
		f.Pause()
	}
	g.state = Paused
	g.writeState.Unlock()
	return nil

}

func (g *Grabber) Resume() error {
	g.readState.Lock()
	switch g.state {
	case Paused:
		g.readState.Unlock()
	default:
		g.readState.Unlock()
		return nil
	}

	g.writeState.Lock()
	for _, f := range g.files {
		// par2 and filtered files must be unpaused explicitly.
		if !f.IsRequired() {
			continue
		}
		f.Resume()

		if f.State() != Pending {
			// We don't need to grab done or working files, and we want an
			// explicit unpause for paused files.
			continue
		}

		if err := g.GrabFile(f); err != nil {
			return err
		}
	}
	g.state = Pending
	g.writeState.Unlock()
	return nil
}

func (g *Grabber) Done(err error) error {
	g.writeState.Lock()
	defer g.writeState.Unlock()

	g.state = Done
	g.err = err
	return err
}

func (g *Grabber) Err() error {
	g.readState.Lock()
	defer g.readState.Unlock()

	return g.err
}

func (g *Grabber) State() State {
	g.readState.Lock()
	defer g.readState.Unlock()

	return g.state
}

func (g *Grabber) Name() string {
	return g.name
}

func (g *Grabber) Hash() string {
	return g.hash
}

func (g *Grabber) WorkDir() string {
	return g.wd
}

func (g *Grabber) Strategy() Strategizer {
	return g.s
}

func (g *Grabber) Metadata() []Metadataer {
	return g.meta
}

func (g *Grabber) Files() []Filer {
	return g.files
}

func (g *Grabber) isPostProcessable() bool {
	return len(g.doneFiles) >= g.required
}

func (g *Grabber) signalPostProcessable() {
	g.pp <- true
}

func (g *Grabber) resetGrabbedFiles() {
	g.doneFiles = make([]Filer, 0)
	g.required = 0
}

func (g *Grabber) GrabbedFiles() []Filer {
	return g.doneFiles
}

func (g *Grabber) Par2Files() []Filer {
	return g.par2Files
}

func (g *Grabber) FileIsPar2(f Filer) error {
	if f.Grabber() != g {
		return UnknownFileError
	}
	g.par2Files = append(g.par2Files, f)
	sort.Sort(BySegments(g.par2Files))
	return nil
}

func (g *Grabber) FileDone(f Filer) error {
	if f.Grabber() != g {
		return UnknownFileError
	}

	g.doneMx.Lock()
	defer g.doneMx.Unlock()

	g.doneFiles = append(g.doneFiles, f)

	if g.isPostProcessable() {
		g.signalPostProcessable()
	}
	return nil
}

func (g *Grabber) FileRequired() {
	g.doneMx.Lock()
	defer g.doneMx.Unlock()

	g.required++
}

func (g *Grabber) PostProcessable() <-chan bool {
	return g.pp
}

func (g *Grabber) handleError(s Segmenter, rsp *AggregatedGrabResponse) {
	switch {
	case decode.IsDecodeError(rsp.Error):
		s.FailServer(rsp.Server)
	case rsp.Error == nntp.NoSuchArticleError:
		s.FailServer(rsp.Server)
	case rsp.Error == nntp.NoSuchGroupError:
		s.FailGroup(rsp.Group)
	default:
		if !s.RetryServer(g.maxRetry) {
			s.FailServer(rsp.Server)
		}
	}
}

func (g *Grabber) handleResponses() {
	g.grabT.Go(func() error {
		segment := make(map[string]Segmenter)
		for _, f := range g.files {
			for _, s := range f.Segments() {
				segment[s.ID()] = s
			}
		}
		for {
			select {
			case <-g.grabT.Dying():
				return nil
			case rsp := <-g.s.Grabbed():
				s := segment[rsp.ID]
				if rsp.Error != nil {
					g.handleError(s, rsp)
					s.WritingTo().Close()
					g.enqueue(s)
					continue
				}
				s.Done(nil)
			}
		}
	})
}

func (g *Grabber) dispatch(s Segmenter) {
	// Only enqueue segments that may enter working state.
	if err := s.Working(); err != nil {
		return
	}

	// Create or truncate the decoded output.
	f, err := g.fileCreator(g, s)
	if err != nil {
		// TODO(negz): Pause grabber instead of failing segment?
		s.Done(err)
		g.segFailed()
		return
	}
	s.WriteTo(f)

	// Select the first untried server.
	srv, err := s.SelectServer(g.s.Servers())
	if err != nil {
		s.Done(err)
		g.segFailed()
		return
	}

	// Ignore servers that have been disconnected.
	// TODO(negz): Don't treat this temporary failure as permanent?
	if !srv.Alive() {
		s.FailServer(srv)
		s.WritingTo().Close()
		g.dispatch(s)
		return
	}

	if srv.Retention() > 0 {
		if time.Since(s.File().Posted()) > srv.Retention() {
			// Download is out of this server's retention.
			s.FailServer(srv)
			s.WritingTo().Close()
			g.dispatch(s)
			return
		}
	}

	group := ""
	if srv.MustBeInGroup() {
		group, err = s.SelectGroup(s.File().Groups())
		if err != nil {
			s.FailServer(srv)
			s.WritingTo().Close()
			g.dispatch(s)
			return
		}
	}

	// Request the article ID from the first non-failed group.
	switch s.Number() {
	case 1:
		// The first segment may provide a hint for the file's type.
		srv.Grab(&nntp.GrabRequest{group, s.ID(), g.decoder(g.sniffer(s.WritingTo(), s.File()))})
	default:
		srv.Grab(&nntp.GrabRequest{group, s.ID(), g.decoder(s.WritingTo())})
	}
}

func (g *Grabber) handleEnqueues() {
	g.enqueueT.Go(func() error {
		b := make([]Segmenter, 0)
		for {
			select {
			case <-g.enqueueT.Dying():
				return nil
			case s := <-g.qIn:
				b = append(b, s)
			default:
			}
			if len(b) == 0 {
				continue
			}
			select {
			case <-g.enqueueT.Dying():
				return nil
			case g.qOut <- b[0]:
				b[0], b = nil, b[1:]
			default:
			}
		}
	})
	g.enqueueT.Go(func() error {
		for {
			select {
			case <-g.enqueueT.Dying():
				return nil
			case s := <-g.qOut:
				g.dispatch(s)
			}
		}
	})
}

func (g *Grabber) enqueue(s Segmenter) {
	select {
	case <-g.enqueueT.Dying():
	case g.qIn <- s:
	}
}

func (g *Grabber) HandleGrabs() {
	g.s.Connect()
	g.handleResponses()
	g.handleEnqueues()
}

func (g *Grabber) GrabFile(f Filer) error {
	if f.Grabber() != g {
		return UnknownFileError
	}

	// If this is the first file requested since we last became postprocessable
	// we reset the counter of done files.
	g.doneMx.Lock()
	if g.isPostProcessable() {
		g.resetGrabbedFiles()
	}
	g.doneMx.Unlock()

	switch f.State() {
	case Pending:
	case Paused:
		f.Resume()
	default:
		// File is either done, or already working.
		return nil
	}

	for _, s := range f.Segments() {
		g.enqueue(s)
	}
	return nil
}

func (g *Grabber) GrabAll() error {
	for _, f := range g.files {
		if f.State() != Pending {
			// We don't need to grab done or working files, and we want an
			// explicit unpause for paused files.
			continue
		}
		if err := g.GrabFile(f); err != nil {
			return err
		}
	}
	return nil
}

func (g *Grabber) Shutdown(err error) error {
	// 1. Stop sending requests (enqueueT.Kill)
	// 2. Stop processing requests and sending responses (g.s.Shutdown)
	// 3. Stop processing responses (g.grabT.Kill)
	g.enqueueT.Kill(err)
	g.grabT.Kill(g.s.Shutdown(g.enqueueT.Wait()))
	return g.grabT.Wait()
}

func (g *Grabber) segFailed() {
	g.failedSegmentsMx.Lock()
	g.failedSegments++
	g.failedSegmentsMx.Unlock()
}
func (g *Grabber) Health() float64 {
	g.failedSegmentsMx.Lock()
	defer g.failedSegmentsMx.Unlock()
	return 100.0 - ((g.failedSegments / g.segments) * 100.0)
}
