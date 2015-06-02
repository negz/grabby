package grabber

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/negz/grabby/decode/yenc"
	"github.com/negz/grabby/nntp"
	"github.com/negz/grabby/nzb"

	upstreamNNTP "github.com/willglynn/nntp"
)

type fakeErroreyConn struct {
}

func (c *fakeErroreyConn) Authenticate(u, p string) error {
	return nil
}

func (c *fakeErroreyConn) EnableCompression() error {
	return nil
}

func (c *fakeErroreyConn) Group(g string) (*upstreamNNTP.Group, error) {
	if rand.Int()%2 == 1 {
		return nil, upstreamNNTP.Error{Code: 411, Msg: "LOL NO GROUP"}
	}
	return nil, nil
}

func (c *fakeErroreyConn) Body(id string) (io.Reader, error) {
	switch rand.Int() % 20 {
	case 1:
		return nil, upstreamNNTP.Error{Code: 430, Msg: "LOL NO ARTICLE"}
	case 2:
		return nil, fmt.Errorf("I'm an unhandled error!")
	}
	time.Sleep(time.Second * time.Duration((rand.Int() % 4)))
	return strings.NewReader(id), nil
}

func (c *fakeErroreyConn) Quit() error {
	return nil
}

func fakeErroreyDial(s *nntp.Server) (*nntp.Session, error) {
	return nntp.NewSession(&fakeErroreyConn{}), nil
}

func fakeErroreyServer(host string, ms int) *Server {
	nntps, _ := nntp.NewServer(host, 119, ms, nntp.SessionDialer(fakeErroreyDial))
	return &Server{Server: nntps, Name: fmt.Sprint(nntps), MustBeInGroup: true, rateMx: new(sync.Mutex)}
}

type fakeFile struct {
	*bytes.Buffer
}

func (ff *fakeFile) Close() error {
	return nil
}

type erroreyDecoder struct {
}

func (ed *erroreyDecoder) Write(b []byte) (int, error) {
	return 3, yenc.DecodeError("What even is this encoding!?")
}

func createFakeErroreyDecoder(w io.Writer) io.Writer {
	if rand.Int()%10 == 1 {
		return &erroreyDecoder{}
	}
	return w
}

func createFakeFile(s *Segment) (io.WriteCloser, error) {
	return &fakeFile{Buffer: new(bytes.Buffer)}, nil
}

var grabberTests = []struct {
	s        []*Server
	f        string
	metadata int
	files    int
	name     string
}{
	{
		s: []*Server{
			fakeErroreyServer("nntp.fake1", 1),
		},
		f:        "testdata/ubuntu-14.04.2-desktop-amd64.nzb",
		metadata: 2,
		files:    15,
		name:     "ubuntu-14.04.2-desktop-amd64",
	},
	{
		s: []*Server{
			fakeErroreyServer("nntp.fake1", 30),
			fakeErroreyServer("nntp.fake2", 10),
			fakeErroreyServer("nntp.fake3", 5),
		},
		f:        "testdata/ubuntu-14.04.2-desktop-amd64.nzb",
		metadata: 2,
		files:    15,
		name:     "ubuntu-14.04.2-desktop-amd64",
	},
	{
		s: []*Server{
			fakeErroreyServer("nntp.fake1", 2),
			fakeErroreyServer("nntp.fake2", 1),
			fakeErroreyServer("nntp.fake3", 3),
		},
		f:        "testdata/ubuntu-14.04.2-desktop-amd64.nzb",
		metadata: 2,
		files:    15,
		name:     "ubuntu-14.04.2-desktop-amd64",
	},
}

func TestGrabber(t *testing.T) {
	for _, tt := range grabberTests {
		ss, err := NewStrategy(tt.s)
		if err != nil {
			t.Errorf("NewStrategy(%+v): %v", tt.s, err)
		}

		n, err := nzb.NewFromFile(tt.f)
		if err != nil {
			t.Errorf("nzb.NewFromFile(%v): %v", tt.f, err)
			continue
		}

		g, err := New(
			"/tmp",
			ss,
			FromNZB(n),
			Decoder(createFakeErroreyDecoder),
			SegmentFileCreator(createFakeFile),
		)
		g.Grab()
		g.Shutdown(nil)
	}
}
