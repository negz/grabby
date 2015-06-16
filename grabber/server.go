package grabber

import (
	"sync"
	"time"

	"gopkg.in/tomb.v2"

	"github.com/negz/grabby/nntp"
	"github.com/negz/grabby/util"
)

const rateDecay float64 = 0.5

// An AggregatedGrabResponse wraps an nntp.GrabResponse with the Server
// that handled it.
type AggregatedGrabResponse struct {
	*nntp.GrabResponse
	Server Serverer
}

type Serverer interface {
	// TODO(negz): Merge with nntp.Serverer?
	nntp.Serverer
	Name() string
	Retention() time.Duration
	MustBeInGroup() bool
	DownloadRate() float64
	UpdateRate(bytes int64)
}

// A Server wraps an nntp.Server with grabber level state information.
type Server struct {
	nntp.Serverer
	name       string
	retention  time.Duration
	needsGroup bool
	bytes      int64
	started    time.Time
	rate       float64
	rateMx     sync.Locker
}

type ServerOption func(*Server) error

func Retention(d time.Duration) ServerOption {
	return func(s *Server) error {
		s.retention = d
		return nil
	}
}

func MustBeInGroup() ServerOption {
	return func(s *Server) error {
		s.needsGroup = true
		return nil
	}
}

func NewServer(ns nntp.Serverer, name string, so ...ServerOption) (Serverer, error) {
	s := &Server{
		Serverer:   ns,
		name:       name,
		retention:  time.Duration(0),
		needsGroup: true,
		started:    time.Now(),
		rateMx:     new(sync.Mutex),
	}

	for _, o := range so {
		if err := o(s); err != nil {
			return nil, err
		}
	}

	return s, nil
}

func (s *Server) Name() string {
	return s.name
}

func (s *Server) Retention() time.Duration {
	return s.retention
}

func (s *Server) MustBeInGroup() bool {
	return s.needsGroup
}

func (s *Server) DownloadRate() float64 {
	s.rateMx.Lock()
	defer s.rateMx.Unlock()
	return s.rate
}

func (s *Server) UpdateRate(bytes int64) {
	s.rateMx.Lock()
	defer s.rateMx.Unlock()

	s.bytes += bytes
	s.rate = util.UpdateDownloadRate(rateDecay, s.rate, time.Since(s.started).Seconds(), s.bytes)
}

type Strategizer interface {
	Servers() []Serverer
	DownloadRate() float64
	Connect()
	Grabbed() <-chan *AggregatedGrabResponse
	Alive() bool
	Err() error
	Shutdown(err error) error
}

// A Strategy is a priority-ordered group of servers, providing a single
// aggregate channel of responses from those servers.
type Strategy struct {
	servers []Serverer
	retry   time.Duration
	started time.Time
	bytes   int64
	rate    float64
	rateMx  sync.Locker
	grabRsp chan *AggregatedGrabResponse
	t       *tomb.Tomb
}

type StrategyOption func(*Strategy) error

func ReconnectInterval(r time.Duration) StrategyOption {
	return func(s *Strategy) error {
		s.retry = r
		return nil
	}
}

func NewStrategy(servers []Serverer, so ...StrategyOption) (Strategizer, error) {
	cb := 0
	for _, s := range servers {
		cb += cap(s.Grabbed())
	}

	st := &Strategy{
		servers: servers,
		retry:   30 * time.Second,
		started: time.Now(),
		rateMx:  new(sync.Mutex),
		grabRsp: make(chan *AggregatedGrabResponse, cb),
	}

	for _, o := range so {
		if err := o(st); err != nil {
			return nil, err
		}
	}
	return st, nil
}

func (ss *Strategy) Servers() []Serverer {
	return ss.servers
}

func (ss *Strategy) DownloadRate() float64 {
	ss.rateMx.Lock()
	defer ss.rateMx.Unlock()
	return ss.rate
}

func (ss *Strategy) updateRate(bytes int64) {
	ss.rateMx.Lock()
	defer ss.rateMx.Unlock()

	ss.bytes += bytes
	ss.rate = util.UpdateDownloadRate(rateDecay, ss.rate, time.Since(ss.started).Seconds(), ss.bytes)
}

func (ss *Strategy) aggregateResponses(s Serverer) {
	ss.t.Go(func() error {
		for {
			select {
			case rsp := <-s.Grabbed():
				s.UpdateRate(rsp.Bytes)
				ss.updateRate(rsp.Bytes)
				ss.grabRsp <- &AggregatedGrabResponse{rsp, s}
			case <-ss.t.Dying():
				return nil
			}
		}
	})
}

func (ss *Strategy) reconnectIfDisconnected(s Serverer) {
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
	if ss.Alive() {
		return
	}

	ss.t = new(tomb.Tomb)

	for _, s := range ss.servers {
		// TODO(negz): Log errors.
		s.HandleGrabs()
		ss.aggregateResponses(s)
		ss.reconnectIfDisconnected(s)
	}
}

func (ss *Strategy) Grabbed() <-chan *AggregatedGrabResponse {
	return ss.grabRsp
}

func (ss *Strategy) Alive() bool {
	return ss.t != nil && ss.t.Alive()
}

func (ss *Strategy) Err() error {
	if ss.t == nil || ss.t.Err() == tomb.ErrStillAlive {
		return nil
	}
	return ss.t.Err()
}

// Shutdown disconnects all servers in this server strategy.
func (ss *Strategy) Shutdown(err error) error {
	for _, s := range ss.servers {
		// TODO(negz): Log errors.
		s.Shutdown(err)
	}
	ss.t.Kill(err)
	return ss.t.Wait()
}
