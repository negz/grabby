package magic

import (
	"bytes"
	"testing"
)

type fakeThingWithFileType struct {
	t FileType
}

func (f *fakeThingWithFileType) FileType() FileType {
	return f.t
}

func (f *fakeThingWithFileType) SetFileType(t FileType) {
	f.t = t
}

var snifferTests = []struct {
	header   []byte
	filetype FileType
}{
	{header: []byte("\x50\x41\x52\x32\x00\x50\x4B\x54\xA0\x00"), filetype: Par2},
	{header: []byte("\x52\x61\x72\x21\x1A\x07\x00\x19\x7A\x73"), filetype: Rar},
	{header: []byte("\xCF\xFA\xED\xFE\x07\x00\x00\x01\x03\x00"), filetype: Unknown},
}

func TestSniffer(t *testing.T) {
	t.Parallel()
	for _, tt := range snifferTests {
		b := new(bytes.Buffer)
		f := new(fakeThingWithFileType)
		s := NewSniffer(b, f)
		s.Write(tt.header)

		if f.FileType() != tt.filetype {
			t.Errorf("<-f.FileType() == %v, want %v", f.FileType(), tt.filetype)
		}
	}
}
