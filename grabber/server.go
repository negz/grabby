package grabber

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"gopkg.in/tomb.v2"

	"github.com/negz/grabby/nntp"
	"github.com/negz/grabby/util"
)

const rateDecay float64 = 0.5

// A Server wraps an nntp.Server with grabber level state information.
type Server struct {
	// TODO(negz): Merge into nntp.Server?
	*nntp.Server
	Name          string
	Retention     time.Duration
	MustBeInGroup bool
	rate          float64
	rateMx        sync.Locker
}

func (s *Server) updateRate(seconds float64, bytes int64) {
	s.rateMx.Lock()
	defer s.rateMx.Unlock()
	s.rate = util.UpdateDownloadRate(rateDecay, s.rate, seconds, bytes)
}

// A Strategy is a priority-ordered group of servers, providing a single
// aggregate channel of responses from those servers.
type Strategy struct {
	Servers    []*Server
	retry      time.Duration
	rate       float64
	rateMx     sync.Locker
	ArticleRsp chan *nntp.ArticleResponse
	t          *tomb.Tomb
}

func (ss *Strategy) String() string {
	sn := make([]string, 0, len(ss.Servers))
	for _, s := range ss.Servers {
		sn = append(sn, fmt.Sprint(s))
	}
	return fmt.Sprintf("Server strategy: %v", strings.Join(sn, ", "))
}

type StrategyOption func(*Strategy) error

func ReconnectInterval(r time.Duration) StrategyOption {
	return func(s *Strategy) error {
		s.retry = r
		return nil
	}
}

func NewStrategy(servers []*Server, so ...StrategyOption) (*Strategy, error) {
	cb := 0
	for _, s := range servers {
		cb += cap(s.ArticleRsp)
	}

	st := &Strategy{
		Servers:    servers,
		retry:      30 * time.Second,
		rateMx:     new(sync.Mutex),
		ArticleRsp: make(chan *nntp.ArticleResponse, cb),
		t:          new(tomb.Tomb),
	}

	for _, o := range so {
		if err := o(st); err != nil {
			return nil, err
		}
	}
	return st, nil
}

func (ss *Strategy) updateRate(seconds float64, bytes int64) {
	ss.rateMx.Lock()
	defer ss.rateMx.Unlock()
	ss.rate = util.UpdateDownloadRate(rateDecay, ss.rate, seconds, bytes)
}

func (ss *Strategy) aggregateResponses(s *Server) {
	agg := func(rc <-chan *nntp.ArticleResponse) func() error {
		return func() error {
			for {
				select {
				case rsp := <-rc:
					s.updateRate(rsp.Duration.Seconds(), rsp.Bytes)
					ss.updateRate(rsp.Duration.Seconds(), rsp.Bytes)
					ss.ArticleRsp <- rsp
				case <-ss.t.Dying():
					return nil
				}
			}
		}
	}
	ss.t.Go(agg(s.ArticleRsp))
}

func (ss *Strategy) reconnectIfDisconnected(s *Server) {
	ss.t.Go(func() error {
		tick := time.NewTicker(ss.retry)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				// TODO(negz): Log errors.
				s.HandleGrabs()
			case <-ss.t.Dying():
				return nil
			}
		}
	})
}

// Connect connects and aggregates the responses of all servers in this
// server strategy.
func (ss *Strategy) Connect() {
	for _, s := range ss.Servers {
		// TODO(negz): Log errors.
		s.HandleGrabs()
		ss.aggregateResponses(s)
		ss.reconnectIfDisconnected(s)
	}
}

// Shutdown disconnects all servers in this server strategy.
func (ss *Strategy) Shutdown(err error) error {
	for _, s := range ss.Servers {
		// TODO(negz): Log errors.
		s.Shutdown()
	}
	ss.t.Kill(err)
	return ss.t.Wait()
}
