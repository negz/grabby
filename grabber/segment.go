package grabber

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/negz/grabby/nntp"
	"github.com/negz/grabby/nzb"
)

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

type SegFileCreator func(*Segment) (io.WriteCloser, error)

func createSegmentFile(s *Segment) (io.WriteCloser, error) {
	return os.Create(filepath.Join(s.f.g.wd, fmt.Sprintf("%v.%08d", s.f.g.Hash(), s.Number)))
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
