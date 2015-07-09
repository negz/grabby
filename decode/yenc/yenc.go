/*
Package yenc implements yEnc 1.3 decoding.
Note that while it is multipart aware it doesn't handle assembling parts, or
their relation to each other in general. This is expected to be handled by an
NZB file.
*/
package yenc

import (
	"bytes"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"strconv"

	"github.com/negz/grabby/util"
)

// maxheaderBuffer specifies how many bytes we should read before giving up on
// receiving a yEnc header.
const maxheaderBuffer int = 1024

// A DecodeError records an error decoding data.
type DecodeError string

func (err DecodeError) Error() string {
	return string(err)
}

// A header represents a yEnc header.
type header struct {
	multipart bool
	part      int // All int because they're derived from strconv.Atoi()
	total     int
	line      int
	size      int
	name      string
}

// A partHeader represents a yEnc part header for multipart binaries.
type partHeader struct {
	Begin int
	End   int
}

// A trailer represents a yEnc part trailer.
type trailer struct {
	size      int
	crc32     string
	partCRC32 string
}

// A Decoder wraps an io.Writer, decoding yEnc as it writes.
// A decoder handles exactly one yEnc file (or one part of a multipart file).
type Decoder struct {
	w                  io.Writer
	b                  *bytes.Buffer
	nextByteIsCritical bool
	crc                hash.Hash32
	header             *header
	partHeader         *partHeader
	trailer            *trailer
	bytesBeforeheader  int
	decodeMap          [256]byte
	criticalDecodeMap  [256]byte
	scanErr            error
	scanLine           []byte
}

// NewDecoder returns an io.Writer that decodes the supplied io.Writer.
func NewDecoder(w io.Writer) io.Writer {
	d := &Decoder{w: w, b: new(bytes.Buffer), crc: crc32.NewIEEE()}
	for i := 0; i < len(d.decodeMap); i++ {
		d.decodeMap[i] = byte((i - 42) & 255)
	}
	for i := 0; i < len(d.criticalDecodeMap); i++ {
		d.criticalDecodeMap[i] = byte((d.decodeMap[i] - 64) & 255)
	}
	return d
}

// headerStringToMap converts a yEnc header (or trailer) to a string map of key
// value pairs.
func headerBytesToMap(h []byte) (map[string]string, error) {
	m := make(map[string]string)

	if bytes.IndexByte(h, ' ') == -1 {
		return m, DecodeError(fmt.Sprintf("malformed yEnc header: %v", h))
	}

	for _, element := range bytes.Split(h, []byte(" ")) {
		if bytes.IndexByte(element, '=') == -1 {
			return m, DecodeError(fmt.Sprintf("malformed yEnc header: %v", h))
		}

		kv := bytes.SplitN(element, []byte("="), 2)
		if len(kv) != 2 {
			// Probably the beginning of the header (=ybegin, =ypart, =yend)
			continue
		}
		m[string(kv[0])] = string(kv[1])
	}

	return m, nil
}

// parseheader parses a yEnc header if one exists in the buffer.
func parseheader(b []byte) (*header, error) {
	headerMap, err := headerBytesToMap(b)
	header := &header{name: headerMap["name"]}

	if header.line, err = strconv.Atoi(headerMap["line"]); err != nil {
		return nil, DecodeError(fmt.Sprintf("malformed yEnc header: %v", string(b)))
	}
	if header.size, err = strconv.Atoi(headerMap["size"]); err != nil {
		return nil, DecodeError(fmt.Sprintf("malformed yEnc header: %v", string(b)))
	}

	// There's nothing more to do if we're not a multipart header
	if headerMap["part"] == "" {
		return header, nil
	}
	if header.part, err = strconv.Atoi(headerMap["part"]); err != nil {
		return nil, DecodeError(fmt.Sprintf("malformed yEnc header: %v", string(b)))
	}

	header.multipart = true

	// total is not required by yEnc 1.1
	if headerMap["total"] == "" {
		return header, nil
	}
	if header.total, err = strconv.Atoi(headerMap["total"]); err != nil {
		return nil, DecodeError(fmt.Sprintf("malformed yEnc header: %v", string(b)))
	}

	return header, nil
}

// parsepartHeader parses a yEnc part header if one exists in the buffer.
func parsepartHeader(b []byte) (*partHeader, error) {
	headerMap, err := headerBytesToMap(b)
	header := &partHeader{}
	if header.Begin, err = strconv.Atoi(headerMap["begin"]); err != nil {
		return nil, DecodeError(fmt.Sprintf("malformed yEnc part header: %v", string(b)))
	}
	if header.End, err = strconv.Atoi(headerMap["end"]); err != nil {
		return nil, DecodeError(fmt.Sprintf("malformed yEnc part header: %v", string(b)))
	}

	return header, nil
}

// parsetrailer parses a yEnc part trailer if one exists in the buffer.
func parsetrailer(b []byte) (*trailer, error) {
	trailerMap, err := headerBytesToMap(b)
	trailer := &trailer{}
	if trailer.size, err = strconv.Atoi(trailerMap["size"]); err != nil {
		return nil, DecodeError(fmt.Sprintf("malformed yEnc part trailer: %v", string(b)))
	}

	// CRCs are optional
	trailer.crc32 = trailerMap["crc32"]
	trailer.partCRC32 = trailerMap["pcrc32"]

	return trailer, nil
}

// verifycrc32s verifies any crc32s found in the trailer.
// Note that if a pcrc32 (i.e. the crc32 of this part) is present we ignore
// crc32  as we don't care about the file's larger context.
func (d *Decoder) verifycrc32s() error {
	switch {
	case d.trailer.partCRC32 != "":
		if !util.ValidCRCString(d.crc.Sum32())[d.trailer.partCRC32] {
			return DecodeError(fmt.Sprintf("invalid part checksum %v - wanted %x", d.trailer.partCRC32, d.crc.Sum32()))
		}
	case d.trailer.crc32 != "":
		if !util.ValidCRCString(d.crc.Sum32())[d.trailer.crc32] {
			return DecodeError(fmt.Sprintf("invalid checksum %v - wanted %x", d.trailer.crc32, d.crc.Sum32()))
		}
	}
	return nil
}

// decodeline decodes a single line of yEnc data, writing it to the output
// writer. Output is also written to the crc32 hash for future verification.
func (d *Decoder) decodeline(line []byte) error {
	p := 0
	for _, b := range line {
		switch {
		case d.nextByteIsCritical:
			line[p] = d.criticalDecodeMap[b]
			d.nextByteIsCritical = false
			p++
		case b == '=':
			d.nextByteIsCritical = true
		default:
			line[p] = d.decodeMap[b]
			p++
		}
	}
	if _, err := d.w.Write(line[:p]); err != nil {
		return err
	}
	if _, err := d.crc.Write(line[:p]); err != nil {
		return err
	}
	return nil
}

func (d *Decoder) scan() bool {
	if bytes.IndexByte(d.b.Bytes(), '\n') == -1 {
		return false
	}
	if d.scanLine, d.scanErr = d.b.ReadBytes('\n'); d.scanErr != nil {
		return false
	}
	return true
}

// Decode decodes yEnc data as it is written to the Decoder's buffer.
func (d *Decoder) yDecode() error {
	var err error
	for d.scan() {
		switch {
		case d.header == nil:
			if d.bytesBeforeheader >= maxheaderBuffer {
				return DecodeError(fmt.Sprintf("no yEnc header found in first %v bytes", maxheaderBuffer))
			}
			if string(d.scanLine[:8]) != "=ybegin " {
				d.bytesBeforeheader += len(d.scanLine)
				continue
			}
			d.scanLine = bytes.TrimRight(d.scanLine, "\r\n")
			if d.header, err = parseheader(d.scanLine); err != nil {
				return err
			}
		case d.header.multipart && (d.partHeader == nil):
			if string(d.scanLine[:7]) != "=ypart " {
				return DecodeError("no yEnc part header immediately followed multipart header")
			}
			d.scanLine = bytes.TrimRight(d.scanLine, "\r\n")
			if d.partHeader, err = parsepartHeader(d.scanLine); err != nil {
				return err
			}
		default:
			d.scanLine = bytes.TrimRight(d.scanLine, "\r\n")
			if string(d.scanLine[:6]) == "=yend " {
				if d.trailer, err = parsetrailer(d.scanLine); err != nil {
					return err
				}
				return d.verifycrc32s()
			}
			if err = d.decodeline(d.scanLine); err != nil {
				return err
			}
		}
	}
	return d.scanErr
}

// Write wraps the Write() of the embedded io.Writer, decoding along the way.
func (d *Decoder) Write(p []byte) (int, error) {
	d.b.Write(p)
	if err := d.yDecode(); err != nil {
		return 0, err
	}
	return len(p), nil
}
