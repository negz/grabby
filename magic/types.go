package magic

type FileType int

const (
	Unknown FileType = iota
	Par2
	Rar
	Nfo
	Sfv
)

var fileTypeName = map[FileType]string{
	Unknown: "Unknown file type",
	Par2:    "Par 2.0",
	Rar:     "RAR Archive",
	Nfo:     "NFO",
	Sfv:     "SFV",
}

func (m FileType) String() string {
	return fileTypeName[m]
}
