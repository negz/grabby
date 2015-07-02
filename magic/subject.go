package magic

import "regexp"

type subjectDef struct {
	re       *regexp.Regexp
	filetype FileType
}

func (sd *subjectDef) Match(s string) bool {
	return sd.re.MatchString(s)
}

func (sd *subjectDef) Filename(s string) string {
	m := sd.re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

var subjectTable = []*subjectDef{
	&subjectDef{re: regexp.MustCompile(`(?i).*(?:"|&quot;|&#34;)(.+\.r(?:ar|\d+))(?:"|&quot;|&#34;) yEnc`), filetype: Rar},
	&subjectDef{re: regexp.MustCompile(`(?i)(.+\.r(?:ar|\d+))`), filetype: Rar},

	&subjectDef{re: regexp.MustCompile(`(?i).*(?:"|&quot;|&#34;)(.+\.par2)(?:"|&quot;|&#34;) yEnc`), filetype: Par2},
	&subjectDef{re: regexp.MustCompile(`(?i)(.+\.par2)`), filetype: Par2},

	&subjectDef{re: regexp.MustCompile(`(?i).*(?:"|&quot;|&#34;)(.+\.nfo)(?:"|&quot;|&#34;) yEnc`), filetype: Nfo},
	&subjectDef{re: regexp.MustCompile(`(?i)(.+\.nfo)`), filetype: Nfo},

	&subjectDef{re: regexp.MustCompile(`(?i).*(?:"|&quot;|&#34;)(.+\.sfv)(?:"|&quot;|&#34;) yEnc`), filetype: Sfv},
	&subjectDef{re: regexp.MustCompile(`(?i)(.+\.sfv)`), filetype: Sfv},
}

func GetSubjectType(s string) FileType {
	for _, sd := range subjectTable {
		if sd.Match(s) {
			return sd.filetype
		}
	}
	return Unknown
}

func GetSubjectFilename(s string) string {
	for _, sd := range subjectTable {
		if sd.Match(s) {
			return sd.Filename(s)
		}
	}
	return ""
}
