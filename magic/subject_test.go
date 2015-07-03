package magic

import "testing"

var subjectTests = []struct {
	subject  string
	filetype FileType
	filename string
}{
	{
		subject:  "Ubuntu 14.04.2 LTS 64-bit - [001/115] - &quot;ubuntu-14.04.2-desktop-amd64.nfo&quot; yEnc (1/1)",
		filetype: Nfo,
		filename: "ubuntu-14.04.2-desktop-amd64.nfo",
	},
	{
		subject:  "Ubuntu 14.04.2 LTS 64-bit - [001/115] - &#34;ubuntu 14.04.2 desktop amd64.sfv&#34; yEnc (1/1)",
		filetype: Sfv,
		filename: "ubuntu 14.04.2 desktop amd64.sfv",
	},
	{
		subject:  "zqjo62c297vyoq5qyid8mxe00ytro63c.vol002+04.par2&#34; yEnc (3/11)",
		filetype: Par2,
		filename: "zqjo62c297vyoq5qyid8mxe00ytro63c.vol002+04.par2",
	},
	{
		subject:  "Ubuntu 14.04.2 LTS 64-bit - [003/115] - &quot;ubuntu-14.04.2-desktop-amd64.part001.rar&quot; yEnc (1/28)",
		filetype: Rar,
		filename: "ubuntu-14.04.2-desktop-amd64.part001.rar",
	},
	{
		subject:  "Ubuntu 14.04.2 LTS 64-bit - [003/115] - \"ubuntu-14.04.2-desktop-amd64.part001.rar\" yEnc (1/28)",
		filetype: Rar,
		filename: "ubuntu-14.04.2-desktop-amd64.part001.rar",
	},
	{
		subject:  "ubuntu-14.04.2-desktop-amd64.r00 (1/3)",
		filetype: Rar,
		filename: "ubuntu-14.04.2-desktop-amd64.r00",
	},
	{
		subject:  "Here is ubuntu-14.04.2-desktop-amd64.r003 lolz",
		filetype: Rar,
		filename: "Here is ubuntu-14.04.2-desktop-amd64.r003",
	},
	{
		subject:  "Here is ubuntu-14.04.2-desktop-amd64.z01",
		filetype: Unknown,
		filename: "Here is ubuntu-14.04.2-desktop-amd64.z01",
	},
}

func TestSubject(t *testing.T) {
	t.Parallel()
	for _, tt := range subjectTests {
		if GetSubjectType(tt.subject) != tt.filetype {
			t.Errorf("GetSubjectType(%v) == %v, want %v", tt.subject, GetSubjectType(tt.subject), tt.filetype)
		}
		if GetSubjectFilename(tt.subject) != tt.filename {
			t.Errorf("GetSubjectFilename(%v) == %v, want %v", tt.subject, GetSubjectFilename(tt.subject), tt.filename)
		}
	}
}
