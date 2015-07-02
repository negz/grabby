package magic

import "testing"

var headerTests = []struct {
	header   []byte
	filetype FileType
}{
	{header: []byte("\x50\x41\x52\x32\x00\x50\x4B\x54\xA0\x00"), filetype: Par2},
	{header: []byte("\x52\x61\x72\x21\x1A\x07\x00\x19\x7A\x73"), filetype: Rar},
	{header: []byte("\xCF\xFA\xED\xFE\x07\x00\x00\x01\x03\x00"), filetype: Unknown},
	{header: []byte("\x00"), filetype: Unknown},
	{header: []byte{}, filetype: Unknown},
}

func TestHeader(t *testing.T) {
	t.Parallel()
	for _, tt := range headerTests {
		if GetHeaderType(tt.header) != tt.filetype {
			t.Errorf("GetHeaderType(%#v) == %v, want %v", tt.header, GetHeaderType(tt.header), tt.filetype)
		}
	}
}
