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
			t.Errorf("error opening test data %v", tt.yenc)
			continue
		}
		p, err := ioutil.ReadFile(tt.plain)
		if err != nil {
			t.Errorf("error opening test data %v", tt.plain)
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
		if d.Header.Name != tt.name {
			t.Errorf("d.Name() == %v, wanted %v", d.Header.Name, tt.name)
		}
		if d.Header.Multipart != tt.multipart {
			t.Errorf("%v d.Header.Multipart == %v, wanted %v", tt.yenc, d.Header.Multipart, tt.multipart)
		}
	}
}
