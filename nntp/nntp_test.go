package nntp

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/willglynn/nntp"
)

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
	c.Connected = false
	return nil
}

var grabTests = []struct {
	d     dialer
	host  string
	port  int
	tls   bool
	u     string
	pw    string
	ms    int
	group []string
	id    string
}{
	{
		func(s *Server) (conner, error) { return &fc{Connected: true}, nil },
		"news.fake", 119, false, "user", "pw", 3, []string{"a.b.dickbutts"}, "$butt",
	},
	{
		func(s *Server) (conner, error) { return &fc{Connected: true}, nil },
		"news.fake", 119, false, "user", "pw", 3, []string{"a.b.dickbutts", "a.b.dickbutts"}, "$dick",
	},
}

func TestGrab(t *testing.T) {
	for _, tt := range grabTests {
		s := NewServer(tt.host, tt.port, tt.tls, tt.u, tt.pw, tt.ms)
		sn, err := NewSession(s, tt.d)
		if err != nil {
			t.Errorf("NewSession(%#v) error: %v", s, err)
		}

		for _, g := range tt.group {
			b := new(bytes.Buffer)
			bytes, err := sn.writeArticleBody(g, tt.id, b)
			if err != nil {
				t.Errorf("sn.writeArticleBody(%v, %v, %v) error: %v", g, tt.id, b, err)
			}

			if bytes != int64((len(tt.id) + 2)) {
				t.Errorf("sn.writeArticleBody() == %v, wanted %v", bytes, len(tt.id)+2)
			}
			if b.String() != fmt.Sprintf("<%v>", tt.id) {
				t.Errorf("sn.writeArticleBody() wrote %v, wanted <%v>", b.String(), tt.id)
			}
		}

		sn.Quit()

		c := sn.c.(*fc)
		if !c.Authenticated {
			t.Errorf("%+v c.Authenticated == false", sn)
		}
		if !c.Compressed {
			t.Errorf("%+v c.Compressed == false", sn)
		}
		if c.GroupChanges != 1 {
			t.Errorf("%v c.GroupChanges == %v, wanted 1", sn, c.GroupChanges)
		}
		if c.Connected {
			t.Errorf("%+v c.Connected == true", sn)
		}
	}
}
