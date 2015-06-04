package grabber

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"gopkg.in/tomb.v2"

	"github.com/negz/grabby/decode"
	"github.com/negz/grabby/decode/yenc"
	"github.com/negz/grabby/nntp"
	"github.com/negz/grabby/nzb"
	"github.com/negz/grabby/util"
)

// A StateError records an invalid segment, file, or grabber state transition.
var StateError = fmt.Errorf("invalid state transition")

type Metadata struct {
	*nzb.Metadata
	g *Grabber
}

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

func NewFile(nf *nzb.File, g *Grabber) *File {
	return &File{
		File:     nf,
		g:        g,
		stateMx:  new(sync.Mutex),
		IsPar2:   strings.Contains(nf.Subject, ".par2"),
		Segments: make([]*Segment, 0, len(nf.Segments)),
	}
}

type segmentState int

const (
	segPending segmentState = iota
	segPausing
	segPaused
	segWorking
	segFailed
	segGrabbed
)

type Segment struct {
	*nzb.Segment
	f            *File
	decodeOut    io.WriteCloser
	state        segmentState
	stateMx      sync.Locker
	failedServer map[*Server]bool
	failedGroup  map[string]bool
	retries      int
}

func NewSegment(ns *nzb.Segment, f *File) *Segment {
	return &Segment{
		Segment:      ns,
		f:            f,
		stateMx:      new(sync.Mutex),
		failedServer: make(map[*Server]bool),
		failedGroup:  make(map[string]bool),
	}
}

type SegFileCreator func(*Segment) (io.WriteCloser, error)

func createSegmentFile(s *Segment) (io.WriteCloser, error) {
	return os.Create(filepath.Join(s.f.g.wd, fmt.Sprintf("%v.%08d", s.f.g.Hash(), s.Number)))
}

func (s *Segment) Working() error {
	s.stateMx.Lock()
	defer s.stateMx.Unlock()

	switch s.state {
	case segWorking:
		return nil
	case segPending:
		s.state = segWorking
		s.f.Working()
		return nil
	case segPausing:
		s.state = segPaused
		return StateError
	default:
		return StateError
	}
}

func (s *Segment) Pause() error {
	s.stateMx.Lock()
	defer s.stateMx.Unlock()

	switch s.state {
	case segPaused:
		return nil
	case segPending:
		s.state = segPaused
		return nil
	case segWorking:
		s.state = segPausing
		return nil
	default:
		return StateError
	}
}

func (s *Segment) Resume() error {
	s.stateMx.Lock()
	defer s.stateMx.Unlock()

	switch s.state {
	case segPaused:
		s.state = segPending
		s.f.g.enqueueT.Go(s.enqueue)
		return nil
	default:
		return StateError
	}
}

func (s *Segment) Failed() error {
	s.stateMx.Lock()
	defer s.stateMx.Unlock()

	s.state = segFailed
	return nil
}

func (s *Segment) Grabbed() error {
	s.stateMx.Lock()
	defer s.stateMx.Unlock()

	s.state = segGrabbed
	return nil
}

func (s *Segment) selectGroup() string {
	for _, g := range s.f.Groups {
		if s.failedGroup[g] {
			continue
		}
		return g
	}
	return ""
}

func (s *Segment) selectServer() *Server {
	for _, srv := range s.f.g.Strategy.Servers {
		if s.failedServer[srv] {
			continue
		}
		return srv
	}
	return nil
}

func (s *Segment) failCurrentServer() bool {
	srv := s.selectServer()
	if srv == nil {
		return false
	}
	s.failedServer[srv] = true
	return true
}

func (s *Segment) enqueue() error {
	select {
	case <-s.f.g.enqueueT.Dying():
		return nil
	default:
		if err := s.Working(); err != nil {
			return nil
		}

		// Create or truncate the decoded output.
		var err error
		if s.decodeOut, err = s.f.g.fc(s); err != nil {
			// TODO(negz): Log error.
			// TODO(negz): Pause grabber instead of failing?
			s.Failed()
			return nil
		}

		// Select the first untried server.
		srv := s.selectServer()
		if srv == nil {
			// Download has failed on all servers.
			s.Failed()
			return nil
		}

		// Ignore servers that have been disconnected.
		// TODO(negz): Don't treat this temporary failure as permanent?
		if !srv.Working() {
			s.failedServer[srv] = true
			s.failedGroup = make(map[string]bool)
			return s.enqueue()
		}

		if srv.Retention > 0 {
			if time.Since(time.Unix(s.f.Date, 0)) > srv.Retention {
				// Download is out of this server's retention.
				s.failedServer[srv] = true
				s.failedGroup = make(map[string]bool)
				return s.enqueue()
			}
		}

		// No GROUP needed, just request the article ID.
		if !srv.MustBeInGroup {
			srv.ArticleReq <- &nntp.ArticleRequest{
				ID:      s.ArticleID,
				WriteTo: s.f.g.decoder(s.decodeOut),
			}
			return nil
		}

		// Select the first untried group.
		g := s.selectGroup()
		if g == "" {
			// Download has failed on this server.
			s.failedServer[srv] = true
			s.failedGroup = make(map[string]bool)
			return s.enqueue()
		}

		// Request the article ID from the first non-failed group.
		srv.ArticleReq <- &nntp.ArticleRequest{
			Group:   g,
			ID:      s.ArticleID,
			WriteTo: s.f.g.decoder(s.decodeOut),
		}
		return nil
	}
}

type grabberState int

const (
	grabPending grabberState = iota
	grabPaused
	grabWorking
	grabDone
)

type Grabber struct {
	Name     string
	wd       string
	Metadata []*Metadata
	Files    []*File
	Strategy *Strategy
	state    grabberState
	stateMx  sync.Locker
	maxRetry int
	hasher   func(string) string
	decoder  func(io.Writer) io.Writer
	fc       SegFileCreator
	grabT    *tomb.Tomb
	enqueueT *tomb.Tomb
}

func fileFromNZBFile(nf *nzb.File, g *Grabber, filter ...regexp.Regexp) *File {
	f := NewFile(nf, g)
	for _, ns := range nf.Segments {
		f.Segments = append(f.Segments, NewSegment(ns, f))
	}
	for _, r := range filter {
		if r.MatchString(nf.Subject) {
			f.Pause()
		}
	}
	return f
}

type GrabberOption func(*Grabber) error

func FromNZB(n *nzb.NZB, filter ...regexp.Regexp) GrabberOption {
	return func(g *Grabber) error {
		if n.Filename != "" {
			g.Name = strings.TrimSuffix(n.Filename, ".nzb")
		}

		g.Metadata = make([]*Metadata, 0, len(n.Metadata))
		for _, m := range n.Metadata {
			g.Metadata = append(g.Metadata, &Metadata{Metadata: m})
		}

		g.Files = make([]*File, 0, len(n.Files))
		for _, f := range n.Files {
			g.Files = append(g.Files, fileFromNZBFile(f, g, filter...))
		}
		return nil
	}
}

func Name(n string) GrabberOption {
	return func(g *Grabber) error {
		g.Name = n
		return nil
	}
}

func Hasher(h func(string) string) GrabberOption {
	return func(g *Grabber) error {
		g.hasher = h
		return nil
	}
}

func Decoder(d func(io.Writer) io.Writer) GrabberOption {
	return func(g *Grabber) error {
		g.decoder = d
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
		g.fc = s
		return nil
	}
}

func New(workDir string, ServerStrategy *Strategy, gro ...GrabberOption) (*Grabber, error) {
	if workDir == "" {
		return nil, fmt.Errorf("you must specify a workdir")
	}
	g := &Grabber{
		wd:       workDir,
		Strategy: ServerStrategy,
		stateMx:  new(sync.Mutex),
		maxRetry: 3,
		hasher:   util.HashString,
		decoder:  yenc.NewDecoder, // TODO(negz): Detect encoding.
		fc:       createSegmentFile,
		grabT:    new(tomb.Tomb),
		enqueueT: new(tomb.Tomb),
	}
	for _, o := range gro {
		if err := o(g); err != nil {
			return nil, err
		}
	}
	if g.Name == "" {
		return nil, fmt.Errorf("at least one GrabberOption must set new Grabber's Name")
	}
	return g, nil
}

func (g *Grabber) Hash() string {
	return g.hasher(g.Name)
}

func (g *Grabber) Working() error {
	g.stateMx.Lock()
	defer g.stateMx.Unlock()

	switch g.state {
	case grabWorking:
		return nil
	case grabPending:
		g.state = grabWorking
		return nil
	default:
		return StateError
	}
}

func (g *Grabber) Pause() error {
	g.stateMx.Lock()
	defer g.stateMx.Unlock()

	switch g.state {
	case grabPaused:
		return nil
	case grabPending:
		g.state = grabPaused
		for _, f := range g.Files {
			if err := f.Pause(); err != nil {
				return err
			}
		}
		return nil
	default:
		return StateError
	}

}

func (g *Grabber) Resume() error {
	g.stateMx.Lock()
	defer g.stateMx.Unlock()

	switch g.state {
	case grabPaused:
		for _, f := range g.Files {
			if err := f.Resume(); err != nil {
				return err
			}
		}
		return nil
	default:
		return StateError
	}

}

func (g *Grabber) Done() error {
	g.stateMx.Lock()
	defer g.stateMx.Unlock()

	g.state = grabDone
	return nil
}

func (g *Grabber) handleResponses() {
	// Handle article responses.
	g.grabT.Go(func() error {
		// Lookup article ID -> Segment
		segment := make(map[string]*Segment)
		for _, f := range g.Files {
			for _, s := range f.Segments {
				segment[s.ArticleID] = s
			}
		}
		for {
			select {
			case rsp := <-g.Strategy.ArticleRsp:
				s := segment[rsp.ID]
				if rsp.Error != nil {
					// TODO(negz): Log error.
					switch {
					case nntp.IsNoSuchGroupError(rsp.Error):
						s.failedGroup[rsp.Group] = true
					case decode.IsDecodeError(rsp.Error):
						if !s.failCurrentServer() {
							s.Failed()
							continue
						}
					case nntp.IsNoSuchArticleError(rsp.Error):
						if !s.failCurrentServer() {
							s.Failed()
							continue
						}
					default:
						if s.retries <= g.maxRetry {
							s.retries++
						} else {
							if !s.failCurrentServer() {
								s.Failed()
								continue
							}
						}
					}
					// TODO(negz): Tomb doesn't make sense here.
					// enqueue errors should not kill the tomb, and enqueue
					// does not listen for dying tombs.
					// Remove enqueue error and have it select for dying tombs?
					g.enqueueT.Go(s.enqueue)
					continue
				}
				// TODO(negz): Fire on a channel when all unpaused/unfiltered
				// files are grabbed.
				s.Grabbed()
			case <-g.grabT.Dying():
				return nil
			}
		}
	})
}
func (g *Grabber) initialEnqueue() {
	// Initial enqueue of all segments.
	g.enqueueT.Go(func() error {
		for _, f := range g.Files {
			for _, s := range f.Segments {
				select {
				case <-g.enqueueT.Dying():
					return nil
				default:
					s.enqueue()
				}
			}
		}
		return nil
	})
}

func (g *Grabber) Grab() {
	g.Strategy.Connect()
	g.handleResponses()
	g.initialEnqueue()
}

func (g *Grabber) Shutdown(err error) error {
	// 1. Stop sending requests (enqueueT.Kill)
	// 2. Stop processing requests and sending responses (g.Strategy.Shutdown)
	// 3. Stop processing responses (g.grabT.Kill)
	g.enqueueT.Kill(err)
	g.grabT.Kill(g.Strategy.Shutdown(g.enqueueT.Wait()))
	return g.grabT.Wait()
}
