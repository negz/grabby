package grabber

import (
	"bytes"
	"io"
	"log"
	"math/rand"
	"regexp"
	"testing"
	"time"

	"github.com/negz/grabby/decode/yenc"
	"github.com/negz/grabby/nntp"
	"github.com/negz/grabby/nzb"
)

type fakeOut struct {
	*bytes.Buffer
}

func (ff *fakeOut) Close() error {
	return nil
}

type erroreyDecoder struct {
}

func (ed *erroreyDecoder) Write(b []byte) (int, error) {
	return 3, yenc.DecodeError("What even is this encoding!?")
}

func createFakeErroreyDecoder(w io.Writer) io.Writer {
	if rand.Int()%10 == 1 {
		return &erroreyDecoder{}
	}
	return w
}

func createFakeOut(g Grabberer, s Segmenter) (io.WriteCloser, error) {
	return &fakeOut{Buffer: new(bytes.Buffer)}, nil
}

var grabberTests = []struct {
	servers       []nntp.Serverer
	options       [][]ServerOption
	f             string
	filters       []*regexp.Regexp
	metadata      int
	files         int
	par2Files     int
	filteredFiles int
	pausedFiles   int
	name          string
}{
	{
		servers:       []nntp.Serverer{newFakeNNTPServer("nntp1.fake:119", 5)},
		options:       [][]ServerOption{[]ServerOption{Retention(1000 * Day), MustBeInGroup()}},
		f:             "testdata/ubuntu-14.04.2-desktop-amd64.nzb",
		filters:       []*regexp.Regexp{regexp.MustCompile(`(?i).+\.(sfv|nfo|nzb).*`)},
		metadata:      2,
		files:         115,
		par2Files:     15,
		filteredFiles: 2,
		pausedFiles:   17,
		name:          "ubuntu-14.04.2-desktop-amd64",
	},
	{
		servers:     []nntp.Serverer{newFakeNNTPServer("nntp1.fake:119", 30)},
		options:     [][]ServerOption{[]ServerOption{Retention(1000 * Day)}},
		f:           "testdata/ubuntu-14.04.2-desktop-amd64.nzb",
		metadata:    2,
		files:       115,
		par2Files:   15,
		pausedFiles: 15,
		name:        "ubuntu-14.04.2-desktop-amd64",
	},
	{
		servers: []nntp.Serverer{
			newFakeNNTPServer("nntp1.fake:119", 20),
			newFakeNNTPServer("nntp2.fake:119", 10),
			newFakeNNTPServer("nntp2.fake:119", 5),
		},
		options: [][]ServerOption{
			[]ServerOption{Retention(3 * Day)},
			[]ServerOption{MustBeInGroup()},
			[]ServerOption{Retention(1000 * Day)},
		},
		f:           "testdata/ubuntu-14.04.2-desktop-amd64.nzb",
		metadata:    2,
		files:       115,
		par2Files:   15,
		pausedFiles: 15,
		name:        "ubuntu-14.04.2-desktop-amd64",
	},
}

func GrabberDone(g Grabberer) bool {
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-tick.C:
			return false
		case <-g.PostProcessable():
			return true
		}
	}
}

func TestGrabber(t *testing.T) {
	t.Parallel()
	for _, tt := range grabberTests {
		servers := make([]Serverer, 0, len(tt.servers))
		for i, ns := range tt.servers {
			s, err := NewServer(ns, ns.Address(), tt.options[i]...)
			if err != nil {
				t.Fatalf("NewServer(%#v, %#v, %#v): %v", ns, ns.Address(), tt.options[i], err)
			}
			servers = append(servers, s)
		}
		ss, err := NewStrategy(servers)
		if err != nil {
			t.Fatalf("NewStrategy(%+v): %v", servers, err)
		}

		n, err := nzb.NewFromFile(tt.f)
		if err != nil {
			t.Fatalf("nzb.NewFromFile(%v): %v", tt.f, err)
			continue
		}

		g, err := New(
			"/tmp",
			ss,
			FromNZB(n, tt.filters...),
			Decoder(createFakeErroreyDecoder),
			SegmentFileCreator(createFakeOut),
		)

		if g.Name() != tt.name {
			t.Errorf("g.Name == %v, wanted %v", g.Name(), tt.name)
		}

		if len(g.Metadata()) != tt.metadata {
			t.Errorf("%v len(g.Metadata) == %v, wanted %v", g.Name(), len(g.Metadata()), tt.metadata)
		}
		if len(g.Files()) != tt.files {
			t.Errorf("%v len(g.Files) == %v, wanted %v", g.Name(), len(g.Files()), tt.files)
		}

		par2Files, filteredFiles, pausedFiles := 0, 0, 0
		for _, f := range g.Files() {
			if f.IsPar2() {
				par2Files++
			}
			if f.IsFiltered() {
				filteredFiles++
			}
			if f.State() == Paused {
				pausedFiles++
			}
		}

		if par2Files != tt.par2Files {
			t.Errorf("%v par2Files == %v, wanted %v", g.Name(), par2Files, tt.par2Files)
		}
		if filteredFiles != tt.filteredFiles {
			t.Errorf("%v filteredFiles == %v, wanted %v", g.Name(), filteredFiles, tt.filteredFiles)
		}
		if pausedFiles != tt.pausedFiles {
			t.Errorf("%v pausedFiles == %v, wanted %v", g.Name(), pausedFiles, tt.pausedFiles)
		}

		g.HandleGrabs()

		log.Printf("GrabAll()")
		if err := g.GrabAll(); err != nil {
			t.Errorf("%v g.GrabAll(): %v", g.Name(), err)
		}

		log.Printf("Pause()")
		if err := g.Pause(); err != nil {
			t.Errorf("%v g.Pause(): %v", g.Name(), err)
		}

		//TODO(negz): Debug sporadic resume bug. Smells like fighting mutexes.
		log.Printf("Resume()")
		if err := g.Resume(); err != nil {
			t.Errorf("%v g.Resume(): %v", g.Name(), err)
		}

		if !GrabberDone(g) {
			t.Errorf("%v timed out waiting to become postprocessable", g.Name())
			g.Shutdown(nil)
			for _, f := range g.Files() {
				t.Logf("File state: %v", f.State())
			}
			continue
		}
		log.Printf("Became postprocessable.")

		log.Printf("Grabbing par2 files")
		for _, f := range g.Files() {
			if f.IsPar2() {
				if err := g.GrabFile(f); err != nil {
					t.Errorf("%v g.GrabFile(%v): %v", g.Name(), f, err)
				}
			}
		}

		if !GrabberDone(g) {
			t.Errorf("%v timed out waiting to become postprocessable after requesting additional par2 files", g.Name())
		}
		log.Printf("Became postprocessable after requesting par2 files.")

		g.Shutdown(nil)
	}
}
