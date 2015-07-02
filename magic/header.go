package magic

import "bytes"

type headerDef struct {
	header   []byte
	filetype FileType
}

func (md *headerDef) Match(b []byte) bool {
	return bytes.HasPrefix(b, md.header)
}

var headerTable = []*headerDef{
	&headerDef{header: []byte("PAR2"), filetype: Par2},
	&headerDef{header: []byte("Rar!"), filetype: Rar},
}

func GetHeaderType(b []byte) FileType {
	for _, md := range headerTable {
		if md.Match(b) {
			return md.filetype
		}
	}
	return Unknown
}
