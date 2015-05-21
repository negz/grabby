package nzb

import (
	"os"
	"testing"
)

var nzbTests = []struct {
	f        string
	metadata int
	files    int
	filename string
}{
	{"testdata/spec.nzb", 4, 1, "spec.nzb"},
	{"testdata/ubuntu-14.04.2-desktop-amd64.nzb", 2, 115, "ubuntu-14.04.2-desktop-amd64.nzb"},
}

func TestNew(t *testing.T) {
	for _, tt := range nzbTests {
		xml, err := os.Open(tt.f)
		if err != nil {
			t.Errorf("error opening test data %v: %v", tt.f, err)
			continue
		}
		n, err := New(xml)
		if err != nil {
			t.Errorf("error parsing NZB %v: %v", tt.f, err)
			continue
		}
		if len(n.Metadata) != tt.metadata {
			t.Errorf("%v len(n.Metadata) == %v, wanted %v", tt.f, len(n.Metadata), tt.metadata)
		}
		if len(n.Files) != tt.files {
			t.Errorf("%v len(n.Files) == %v, wanted %v", tt.f, len(n.Files), tt.files)
		}
	}
}

func TestNewFromFile(t *testing.T) {
	for _, tt := range nzbTests {
		n, err := NewFromFile(tt.f)
		if err != nil {
			t.Errorf("error parsing NZB %v: %v", tt.f, err)
			continue
		}
		if n.Filename != tt.filename {
			t.Errorf("%v n.Filename == %v, wanted %v", tt.f, n.Filename, tt.filename)
		}
		if len(n.Metadata) != tt.metadata {
			t.Errorf("%v len(n.Metadata) == %v, wanted %v", tt.f, len(n.Metadata), tt.metadata)
		}
		if len(n.Files) != tt.files {
			t.Errorf("%v len(n.Files) == %v, wanted %v", tt.f, len(n.Files), tt.files)
		}
	}
}
