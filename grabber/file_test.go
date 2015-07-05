package grabber

import (
	"time"

	"github.com/negz/grabby/magic"
)

type fakeFile struct {
	*fakeFSM
	g []string
	d int
	t magic.FileType
	s []Segmenter
}

func (ff *fakeFile) Grabber() Grabberer {
	return nil
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

func (ff *fakeFile) FileType() magic.FileType {
	return ff.t
}

func (ff *fakeFile) SetFileType(t magic.FileType) {
	ff.t = t
}
