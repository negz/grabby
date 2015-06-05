package nntp

import (
	"crypto/tls"
	"io"
	"sync"

	"github.com/negz/grabby/util"
)

// Sessioner is implemented by Server Sessions.
type Sessioner interface {
	Connect(address string, c *tls.Config) error
	Authenticate(username, password string) error
	Compress()
	Status() (bool, bool, bool)
	WriteArticleBody(group, id string, w io.Writer) (int64, error)
	Quit() error
}

// A Session represents a single connection to a Server.
type Session struct {
	d             dialer
	c             conner
	connected     bool
	authenticated bool
	compressed    bool
	currentGroup  string
	// TODO(negz): Why are we seeing races for a session's current group? It
	// should only ever be touched by one goroutine per session.
	groupMx sync.Locker
}

// NewSession returns a new session using the supplied conner.
func NewSession(d dialer) Sessioner {
	return &Session{
		d:       d,
		groupMx: new(sync.Mutex),
	}
}

// Connect connects the Session.
func (sn *Session) Connect(a string, c *tls.Config) error {
	conn, err := sn.d(a, c)
	if err != nil {
		return err
	}
	sn.c = conn
	sn.connected = true
	return nil
}

// Authenticate authenticates a session. It does nothing if the supplied
// username is the empty string.
func (sn *Session) Authenticate(username, password string) error {
	if username == "" {
		return nil
	}
	if err := sn.c.Authenticate(username, password); err != nil {
		return err
	}
	sn.authenticated = true
	return nil
}

// Compress attempts to enable compression for a session.
func (sn *Session) Compress() {
	if err := sn.c.EnableCompression(); err == nil {
		sn.compressed = true
	}
}

// Status returns booleans representing whether this session is currently
// connected, authenticated, and compressed, respectively.
func (sn *Session) Status() (bool, bool, bool) {
	return sn.connected, sn.authenticated, sn.compressed
}

// selectGroup switches the session to the requested group.
// This is a no-op if the session is already in the requested group.
func (sn *Session) selectGroup(g string) error {
	if g == "" {
		return nil
	}
	sn.groupMx.Lock()
	defer sn.groupMx.Unlock()
	if sn.currentGroup == g {
		return nil
	}
	if _, err := sn.c.Group(g); err != nil {
		return err
	}
	sn.currentGroup = g
	return nil
}

// WriteArticleBody writes the requested article's body to the supplied
// io.Writer.
func (sn *Session) WriteArticleBody(group, id string, w io.Writer) (int64, error) {
	if err := sn.selectGroup(group); err != nil {
		return 0, err
	}
	body, err := sn.c.Body(util.FormatArticleID(id))
	if err != nil {
		return 0, err
	}

	return io.Copy(w, body)
}

// Quit terminates a Session, including sending QUIT.
func (sn *Session) Quit() error {
	sn.connected = false
	sn.authenticated = false
	sn.compressed = false
	sn.currentGroup = ""
	return sn.c.Quit()
}
