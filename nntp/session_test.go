package nntp

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
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

func fakeDialer(a string, c *tls.Config) (conner, error) {
	return &fc{Connected: true}, nil
}

var dialError = errors.New("dial error")

func errorDialer(a string, c *tls.Config) (conner, error) {
	return nil, dialError
}

var sessionTests = []struct {
	d            dialer
	groups       []string
	id           string
	w            io.ReadWriter
	groupChanges int
}{
	{errorDialer, []string{"alt.dick.butts"}, "dick@butt$", new(bytes.Buffer), 1},
	{fakeDialer, []string{"alt.dick.butts"}, "dick@butt$", new(bytes.Buffer), 1},
	{fakeDialer, []string{"alt.dick.butts", "alt.dick.butts"}, "dick@butt$", new(bytes.Buffer), 1},
	{fakeDialer, []string{"alt.dick.butts", "", "alt.duck.bitts"}, "dick@butt$", new(bytes.Buffer), 2},
}

func TestSession(t *testing.T) {
	for _, tt := range sessionTests {
		s := NewSession(tt.d)
		if err := s.Connect("nntp1.fake", nil); err != nil {
			if err == dialError {
				continue
			}
			t.Errorf("s.Connect(): %v", err)
		}
		if err := s.Authenticate("dick", "butt"); err != nil {
			t.Errorf("s.Authenticate(): %v", err)
		}
		s.Compress()

		connected, authenticated, compressed := s.Status()
		if !connected {
			t.Errorf("s.Status connected == false")
		}
		if !authenticated {
			t.Errorf("s.Status authenticated == false")
		}
		if !compressed {
			t.Errorf("s.Status compressed == false")
		}

		for _, g := range tt.groups {
			s.WriteArticleBody(g, tt.id, tt.w)
			if r, _ := ioutil.ReadAll(tt.w); string(r) != fmt.Sprintf("<%v>", tt.id) {
				t.Errorf("s.WriteArticleBody(%v, %v, buffer) wrote %s, wanted %v", g, tt.id, r, tt.id)
			}
		}

		sn := s.(*Session)
		conn := sn.c.(*fc)

		if !conn.Connected {
			t.Errorf("s.c.Connected == false")
		}
		if !conn.Authenticated {
			t.Errorf("s.c.Authenticated == false")
		}
		if !conn.Compressed {
			t.Errorf("s.c.Compressed == false")
		}

		if conn.GroupChanges != tt.groupChanges {
			t.Errorf("s.c.GroupChanges == %v, want %v", conn.GroupChanges, tt.groupChanges)
		}

		s.Quit()
		connected, authenticated, compressed = s.Status()
		if connected {
			t.Errorf("s.Status connected == true")
		}
		if authenticated {
			t.Errorf("s.Status authenticated == true")
		}
		if compressed {
			t.Errorf("s.Status compressed == true")
		}

		if conn.Connected {
			t.Errorf("s.c.Connected == true")
		}
		if conn.Authenticated {
			t.Errorf("s.c.Authenticated == true")
		}
		if conn.Compressed {
			t.Errorf("s.c.Compressed == true")
		}
	}
}
