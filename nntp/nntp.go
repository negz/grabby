/*
Package nntp implements functions for interfacing with an NNTP server.
It is designed to handle multiple sessions, and has a focus on downloading
binary files.
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

// A SessionError records an error handling a server's sessions.
type SessionError string

func (err SessionError) Error() string {
	return string(err)
}

// A Session represents a single connection to a usenet server.
type Session struct {
	Server       *Server
	CurrentGroup string
	Compressed   bool
	Busy         bool
	Connected    bool
	conn         *nntp.Conn
}

// dial connects, authenticates, and attempts to enable compression.
func (s *Session) dial() error {
	if s.Connected {
		return SessionError("session is already connected")
	}

	var conn *nntp.Conn
	var err error
	if s.Server.TLS {
		conn, err = nntp.DialTLS("tcp", fmt.Sprint(s.Server), s.Server.TLSConfig)
	} else {
		conn, err = nntp.Dial("tcp", fmt.Sprint(s.Server))
	}
	if err != nil {
		return err
	}

	if err := conn.Authenticate(s.Server.Username, s.Server.password); err != nil {
		return err
	}

	s.Connected = true
	if err := conn.EnableCompression(); err == nil {
		s.Compressed = true
	}

	s.conn = conn
	return nil
}

// busy marks a session as busy.
func (s *Session) busy() {
	s.Server.sessionMutex.Lock()
	defer s.Server.sessionMutex.Unlock()

	delete(s.Server.idle, s)
	s.Server.busy[s] = true
	s.Busy = true
}

// idle marks a session as idle.
func (s *Session) idle() {
	s.Server.sessionMutex.Lock()
	defer s.Server.sessionMutex.Unlock()

	delete(s.Server.busy, s)
	s.Server.idle[s] = true
	s.Busy = false
}

// close attempts to cleanly QUIT and close the session.
// Note the connection is always terminated, even if an error is raised on QUIT.
func (s *Session) close() error {
	s.Connected = false
	return s.conn.Quit()
}

// selectGroup switches the session to the requested group.
// This is a no-op if the session is already in the requested group.
func (s *Session) selectGroup(g string) error {
	if s.CurrentGroup == g {
		return nil
	}

	if _, err := s.conn.Group(g); err != nil {
		return err
	}

	s.CurrentGroup = g
	return nil
}

// getArticleBody returns an io.Reader for the body of article id (without <>)
// in the current group.
func (s *Session) getArticleBody(id string) (io.Reader, error) {
	body, err := s.conn.Body(util.FormatArticleID(id))
	if err != nil {
		return nil, err
	}
	return body, nil
}

// A Server represents a usenet server, which may have many sessions.
type Server struct {
	Hostname        string
	Port            int
	TLS             bool
	TLSConfig       *tls.Config
	Username        string
	password        string
	MaxSessions     int
	sessionMutex    *sync.Mutex
	busy            map[*Session]bool
	idle            map[*Session]bool
	isDisconnecting bool
}

// NewServer creates and a initialises a new Server.
func NewServer(hostname string, port int, useTLS bool, username, password string, maxSessions int) *Server {
	return &Server{
		Hostname:     hostname,
		Port:         port,
		TLS:          useTLS,
		TLSConfig:    &tls.Config{InsecureSkipVerify: true},
		Username:     username,
		password:     password,
		MaxSessions:  maxSessions,
		sessionMutex: &sync.Mutex{},
		busy:         make(map[*Session]bool),
		idle:         make(map[*Session]bool),
	}
}

// String returns the server as a hostname:port string.
func (s *Server) String() string {
	return fmt.Sprintf("%v:%v", s.Hostname, s.Port)
}

// Disconnect terminates all sessions to the server.
func (s *Server) Disconnect() {
	s.isDisconnecting = true
	defer func() { s.isDisconnecting = false }()

	// TODO(negz): Too broad - i.e. will prevent busy sessions becoming idle
	s.sessionMutex.Lock()
	defer s.sessionMutex.Unlock()

	for session := range s.idle {
		session.close()
		delete(s.idle, session)
	}

	for session := range s.busy {
		// TODO(negz): Wait for session to become idle? Will need some timeout.
		session.close()
		delete(s.busy, session)
	}
}

// newSession returns a new *Session to the server.
func (s *Server) newSession() (*Session, error) {
	session := &Session{Server: s}
	if err := session.dial(); err != nil {
		return nil, err
	}
	return session, nil
}

// getSession returns a *Session to download... stuff with.
func (s *Server) getSession() (*Session, error) {
	if s.isDisconnecting {
		return nil, SessionError(fmt.Sprintf("server %v is currently disconnecting", s))
	}

	s.sessionMutex.Lock()
	defer s.sessionMutex.Unlock()

	// Try to return an idle session
	for session := range s.idle {
		return session, nil
	}

	// We know there's no idle sessions, and we're at max busy sessions.
	if len(s.busy) == s.MaxSessions {
		return nil, SessionError(fmt.Sprintf("server %v has no available sessions", s))
	}

	// We know there's no idle sessions, and we're under max busy sessions.
	session, err := s.newSession()
	if err != nil {
		return nil, err
	}
	return session, nil
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

// writeArticleBody writes the requested article's body to the supplied
// io.Writer.
func (s *Server) writeArticleBody(group, id string, w io.Writer) (int64, error) {
	session, err := s.getSession()
	if err != nil {
		return 0, err
	}
	session.busy()
	defer session.idle()

	if err = session.selectGroup(group); err != nil {
		return 0, err
	}

	body, err := session.getArticleBody(id)
	if err != nil {
		return 0, err
	}

	return io.Copy(w, body)
}

// Grabby fulfills ArticleRequests with ArticleResponses.
func (s *Server) Grabby(req <-chan *ArticleRequest, resp chan<- *ArticleResponse) {
	for request := range req {
		bytes, err := s.writeArticleBody(request.Group, request.ID, request.WriteTo)
		resp <- &ArticleResponse{ArticleRequest: request, Bytes: bytes, Error: err}
	}
}
