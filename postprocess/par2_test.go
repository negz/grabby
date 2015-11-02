package postprocess

import (
	"bufio"
	"io"
	"os"
	"reflect"
	"testing"
)

func testPipe(log func([]byte), r io.Reader) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		log(scanner.Bytes())
	}
	return scanner.Err()
}

var par2CmdlineTests = []struct {
	td           string
	version      string
	numFiles     int
	repaired     bool
	blocksNeeded int
	renamed      map[string]string
}{
	{
		td:       "testdata/par2.norepair",
		version:  "0.4",
		numFiles: 98,
		repaired: true,
		renamed:  map[string]string{},
	},
	{
		td:       "testdata/par2.misnamed",
		version:  "0.4",
		numFiles: 98,
		repaired: true,
		renamed: map[string]string{
			"ubuntu-14.04.2- \"-amd64.part001.rar":  "ubuntu-14.04.2-desktop-amd64.part001.rar",
			"ubuntu-14.04.2-desktop-amd64.womp.rar": "ubuntu-14.04.2-desktop-amd64.part095.rar",
		},
	},
	{
		td:           "testdata/par2.repairnotpossible",
		version:      "0.4",
		numFiles:     98,
		repaired:     false,
		blocksNeeded: 90,
		renamed:      map[string]string{},
	},
}

func TestPar2Cmdline(t *testing.T) {
	t.Parallel()
	for _, tt := range par2CmdlineTests {
		f, err := os.Open(tt.td)
		if err != nil {
			t.Errorf("Unable to open %v: %v", tt.td, err)
			continue
		}

		p2r := NewPar2Cmdline("/tmp/fakepar2")
		p2, ok := p2r.(*Par2Cmdline)
		if !ok {
			t.Errorf("p2r.(*Par2Cmdline) != ok")
		}

		if err = testPipe(p2.handleStdout, f); err != nil {
			t.Errorf("testPipe: %v", err)
		}

		if p2.version != tt.version {
			t.Errorf("p2.version == %v, want %v", p2.version, tt.version)
		}
		if p2.numFiles != tt.numFiles {
			t.Errorf("p2.numFiles == %v, want %v", p2.numFiles, tt.numFiles)
		}
		if p2.repaired != tt.repaired {
			t.Errorf("p2.repaired == %v, want %v", p2.repaired, tt.repaired)
		}
		if p2.blocksNeeded != tt.blocksNeeded {
			t.Errorf("p2.blocksNeeded == %v, want %v", p2.blocksNeeded, tt.blocksNeeded)
		}
		if !reflect.DeepEqual(p2.renamedFiles, tt.renamed) {
			t.Errorf("DeepEqual(%#v, %#v) == false", p2.renamedFiles, tt.renamed)
		}
	}
}

var blocksFromFilenameTests = []struct {
	filename string
	blocks   int
}{
	{filename: "my awesome file.vol002+008.par2", blocks: 8},
}

func TestBlocksFromFilename(t *testing.T) {
	t.Parallel()
	for _, tt := range blocksFromFilenameTests {
		if Par2BlocksFromFilename(tt.filename) != tt.blocks {
			t.Errorf("BlocksFromFilename(%v) == %v, want %v", tt.filename, Par2BlocksFromFilename(tt.filename), tt.blocks)
		}
	}

}
