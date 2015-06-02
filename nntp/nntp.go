/*
Package nntp is a light wrapper around github.com/willglynn/nntp.
It provides convenience for multi-connection binary downloads.
*/
package nntp

import (
	"crypto/tls"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/negz/grabby/util"
	"github.com/willglynn/nntp"
	"gopkg.in/tomb.v2"
)

// conner is an interface that wraps the functionality of nntp.Conn().
// Mostly for testing purposes.
type conner interface {
	Authenticate(u, p string) error
	EnableCompression() error
	Group(g string) (*nntp.Group, error)
	Body(id string) (io.Reader, error)
	Quit() error
}

// An ArticleRequest specifies an article to download by its Group and ID
// (without <>s).
type ArticleRequest struct {
	Group   string
	ID      string
	WriteTo io.Writer
}

// An ArticleResponse contains the response to an ArticleRequest - either an
// io.Reader or an error.
type ArticleResponse struct {
	*ArticleRequest
	Bytes    int64
	Duration time.Duration
	Error    error
}

// IsNoSuchGroupError returns true if error e was recorded while attempting to
// select a non-existent NNTP group.
func IsNoSuchGroupError(e error) bool {
	err, ok := e.(nntp.Error)
	if !ok {
		return false
	}
	return err.Code == 411
}

// IsNoSuchArticleError returns true if error e was recorded while attempting to
// select a non-existent NNTP Message-Id.
func IsNoSuchArticleError(e error) bool {
	err, ok := e.(nntp.Error)
	if !ok {
		return false
	}
	return err.Code == 430
}

// A Server represents an NNTP server, which may have many connections
type Server struct {
	// TODO(negz): Use a single bidirectional channel?
	// Using two for now because I think we want to fan in on responses only.
	Hostname    string
	Port        int
	TLS         bool
	TLSConfig   *tls.Config
	Username    string
	password    string
	dialSession func(*Server) (*Session, error)
	sessions    []*Session
	ArticleReq  chan *ArticleRequest
	ArticleRsp  chan *ArticleResponse
	t           *tomb.Tomb
}

// String returns the server as a hostname:port string.
func (s *Server) String() string {
	return fmt.Sprintf("%v:%v", s.Hostname, s.Port)
}

// A ServerOption is a function that can be passed in to NewServer to influence
// how the Server is created.
type ServerOption func(*Server) error

// TLS is a ServerOption that enables TLS using the supplied tls.Config.
func TLS(c *tls.Config) ServerOption {
	return func(s *Server) error {
		s.TLS = true
		s.TLSConfig = c
		return nil
	}
}

// Credentials is a ServerOption that enables authentication using the supplied
// username and password.
func Credentials(username, password string) ServerOption {
	return func(s *Server) error {
		s.Username, s.password = username, password
		return nil
	}
}

// SessionDialer is a ServerOption that allows the use of an alternative
// underlying session creation function.
func SessionDialer(d func(*Server) (*Session, error)) ServerOption {
	return func(s *Server) error {
		s.dialSession = d
		return nil
	}
}

// NewServer creates and a initialises a new Server.
func NewServer(hostname string, port int, maxSessions int, so ...ServerOption) (*Server, error) {
	s := &Server{
		Hostname:    hostname,
		Port:        port,
		dialSession: nntpDial,
		sessions:    make([]*Session, maxSessions),
		ArticleReq:  make(chan *ArticleRequest, maxSessions),
		ArticleRsp:  make(chan *ArticleResponse, maxSessions),
	}
	for _, o := range so {
		if err := o(s); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// A Session represents a single connection to a *Server.
type Session struct {
	c             conner
	Connected     bool
	Authenticated bool
	Compressed    bool
	CurrentGroup  string
	// TODO(negz): Why are we seeing races for a session's current group? It
	// should only ever be touched by one goroutine per session.
	groupMx sync.Locker
}

// NewSession returns a new session using the supplied conner.
func NewSession(c conner) *Session {
	return &Session{c: c, groupMx: new(sync.Mutex)}
}

// nntpDial is the default dialSession.
// It uses github.com/willglynn to create and return a Session.
func nntpDial(s *Server) (*Session, error) {
	var err error
	sn := &Session{groupMx: new(sync.Mutex)}

	switch s.TLS {
	case true:
		sn.c, err = nntp.DialTLS("tcp", fmt.Sprint(s), s.TLSConfig)
	case false:
		sn.c, err = nntp.Dial("tcp", fmt.Sprint(s))
	}
	if err != nil {
		return nil, err
	}

	sn.Connected = true
	return sn, nil
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
	sn.Authenticated = true
	return nil
}

// Compress attempts to enable compression for a session.
func (sn *Session) Compress() {
	if err := sn.c.EnableCompression(); err == nil {
		sn.Compressed = true
	}
}

// selectGroup switches the session to the requested group.
// This is a no-op if the session is already in the requested group.
func (sn *Session) selectGroup(g string) error {
	sn.groupMx.Lock()
	defer sn.groupMx.Unlock()
	if sn.CurrentGroup == g {
		return nil
	}
	if _, err := sn.c.Group(g); err != nil {
		return err
	}
	sn.CurrentGroup = g
	return nil
}

// writeArticleBody writes the requested article's body to the supplied
// io.Writer.
func (sn *Session) writeArticleBody(group, id string, w io.Writer) (int64, error) {
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
	sn.Connected = false
	sn.Authenticated = false
	sn.Compressed = false
	sn.CurrentGroup = ""
	return sn.c.Quit()
}

func (s *Server) newSession() (*Session, error) {
	sn, err := s.dialSession(s)
	if err != nil {
		return nil, err
	}
	if err = sn.Authenticate(s.Username, s.password); err != nil {
		return nil, err
	}
	sn.Compress()
	return sn, nil
}

// HandleGrabs uses a Server's sessions to fulfill ArticleRequests.
// All sessions will be quit if any one session becomes unhealthy.
func (s *Server) HandleGrabs() error {
	if s.Working() {
		return nil
	}

	// Make sure all sessions are connected.
	for i := 0; i < len(s.sessions); i++ {
		sn, err := s.newSession()
		if err != nil {
			return err
		}
		s.sessions[i] = sn
	}

	// Tombs cannot be re-used once they have died, so we create a new one here.
	s.t = new(tomb.Tomb)
	for _, sn := range s.sessions {
		s.t.Go(func() error {
			for {
				select {
				case request := <-s.ArticleReq:
					start := time.Now()
					bytes, err := sn.writeArticleBody(request.Group, request.ID, request.WriteTo)
					// A ProtocolError indicates an unhealthy session.
					if _, ok := err.(nntp.ProtocolError); ok {
						return err
					}
					s.ArticleRsp <- &ArticleResponse{
						ArticleRequest: request,
						Bytes:          bytes,
						Duration:       time.Since(start),
						Error:          err,
					}
				case <-s.t.Dying():
					return nil
				}
			}
		})
	}
	return nil
}

// Working returns true if our server is still handling requests.
func (s *Server) Working() bool {
	return s.t != nil && s.t.Alive()
}

// Err returns any errors that have caused the server to die.
func (s *Server) Err() error {
	if s.Working() {
		return nil
	}
	return s.t.Err()
}

// Shutdown disconnects all of the Server's sessions.
func (s *Server) Shutdown() error {
	if s.t == nil {
		return nil
	}
	s.t.Kill(nil)
	err := s.t.Wait()
	for _, sn := range s.sessions {
		// TODO(negz): Log errors? The connection is closed regardless.
		sn.Quit()
	}
	return err
}
