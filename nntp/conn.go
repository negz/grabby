package nntp

import (
	"crypto/tls"
	"io"

	"github.com/willglynn/nntp"
)

// conner is an interface that wraps the functionality of willglynn/nntp.Conn().
// Mostly for testing purposes.
type conner interface {
	Authenticate(u, p string) error
	EnableCompression() error
	Group(g string) (*nntp.Group, error)
	Body(id string) (io.Reader, error)
	Quit() error
}

type dialer func(a string, c *tls.Config) (conner, error)

func nntpDialer(a string, c *tls.Config) (conner, error) {
	switch {
	case c != nil:
		return nntp.DialTLS("tcp", a, c)
	default:
		return nntp.Dial("tcp", a)
	}
}

// IsNoSuchGroupError returns true if error e was recorded while attempting to
// select a non-existent NNTP group.
func IsNoSuchGroupNNTPError(e error) bool {
	err, ok := e.(nntp.Error)
	if !ok {
		return false
	}
	return err.Code == 411
}

// IsNoSuchArticleError returns true if error e was recorded while attempting to
// select a non-existent NNTP Message-Id.
func IsNoSuchArticleNNTPError(e error) bool {
	err, ok := e.(nntp.Error)
	if !ok {
		return false
	}
	return err.Code == 430
}

// IsProtocolError returns true if error e was recorded due to an issue
// speaking NNTP with the server.
func IsNNTPProtocolError(e error) bool {
	_, ok := e.(nntp.ProtocolError)
	return ok
}
