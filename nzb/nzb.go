// Package nzb implements functions for unmarshaling NZB files.
package nzb

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"code.google.com/p/go-charset/charset"
	_ "code.google.com/p/go-charset/data" // NZBs often use iso-8859-1
	"github.com/negz/xmlstream"
)

// A Segment represents a segment of a file posted to Usenet and represented in
// an NZB.
type Segment struct {
	XMLName   xml.Name `xml:"segment"`
	Bytes     uint64   `xml:"bytes,attr"`
	Number    int      `xml:"number,attr"`
	ArticleID string   `xml:",innerxml"`
}

func (s *Segment) String() string {
	return fmt.Sprintf("%v (segment %v, %v bytes)", s.ArticleID, s.Number, s.Bytes)
}

// A File represents a file posted to Usenet and represented in an NZB.
type File struct {
	XMLName  xml.Name   `xml:"file"`
	Poster   string     `xml:"poster,attr"`
	Date     int64      `xml:"date,attr"` // TODO(negz): time.Unix()?
	Subject  string     `xml:"subject,attr"`
	Groups   []string   `xml:"groups>group"`
	Segments []*Segment `xml:"segments>segment"`
}

func (f *File) String() string {
	return fmt.Sprintf("%v (%v segments in %v groups)", f.Subject, len(f.Segments), len(f.Groups))
}

// Metadata represents an element of metadata in an NZB.
type Metadata struct {
	XMLName xml.Name `xml:"meta"`
	Type    string   `xml:"type,attr"`
	Value   string   `xml:",innerxml"`
}

func (m *Metadata) String() string {
	return fmt.Sprintf("%v:%v", m.Type, m.Value)
}

// An NZB is a representation of an NZB file.
type NZB struct {
	Filename string // For renaming output files with munged named.
	Metadata []*Metadata
	Files    []*File
}

func (n *NZB) String() string {
	name := "NZB"
	if n.Filename != "" {
		name = n.Filename
	}
	return fmt.Sprintf("%v (%v files)", name, len(n.Files))
}

// New unmarshals an io.Reader into an *NZB.
func New(x io.Reader) (*NZB, error) {
	n := &NZB{Metadata: make([]*Metadata, 0), Files: make([]*File, 0)}
	// TODO(negz): This might be overkill for NZB files.
	s := xmlstream.NewScanner(x, &Metadata{}, &File{})
	s.Decoder.CharsetReader = charset.NewReader

	for s.Scan() {
		tag := s.Element()
		switch element := tag.(type) {
		case *Metadata:
			n.Metadata = append(n.Metadata, element)
		case *File:
			n.Files = append(n.Files, element)
		}
	}

	if err := s.Err(); err != nil {
		return nil, err
	}
	return n, nil
}

// NewFromFile unmarshals an os.File into an *NZB, and records its filename.
func NewFromFile(name string) (*NZB, error) {
	nzbFile, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer nzbFile.Close()

	nzb, err := New(nzbFile)
	if err != nil {
		return nil, err
	}

	nzb.Filename = filepath.Base(nzbFile.Name())
	return nzb, nil
}
