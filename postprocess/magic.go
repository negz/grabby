package postprocess

import "bytes"

type FileMagic int

const (
	Unknown FileMagic = iota
	Par2
	Rar
)

var fileMagicName = map[FileMagic]string{
	Unknown: "Unknown file type",
	Par2:    "Par 2.0",
	Rar:     "RAR Archive",
}

func (m FileMagic) String() string {
	return fileMagicName[m]
}

type magicDef struct {
	header []byte
	magic  FileMagic
}

func (md *magicDef) Match(b []byte) bool {
	return bytes.HasPrefix(b, md.header)
}

var magicTable = []*magicDef{
	&magicDef{header: []byte("PAR2"), magic: Par2},
	&magicDef{header: []byte("Rar!"), magic: Rar},
}

func GetMagic(b []byte) FileMagic {
	for _, md := range magicTable {
		if md.Match(b) {
			return md.magic
		}
	}
	return Unknown
}
