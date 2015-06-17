package grabber

import (
	"bytes"
	"testing"
	"time"

	"github.com/negz/grabby/nzb"
)

var postDate time.Time = time.Date(1985, time.October, 2, 0, 0, 0, 0, time.UTC)

var segmentTests = []struct {
	ns  *nzb.Segment
	g   []string
	num int
	id  string
}{
	{
		ns:  &nzb.Segment{Number: 1, ArticleID: "dick@butts$"},
		g:   []string{"alt.bin.dickbutts", "alt.bin.duckbitts"},
		num: 1,
		id:  "dick@butts$",
	},
}

func TestSegment(t *testing.T) {
	t.Parallel()
	for _, tt := range segmentTests {
		f := &fakeFile{fakeFSM: &fakeFSM{s: Pending}, g: tt.g, s: []Segmenter{}}
		s := NewSegment(tt.ns, f)

		if s.ID() != tt.id {
			t.Errorf("s.ID() == %v, want %v", s.ID(), tt.id)
		}
		if s.Number() != tt.num {
			t.Errorf("s.Number() == %v, want %v", s.Number(), tt.num)
		}
		if s.Posted() != postDate {
			t.Errorf("s.Posted() == %v, want %v", s.Posted(), postDate)
		}

		if err := s.Pause(); err != nil {
			t.Errorf("s.Pause(): %v", err)
		}
		if err := s.Pause(); err != nil {
			t.Errorf("s.Pause(): %v", err)
		}
		if err := s.Resume(); err != nil {
			t.Errorf("s.Resume(): %v", err)
		}
		if err := s.Resume(); err != nil {
			t.Errorf("s.Resume(): %v", err)
		}
		if err := s.Working(); err != nil {
			t.Errorf("s.Working(): %v", err)
		}
		if err := s.Working(); err != nil {
			t.Errorf("s.Working(): %v", err)
		}
		s.WriteTo(&fakeOut{Buffer: new(bytes.Buffer)})
		if f.State() != Working {
			t.Errorf("f.State() == %v, want %v", f.State(), Working)
		}
		if err := s.Done(nil); err != nil {
			t.Errorf("s.Done(): %v", err)
		}
		if f.d != 1 {
			t.Errorf("f.d == %v, want 1", f.d)
		}
		if err := s.Working(); err == nil {
			t.Errorf("s.Working(): %v, want %v", err, StateError)
		}
	}
}
