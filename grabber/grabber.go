package grabber

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"

	"gopkg.in/tomb.v2"

	"github.com/negz/grabby/decode"
	"github.com/negz/grabby/decode/yenc"
	"github.com/negz/grabby/nntp"
	"github.com/negz/grabby/nzb"
	"github.com/negz/grabby/util"
)

// A StateError records an invalid segment, file, or grabber state transition.
var StateError = fmt.Errorf("invalid state transition")

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
