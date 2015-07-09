package magic

import (
	"bytes"
	"io"
)

const requiredHeaderBytes int = 10

type FileTyper interface {
	FileType() FileType
	SetFileType(t FileType)
}

type Sniffer struct {
	w        io.Writer
	f        FileTyper
	b        *bytes.Buffer
	reqBytes int
	done     bool
	c        int
}

func NewSniffer(w io.Writer, f FileTyper) io.Writer {
	return &Sniffer{
		w:        w,
		f:        f,
		b:        new(bytes.Buffer),
		reqBytes: requiredHeaderBytes,
	}
}

func (s *Sniffer) sniff() {
	b := make([]byte, s.reqBytes)
	s.b.Read(b)
	s.f.SetFileType(GetHeaderType(b))
	s.done = true
}

func (s *Sniffer) Write(p []byte) (int, error) {
	if s.done {
		return s.w.Write(p)
	}

	s.b.Write(p)
	if s.b.Len() >= s.reqBytes {
		s.sniff()
	}
	return s.w.Write(p)
}
