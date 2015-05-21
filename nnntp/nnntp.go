/*
Package nnntp is a new nntp package!
*/
package nnntp

import (
	"crypto/tls"
	"fmt"
	"io"

	"github.com/negz/grabby/util"
	"github.com/willglynn/nntp"
)

// A SessionError records an error handling a server's sessions.
type SessionError string

func (err SessionError) Error() string {
	return string(err)
}

// Connectioner is an interface that wraps the functionality of nntp.Conn().
type Connectioner interface {
	Authenticate(u, p string) error
	EnableCompression() error
	Group(g string) (*nntp.Group, error)
	Body(id string) (io.Reader, error)
	Quit() error
}

type Serverer interface {
	Dial() (Connectioner, error)
	Username() string
	Password() string
}

type Server struct {
	Hostname    string
	Port        int
	TLS         bool
	TLSConfig   *tls.Config
	username    string
	password    string
	MaxSessions int
}

// String returns the server as a hostname:port string.
func (s *Server) String() string {
	return fmt.Sprintf("%v:%v", s.Hostname, s.Port)
}

func (s *Server) Username() string {
	return s.username
}

func (s *Server) Password() string {
	return s.password
}

// NewServer creates and a initialises a new Server.
func NewServer(hostname string, port int, useTLS bool, username, password string, maxSessions int) *Server {
	return &Server{
		Hostname:    hostname,
		Port:        port,
		TLS:         useTLS,
		TLSConfig:   &tls.Config{InsecureSkipVerify: true},
		username:    username,
		password:    password,
		MaxSessions: maxSessions,
	}
}

func (s *Server) Dial() (Connectioner, error) {
	if s.TLS {
		return nntp.DialTLS("tcp", fmt.Sprint(s), s.TLSConfig)
	}
	return nntp.Dial("tcp", fmt.Sprint(s))
}

type Session struct {
	Server       Serverer
	CurrentGroup string
	Compressed   bool
	c            Connectioner
}

func NewSession(s Serverer) (*Session, error) {
	sn := &Session{Server: s}
	c, err := s.Dial()
	if err != nil {
		return nil, err
	}
	sn.c = c
	if err = sn.c.Authenticate(s.Username(), s.Password()); err != nil {
		return nil, err
	}
	if err = sn.c.EnableCompression(); err == nil {
		sn.Compressed = true
	}
	return sn, nil
}

func (s *Session) Close() error {
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

// Grabby fulfills ArticleRequests with ArticleResponses.
func (s *Session) Grabby(req <-chan *ArticleRequest, resp chan<- *ArticleResponse) {
	for request := range req {
		bytes, err := s.writeArticleBody(request.Group, request.ID, request.WriteTo)
		resp <- &ArticleResponse{ArticleRequest: request, Bytes: bytes, Error: err}
	}
}
