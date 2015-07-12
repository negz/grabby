package postprocess

import (
	"errors"
	"os/exec"
	"regexp"
	"time"

	"github.com/juju/deputy"
	"github.com/negz/grabby/util"
)

var blocksRegexp *regexp.Regexp = regexp.MustCompile(`(?i)^.+\.vol\d+\+(\d+).par2$`)

func BlocksFromFilename(filename string) int {
	if !blocksRegexp.MatchString(filename) {
		return 0
	}
	return util.DefaultAtoi(blocksRegexp.FindStringSubmatch(filename)[1], 0)
}

type lineType int

const (
	unknown lineType = iota
	version
	numFiles
	repairComplete
	repairNotPossible
	fileRenamed
)

type par2Line struct {
	linetype lineType
	re       *regexp.Regexp
}

func (p2l par2Line) Match(b []byte) bool {
	return p2l.re.Match(b)
}

func (p2l par2Line) Submatches(b []byte) []string {
	return p2l.re.FindStringSubmatch(string(b))
}

var lineTable = []*par2Line{
	&par2Line{version, regexp.MustCompile(`^par2cmdline version (.+), .*`)},
	&par2Line{numFiles, regexp.MustCompile(`^There are (\d+) recoverable files and \d+ other files.$`)},
	&par2Line{repairComplete, regexp.MustCompile(`^All files are correct, repair is not required.$`)},
	&par2Line{repairComplete, regexp.MustCompile(`^Repair complete.$`)},
	&par2Line{repairNotPossible, regexp.MustCompile(`^You need (\d+) more recovery blocks to be able to repair.$`)},
	&par2Line{fileRenamed, regexp.MustCompile(`File: "(.+)" - is a match for "(.+)".$`)},
}

type Repairer interface {
	Repair() error
	Repaired() bool
	RenamedFiles() map[string]string
	BlocksNeeded() int
}

type Par2Cmdline struct {
	args         []string
	par2FilePath string
	filePaths    []string
	cmd          *exec.Cmd
	timeout      time.Duration
	version      string // TODO(negz): Whitelist supported versions?
	numFiles     int
	repaired     bool
	blocksNeeded int
	renamedFiles map[string]string
	err          error
}

type Par2CmdlineOption func(*Par2Cmdline)

func Timeout(t time.Duration) Par2CmdlineOption {
	return func(p2 *Par2Cmdline) {
		p2.timeout = t
	}
}

func Args(a ...string) Par2CmdlineOption {
	return func(p2 *Par2Cmdline) {
		p2.args = a
	}
}

func Par2(par2 Filer) Par2CmdlineOption {
	return func(p2 *Par2Cmdline) {
		p2.par2FilePath = par2.Path()
	}
}

func Files(files []Filer) Par2CmdlineOption {
	return func(p2 *Par2Cmdline) {
		fp := make([]string, len(files))
		for i, p := range files {
			fp[i] = p.Path()
		}
		p2.filePaths = fp
	}
}

func NewPar2Cmdline(path string, p2o ...Par2CmdlineOption) Repairer {
	p2 := &Par2Cmdline{
		args:         []string{"--"},
		filePaths:    make([]string, 0),
		timeout:      time.Minute * 5,
		renamedFiles: make(map[string]string),
	}
	for _, o := range p2o {
		o(p2)
	}

	args := make([]string, 0, len(p2.args)+len(p2.filePaths)+1)
	args = append(args, p2.args...)
	args = append(args, p2.par2FilePath)
	args = append(args, p2.filePaths...)
	p2.cmd = exec.Command(path, args...)

	return p2
}

func (p2 *Par2Cmdline) handleStdout(b []byte) {
	// TODO(negz): Log output.
	for _, l := range lineTable {
		if l.Match(b) {
			s := l.Submatches(b)
			switch l.linetype {
			case version:
				p2.version = s[1]
			case numFiles:
				p2.numFiles = util.DefaultAtoi(s[1], 0)
			case repairComplete:
				p2.repaired = true
			case repairNotPossible:
				p2.blocksNeeded = util.DefaultAtoi(s[1], 0)
			case fileRenamed:
				p2.renamedFiles[s[1]] = s[2]
			}
		}
	}
}

func (p2 *Par2Cmdline) Repair() error {
	if p2.par2FilePath == "" {
		return errors.New("no par2 file path supplied")
	}
	if len(p2.filePaths) == 0 {
		return errors.New("no file paths supplied")
	}
	d := deputy.Deputy{
		Errors:    deputy.FromStderr,
		StdoutLog: p2.handleStdout,
		Timeout:   p2.timeout,
	}
	if err := d.Run(p2.cmd); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			// Exit errors are expected when we can't recover the file, but we
			// set repaired to reflect that.
			return nil
		}
		return err
	}
	return nil
}

func (p2 *Par2Cmdline) Repaired() bool {
	return p2.repaired
}

func (p2 *Par2Cmdline) RenamedFiles() map[string]string {
	return p2.renamedFiles
}

func (p2 *Par2Cmdline) BlocksNeeded() int {
	return p2.blocksNeeded
}
