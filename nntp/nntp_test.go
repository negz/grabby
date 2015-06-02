package nntp

import (
	"bytes"
	"crypto/tls"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/willglynn/nntp"
)

// fc satisfies conner, but doesn't do much else.
type fc struct {
	Authenticated bool
	Compressed    bool
	CurrentGroup  string
	GroupChanges  int
	Connected     bool
}

func (c *fc) Authenticate(u, p string) error {
	c.Authenticated = true
	return nil
}

func (c *fc) EnableCompression() error {
	c.Compressed = true
	return nil
}

func (c *fc) Group(g string) (*nntp.Group, error) {
	c.CurrentGroup = g
	c.GroupChanges++
	return nil, nil
}

func (c *fc) Body(id string) (io.Reader, error) {
	return strings.NewReader(id), nil
}

func (c *fc) Quit() error {
	c.Authenticated = false
	c.Compressed = false
	c.Connected = false
	c.CurrentGroup = ""
	c.GroupChanges = 0
	return nil
}

func fakeDial(s *Server) (*Session, error) {
	return &Session{c: &fc{Connected: true}, Connected: true, groupMx: new(sync.Mutex)}, nil
}

type errorDialError string

func (ed errorDialError) Error() string {
	return string(ed)
}

func errorDial(s *Server) (*Session, error) {
	return nil, errorDialError("kaboom!")
}

var grabTests = []struct {
	host         string
	port         int
	ms           int
	opts         []ServerOption
	groups       []string
	groupChanges int
	id           string
}{
	{
		host:         "nntp.fake",
		port:         119,
		ms:           20,
		opts:         []ServerOption{SessionDialer(fakeDial)},
		groups:       []string{"alt.bin.dickbutts"},
		groupChanges: 1,
		id:           "dickbutt$!",
	},
	{
		host:         "nntp.fake",
		port:         119,
		ms:           3,
		opts:         []ServerOption{SessionDialer(errorDial)},
		groups:       []string{"alt.bin.dickbutts"},
		groupChanges: 1,
		id:           "dickbutt$!",
	},
	{
		host:         "nntp.fake",
		port:         119,
		ms:           15,
		opts:         []ServerOption{SessionDialer(fakeDial), Credentials("dick", "butt")},
		groups:       []string{"alt.bin.dickbutts"},
		groupChanges: 1,
		id:           "dickbutt$!",
	},
	{
		host:         "nntp.fake",
		port:         119,
		ms:           1,
		opts:         []ServerOption{SessionDialer(fakeDial), Credentials("dick", "butt")},
		groups:       []string{"alt.bin.dickbutts", "alt.bin.buttdicks"},
		groupChanges: 2,
		id:           "dickbutt$!",
	},
	{
		host:         "nntp.fake",
		port:         119,
		ms:           1,
		opts:         []ServerOption{SessionDialer(fakeDial), TLS(new(tls.Config))},
		groups:       []string{"alt.bin.dickbutts", "alt.bin.buttdicks"},
		groupChanges: 2,
		id:           "dickbutt$!",
	},
}

func TestGrab(t *testing.T) {
	for _, tt := range grabTests {
		s, err := NewServer(tt.host, tt.port, tt.ms, tt.opts...)
		if err != nil {
			t.Errorf("NewServer(%v, %v, %v, %v): %v", tt.host, tt.port, tt.ms, tt.opts, err)
			continue
		}

		// There's no sessions to shutdown yet.
		if err = s.Shutdown(); err != nil {
			t.Errorf("s.Shutdown(): %v", err)
		}

		// Setup some sessions
		if err = s.HandleGrabs(); err != nil {
			if _, ok := err.(errorDialError); ok {
				// We like errorDialErrors.
				continue
			}
			t.Errorf("s.HandleGrabs(): %v", err)
		}

		// We're still working, so this should have no effect until we shutdown
		if err = s.HandleGrabs(); err != nil {
			t.Errorf("s.HandleGrabs(): %v", err)
		}

		// Shutdown fo reals.
		if err = s.Shutdown(); err != nil {
			t.Errorf("s.Shutdown(): %v", err)
		}

		// Replace our sessions with newer, better ones.
		if err = s.HandleGrabs(); err != nil {
			t.Errorf("s.HandleGrabs(): %v", err)
		}

		for _, sn := range s.sessions {
			if s.Username != "" && !sn.Authenticated {
				t.Errorf("%+v sn.Authenticated == false", sn)
			}
			if !sn.Compressed {
				t.Errorf("%+v sn.Compressed == false", sn)
			}
			if !sn.Connected {
				t.Errorf("%+v sn.Connected == false", sn)
			}

			c := sn.c.(*fc)
			if s.Username != "" && !c.Authenticated {
				t.Errorf("%+v c.Authenticated == false", sn)
			}
			if !c.Compressed {
				t.Errorf("%+v c.Compressed == false", sn)
			}
		}

		for _, g := range tt.groups {
			b := new(bytes.Buffer)
			s.ArticleReq <- &ArticleRequest{Group: g, ID: tt.id, WriteTo: b}
			rsp := <-s.ArticleRsp

			if rsp.Error != nil {
				t.Errorf("rsp.Error: %v", rsp.Error)
			}

			if rsp.Bytes != int64(len(tt.id)+2) {
				t.Errorf("rsp.Bytes == %v, wanted %v", rsp.Bytes, len(tt.id)+2)
			}
		}

		if err = s.Err(); err != nil {
			t.Errorf("s.Err(): %v", err)
		}

		// Shutdown fo really reals
		if err = s.Shutdown(); err != nil {
			t.Errorf("s.Shutdown(): %v", err)
		}

		for _, sn := range s.sessions {
			if sn.Authenticated {
				t.Errorf("%+v sn.Authenticated == true", sn)
			}
			if sn.Compressed {
				t.Errorf("%+v sn.Compressed == true", sn)
			}
			if sn.Connected {
				t.Errorf("%+v sn.Connected == true", sn)
			}

			c := sn.c.(*fc)
			if c.Authenticated {
				t.Errorf("%+v c.Authenticated == true", sn)
			}
			if c.Compressed {
				t.Errorf("%+v c.Compressed == true", sn)
			}
			if c.Connected {
				t.Errorf("%+v c.Connected == true", sn)
			}
		}

	}
}
