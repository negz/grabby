package grabber

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/negz/grabby/nntp"

	upstreamNNTP "github.com/willglynn/nntp"
)

// fc satisfies nntp.conner, but doesn't do much else.
type fc struct {
	// TODO(negz): Dedupe this with the nntp_test implementation.
}

func (c *fc) Authenticate(u, p string) error {
	return nil
}

func (c *fc) EnableCompression() error {
	return nil
}

func (c *fc) Group(g string) (*upstreamNNTP.Group, error) {
	return nil, nil
}

func (c *fc) Body(id string) (io.Reader, error) {
	return strings.NewReader(id), nil
}

func (c *fc) Quit() error {
	return nil
}

func fakeDial(s *nntp.Server) (*nntp.Session, error) {
	return nntp.NewSession(&fc{}), nil
}

func fakeServer(host string, ms int) *Server {
	nntps, _ := nntp.NewServer(host, 119, ms, nntp.SessionDialer(fakeDial))
	return &Server{Server: nntps, Name: fmt.Sprint(nntps), rateMx: new(sync.Mutex)}
}

var serverTests = []struct {
	servers   []*Server
	bytes     []int64
	durations []time.Duration
	rate      float64
}{
	{
		[]*Server{
			fakeServer("prio1.nntp.fake", 3),
			fakeServer("prio2.nntp.fake", 2),
			fakeServer("prio3.nntp.fake", 1),
		},
		[]int64{700, 750, 700},
		[]time.Duration{time.Second * 10, time.Second * 8, time.Second * 7},
		90.9375,
	},
	{
		[]*Server{
			fakeServer("prio1.nntp.fake", 10),
			fakeServer("prio2.nntp.fake", 10),
			fakeServer("prio3.nntp.fake", 5),
		},
		[]int64{1000, 1000, 1000},
		[]time.Duration{time.Second * 10, time.Second * 10, time.Second * 10},
		100.0,
	},
	{
		[]*Server{
			fakeServer("prio1.nntp.fake", 1),
		},
		[]int64{1000},
		[]time.Duration{time.Second * 200},
		5.0,
	},
}

func TestServer(t *testing.T) {
	for _, tt := range serverTests {
		ss, err := NewStrategy(tt.servers)
		if err != nil {
			t.Errorf("NewStrategy(%+v): %v", tt.servers, err)
		}
		ss.Connect()

		for i, s := range ss.Servers {
			sentRsp := &nntp.ArticleResponse{Bytes: tt.bytes[i], Duration: tt.durations[i]}
			s.ArticleRsp <- sentRsp
			recvRsp := <-ss.ArticleRsp
			if recvRsp != sentRsp {
				t.Errorf("<-%v.ArticleRsp == %+v, want %+v ", ss, recvRsp, sentRsp)
			}
		}

		if ss.rate != tt.rate {
			t.Errorf("ss.rate == %f, want %f", ss.rate, tt.rate)
		}

		ss.Shutdown(nil)
	}
}
