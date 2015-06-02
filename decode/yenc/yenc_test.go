package yenc

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"testing"
)

var decodeTests = []struct {
	plain     string
	yenc      string
	name      string
	multipart bool
	err       string
}{
	{
		plain: "testdata/singlepart",
		yenc:  "testdata/singlepart.yenc",
		name:  "testfile.txt",
	},
	{
		plain: "testdata/singlepart",
		yenc:  "testdata/singlepart.noheaders.yenc",
		name:  "testfile.txt",
	},
	{
		plain:     "testdata/multipart.part1",
		yenc:      "testdata/multipart.part1.yenc",
		name:      "joystick.jpg",
		multipart: true,
	},
	{
		plain:     "testdata/multipart.part2",
		yenc:      "testdata/multipart.part2.yenc",
		name:      "joystick.jpg",
		multipart: true,
	},
	{
		plain: "testdata/singlepart",
		yenc:  "testdata/singlepart.longheaders.yenc",
		err:   "no yEnc header found in first 1024 bytes",
	},
	{
		plain:     "testdata/multipart.part1",
		yenc:      "testdata/multipart.part1.badcrc.yenc",
		err:       "invalid part checksum bfae5c0c - wanted bfae5c0b",
		multipart: true,
	},
	{
		plain: "testdata/singlepart",
		yenc:  "testdata/singlepart.badcrc.yenc",
		err:   "invalid checksum ded29f4e - wanted ded29f4f",
	},
	{
		plain:     "testdata/multipart.part1",
		yenc:      "testdata/multipart.part1.malformedheader.yenc",
		err:       "malformed yEnc part header: =ypart began=1 ended=11250",
		multipart: true,
	},
	{
		plain:     "testdata/multipart.part2",
		yenc:      "testdata/multipart.part2.nopartheader.yenc",
		err:       "no yEnc part header immediately followed multipart header",
		multipart: true,
	},
}

func TestDecode(t *testing.T) {
	for _, tt := range decodeTests {
		y, err := ioutil.ReadFile(tt.yenc)
		if err != nil {
			t.Errorf("error opening test data %v: %v", tt.yenc, err)
			continue
		}
		p, err := ioutil.ReadFile(tt.plain)
		if err != nil {
			t.Errorf("error opening test data %v: %v", tt.plain, err)
			continue
		}

		b := new(bytes.Buffer)
		d := NewDecoder(b)
		_, err = d.Write(y)

		if tt.err != "" {
			_, ok := err.(DecodeError)
			if !ok {
				t.Errorf("d.Write(%v) == %v, wanted DecodeError", tt.yenc, err)
			}
			if fmt.Sprintf("%v", err) != tt.err {
				t.Errorf("err == '%v', wanted '%v'", err, tt.err)
			}
			continue
		}
		if string(p) != b.String() {
			t.Errorf("d.Write(%v) != %v", tt.yenc, tt.plain)
		}

		dd, ok := d.(*Decoder)
		if !ok {
			t.Errorf("d.(*Decoder) != ok")
		}
		if dd.header.name != tt.name {
			t.Errorf("d.name() == %v, wanted %v", dd.header.name, tt.name)
		}
		if dd.header.multipart != tt.multipart {
			t.Errorf("%v d.Header.multipart == %v, wanted %v", tt.yenc, dd.header.multipart, tt.multipart)
		}
	}
}

/*
 This rudimentary benchmark indicates that using precomputed decode maps doesn't
 actually give much of a speedup.
*/
func BenchmarkDecodemultipart(b *testing.B) {
	bb := decodeTests[2]
	y, err := ioutil.ReadFile(bb.yenc)
	if err != nil {
		b.Fatalf("error opening test data %v: %v", bb.yenc, err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b := new(bytes.Buffer)
		d := NewDecoder(b)
		_, err = d.Write(y)
	}
}
