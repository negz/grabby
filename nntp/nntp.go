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

	"github.com/negz/grabby/util"
	"github.com/willglynn/nntp"
)

// conner is an interface that wraps the functionality of nntp.Conn().
type conner interface {
	Authenticate(u, p string) error
	EnableCompression() error
	Group(g string) (*nntp.Group, error)
	Body(id string) (io.Reader, error)
	Quit() error
}

// A Server represents an NNTP server, which may have many connections
type Server struct {
	Hostname    string
	Port        int
	TLS         bool
	TLSConfig   *tls.Config
	Username    string
	password    string
	MaxSessions int
}

// String returns the server as a hostname:port string.
func (s *Server) String() string {
	return fmt.Sprintf("%v:%v", s.Hostname, s.Port)
}

// NewServer creates and a initialises a new Server.
func NewServer(hostname string, port int, useTLS bool, username, password string, maxSessions int) *Server {
	return &Server{
		Hostname:    hostname,
		Port:        port,
		TLS:         useTLS,
		TLSConfig:   &tls.Config{InsecureSkipVerify: true},
		Username:    username,
		password:    password,
		MaxSessions: maxSessions,
	}
}

// A dialer dials a server and returns a conner.
type dialer func(*Server) (conner, error)

// Dial dials the supplied *Server, returning a connection.
func Dial(s *Server) (conner, error) {
	if s.TLS {
		return nntp.DialTLS("tcp", fmt.Sprint(s), s.TLSConfig)
	}
	return nntp.Dial("tcp", fmt.Sprint(s))
}

// A Session represents a single connection to a *Server.
type Session struct {
	Server       *Server
	CurrentGroup string
	Compressed   bool
	c            conner
}

// NewSession creates, authenticates, and attempts to enable compression for a new *Session.
func NewSession(s *Server, d dialer) (*Session, error) {
	sn := &Session{Server: s}
	c, err := d(s)
	if err != nil {
		return nil, err
	}
	sn.c = c
	if err = sn.c.Authenticate(s.Username, s.password); err != nil {
		return nil, err
	}
	if err = sn.c.EnableCompression(); err == nil {
		sn.Compressed = true
	}
	return sn, nil
}

// Quit() terminates a *Session, including sending QUIT.
func (s *Session) Quit() error {
	return s.c.Quit()
}

// selectGroup switches the session to the requested group.
// This is a no-op if the session is already in the requested group.
func (s *Session) selectGroup(g string) error {
	if s.CurrentGroup == g {
		return nil
	}
	if _, err := s.c.Group(g); err != nil {
		return err
	}
	s.CurrentGroup = g
	return nil
}

// writeArticleBody writes the requested article's body to the supplied
// io.Writer.
func (s *Session) writeArticleBody(group, id string, w io.Writer) (int64, error) {
	if err := s.selectGroup(group); err != nil {
		return 0, err
	}
	body, err := s.c.Body(util.FormatArticleID(id))
	if err != nil {
		return 0, err
	}

	return io.Copy(w, body)
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
	Bytes int64
	Error error
}

// Grab fulfills ArticleRequests with ArticleResponses.
func (s *Session) Grab(wg *sync.WaitGroup, req <-chan *ArticleRequest, resp chan<- *ArticleResponse) {
	defer wg.Done()
	for request := range req {
		bytes, err := s.writeArticleBody(request.Group, request.ID, request.WriteTo)
		resp <- &ArticleResponse{ArticleRequest: request, Bytes: bytes, Error: err}
	}
}
