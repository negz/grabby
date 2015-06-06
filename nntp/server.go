/*
Package nntp is a light wrapper around github.com/willglynn/nntp.
It provides convenience for multi-connection binary downloads.
*/
package nntp

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"time"

	"gopkg.in/tomb.v2"
)

var (
	NoSuchArticleError = errors.New("no such article ID")
	NoSuchGroupError   = errors.New("no such group")
)

// An GrabRequest specifies an article to download by its Group and ID
// (without <>s).
type GrabRequest struct {
	Group   string
	ID      string
	WriteTo io.Writer
}

// An GrabResponse contains the response to an GrabRequest - either an
// io.Reader or an error.
type GrabResponse struct {
	*GrabRequest
	Bytes    int64
	Duration time.Duration
	Error    error
}

type Serverer interface {
	Address() string
	TLS() bool
	Username() string
	HandleGrabs() error
	Grab(g *GrabRequest)
	Grabbed() <-chan *GrabResponse
	Alive() bool
	Err() error
	Shutdown(err error) error
}

// A Server represents an NNTP server, which may have many connections
type Server struct {
	// TODO(negz): Use a single bidirectional channel?
	// Using two for now because I think we want to fan in on responses only.
	hostname  string
	port      int
	tlsConfig *tls.Config
	username  string
	password  string
	sd        dialer
	sessions  []Sessioner
	gReq      chan *GrabRequest
	gRsp      chan *GrabResponse
	t         *tomb.Tomb
}

// String returns the server as a hostname:port string.
func (s *Server) String() string {
	return s.Address()
}

// A ServerOption is a function that can be passed in to NewServer to influence
// how the Server is created.
type ServerOption func(*Server) error

// TLS is a ServerOption that enables TLS using the supplied tls.Config.
func TLS(c *tls.Config) ServerOption {
	return func(s *Server) error {
		s.tlsConfig = c
		return nil
	}
}

// Credentials is a ServerOption that enables authentication using the supplied
// username and password.
func Credentials(username, password string) ServerOption {
	return func(s *Server) error {
		s.username, s.password = username, password
		return nil
	}
}

// sessionDialer is a ServerOption that allows the use of an alternative
// underlying session creation function.
func sessionDialer(d dialer) ServerOption {
	return func(s *Server) error {
		s.sd = d
		return nil
	}
}

// NewServer creates and a initialises a new Server.
func NewServer(hostname string, port int, maxSessions int, so ...ServerOption) (Serverer, error) {
	s := &Server{
		hostname: hostname,
		port:     port,
		sd:       nntpDialer,
		sessions: make([]Sessioner, maxSessions),
		gReq:     make(chan *GrabRequest, maxSessions),
		gRsp:     make(chan *GrabResponse, maxSessions),
	}
	for _, o := range so {
		if err := o(s); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Server) newSession() (Sessioner, error) {
	sn := NewSession(s.sd)
	if err := sn.Connect(s.Address(), s.tlsConfig); err != nil {
		return nil, err
	}
	if err := sn.Authenticate(s.username, s.password); err != nil {
		return nil, err
	}
	sn.Compress()
	return sn, nil
}

// HandleGrabs uses a Server's sessions to fulfill GrabRequests.
// All sessions will be quit if any one session becomes unhealthy.
func (s *Server) HandleGrabs() error {
	if s.Alive() {
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
				case req := <-s.gReq:
					start := time.Now()
					bytes, err := sn.WriteArticleBody(req.Group, req.ID, req.WriteTo)
					switch {
					case IsNNTPProtocolError(err):
						return err
					case IsNoSuchGroupNNTPError(err):
						err = NoSuchGroupError
					case IsNoSuchArticleNNTPError(err):
						err = NoSuchArticleError
					}
					s.gRsp <- &GrabResponse{
						GrabRequest: req,
						Bytes:       bytes,
						Duration:    time.Since(start),
						Error:       err,
					}
				case <-s.t.Dying():
					return nil
				}
			}
		})
	}
	return nil
}

// Address returns the Server's address as a host:port string.
func (s *Server) Address() string {
	return fmt.Sprintf("%v:%v", s.hostname, s.port)
}

// TLS returns true if the Server is using TLS.
func (s *Server) TLS() bool {
	return s.tlsConfig != nil
}

// Username returns the configured Username, or an empty string if
// authentication is not enabled.
func (s *Server) Username() string {
	return s.username
}

// Grab fulfills the passed GrabRequest.
func (s *Server) Grab(g *GrabRequest) {
	s.gReq <- g
}

// Grabbed returns a channel of GrabResponses to Grab()s.
func (s *Server) Grabbed() <-chan *GrabResponse {
	return s.gRsp
}

// Alive returns true if our server is still handling requests.
func (s *Server) Alive() bool {
	return s.t != nil && s.t.Alive()
}

// Err returns any errors that have caused the server to die.
func (s *Server) Err() error {
	if s.t == nil || s.t.Err() == tomb.ErrStillAlive {
		return nil
	}
	return s.t.Err()
}

// Shutdown disconnects all of the Server's sessions.
func (s *Server) Shutdown(err error) error {
	if s.t == nil {
		return nil
	}
	s.t.Kill(err)
	err = s.t.Wait()
	for _, sn := range s.sessions {
		// TODO(negz): Log errors? The connection is closed regardless.
		sn.Quit()
	}
	return err
}
