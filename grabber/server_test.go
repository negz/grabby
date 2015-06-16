package grabber

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"gopkg.in/tomb.v2"

	"github.com/negz/grabby/nntp"
	"github.com/negz/grabby/util"
)

const Day time.Duration = time.Hour * 24

type fakeNNTPServer struct {
	addr     string
	err      error
	sessions int
	greq     chan *nntp.GrabRequest
	grsp     chan *nntp.GrabResponse
	t        *tomb.Tomb
}

func (fs *fakeNNTPServer) Address() string {
	return fs.addr
}

func (fs *fakeNNTPServer) TLS() bool {
	return false
}

func (fs *fakeNNTPServer) Username() string {
	return "dick.t.butt"
}

func (fs *fakeNNTPServer) HandleGrabs() error {
	if fs.Alive() {
		return nil
	}

	fs.t = new(tomb.Tomb)
	for i := 0; i < fs.sessions; i++ {
		fs.t.Go(func() error {
			for {
				select {
				case <-fs.t.Dying():
					return nil
				case req := <-fs.greq:
					var err error
					switch rand.Int() % 60 {
					case 1:
						err = nntp.NoSuchArticleError
					case 2:
						err = nntp.NoSuchGroupError
					case 3:
						err = fmt.Errorf("I'm an unhandled error!")
					}
					fs.grsp <- &nntp.GrabResponse{
						req,
						rand.Int63n(100) + 680,
						err,
					}
				}
			}
		})
	}
	return nil
}

func (fs *fakeNNTPServer) Grab(g *nntp.GrabRequest) {
	fs.greq <- g
}

func (fs *fakeNNTPServer) Grabbed() <-chan *nntp.GrabResponse {
	return fs.grsp
}

func (fs *fakeNNTPServer) Alive() bool {
	return fs.t != nil && fs.t.Alive()
}

func (fs *fakeNNTPServer) Err() error {
	return fs.t.Err()
}

func (fs *fakeNNTPServer) Shutdown(err error) error {
	fs.t.Kill(err)
	return fs.t.Wait()
}

func newFakeNNTPServer(a string, s int) nntp.Serverer {
	return &fakeNNTPServer{
		addr:     a,
		sessions: s,
		greq:     make(chan *nntp.GrabRequest, s),
		grsp:     make(chan *nntp.GrabResponse, s),
	}
}

var serverTests = []struct {
	servers []nntp.Serverer
	options [][]ServerOption
}{
	{
		[]nntp.Serverer{newFakeNNTPServer("nntp1.fake:119", 5)},
		[][]ServerOption{
			[]ServerOption{Retention(1000 * Day), MustBeInGroup()},
		},
	},
	{
		[]nntp.Serverer{newFakeNNTPServer("nntp1.fake:119", 30)},
		[][]ServerOption{
			[]ServerOption{Retention(10 * Day)},
		},
	},
	{
		[]nntp.Serverer{
			newFakeNNTPServer("nntp1.fake:119", 20),
			newFakeNNTPServer("nntp2.fake:119", 10),
			newFakeNNTPServer("nntp2.fake:119", 5),
		},
		[][]ServerOption{
			[]ServerOption{Retention(3 * Day)},
			[]ServerOption{MustBeInGroup()},
			[]ServerOption{Retention(1000 * Day)},
		},
	},
}

func TestServer(t *testing.T) {
	t.Parallel()
	for _, tt := range serverTests {
		servers := make([]Serverer, 0, len(tt.servers))
		for i, ns := range tt.servers {
			s, err := NewServer(ns, ns.Address(), tt.options[i]...)
			if err != nil {
				t.Fatalf("NewServer(%#v, %#v, %#v): %v", ns, ns.Address(), tt.options[i], err)
			}
			servers = append(servers, s)
		}
		ss, err := NewStrategy(servers)
		if err != nil {
			t.Fatalf("NewStrategy(%+v): %v", servers, err)
		}
		ss.Connect()

		for i := 0; i < 1000; i++ {
			s := ss.Servers()[rand.Intn(len(ss.Servers()))]
			id := util.HashBytes([]byte{byte(rand.Int())})
			s.Grab(&nntp.GrabRequest{"alt.dick.butts", id, new(bytes.Buffer)})
			rsp := <-ss.Grabbed()
			if rsp.ID != id {
				t.Errorf("ss.Grabbed() rsp.ID == %v, want %v", rsp.ID, id)
			}
		}

		ss.Shutdown(nil)
		if ss.DownloadRate() < 0 {
			t.Errorf("ss.DownloadRate == %v, want < 0", ss.DownloadRate())
		}
		for _, s := range ss.Servers() {
			if s.DownloadRate() < 0 {
				t.Errorf("s.DownloadRate == %v, want < 0", s.DownloadRate())
			}
		}
		ss.Connect()
		shutdownErr := errors.New("hi!")
		if err = ss.Shutdown(shutdownErr); err != shutdownErr {
			t.Errorf("ss.Shutdown(%v): %v, want %v", shutdownErr, err, shutdownErr)
		}
		if ss.Err() != shutdownErr {
			t.Errorf("ss.Err(): %v, want %v", ss.Err(), shutdownErr)
		}
	}
}
