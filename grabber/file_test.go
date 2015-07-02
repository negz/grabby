package grabber

import (
	"testing"
	"time"

	"github.com/negz/grabby/magic"
	"github.com/negz/grabby/nzb"
)

type fakeFile struct {
	*fakeFSM
	g []string
	d int
	t magic.FileType
	s []Segmenter
}

func (ff *fakeFile) Subject() string {
	return "yEnc: Here guys I yEncoded some dickbutts for you [1/1]"
}

func (ff *fakeFile) Hash() string {
	return "SUCHHASH"
}

func (ff *fakeFile) Poster() string {
	return "Dick T. Butt"
}

func (ff *fakeFile) Posted() time.Time {
	return postDate
}

func (ff *fakeFile) Groups() []string {
	return ff.g
}

func (ff *fakeFile) Segments() []Segmenter {
	return ff.s
}

func (ff *fakeFile) SortSegments() {
	// TOTALLY SORTING YOUR SEGMENTS RIGHT NOW!
}

func (ff *fakeFile) Filename() string {
	return "dickbutt.rar"
}

func (ff *fakeFile) IsPar2() bool {
	return false
}

func (ff *fakeFile) IsFiltered() bool {
	return false
}

func (ff *fakeFile) IsRequired() bool {
	return true
}

func (ff *fakeFile) SegmentDone() {
	ff.d++
}

func (ff *fakeFile) SetFileType(t magic.FileType) {
	ff.t = t
}

func TestSmallestFile(t *testing.T) {
	t.Parallel()

	f1, f2 := &fakeFile{}, &fakeFile{}

	f1s1 := NewSegment(&nzb.Segment{Number: 1, ArticleID: "dick@butts$"}, f1)
	f1.s = append(f1.s, f1s1)

	f2s1 := NewSegment(&nzb.Segment{Number: 1, ArticleID: "dick@butts$"}, f2)
	f2s2 := NewSegment(&nzb.Segment{Number: 2, ArticleID: "dick@butts$"}, f2)
	f2.s = append(f2.s, f2s1, f2s2)

	if smallestFile(nil, f1) != f1 {
		t.Errorf("smallestFile(%v, %v) == %v, want %v", nil, f1, smallestFile(nil, f1), f1)
	}
	if smallestFile(f2, f1) != f1 {
		t.Errorf("smallestFile(%v, %v) == %v, want %v", f2, f1, smallestFile(f2, f1), f2)
	}
	if smallestFile(f1, f2) != f1 {
		t.Errorf("smallestFile(%v, %v) == %v, want %v", f1, f2, smallestFile(f1, f2), f1)
	}
}
