// Package yenc implements yEnc 1.3 decoding.
// Note that while it is multipart aware it doesn't handle assembling parts,
// or their relation to each other in general. This is expected to be handled
// by an NZB file.
package yenc

import (
	"bytes"
	"fmt"
	"github.com/negz/grabby/util"
	"hash"
	"hash/crc32"
	"io"
	"strconv"
)

// maxHeaderBuffer specifies how many bytes we should read before giving up on receiving a yEnc header.
const maxHeaderBuffer int = 1024

// DecodeError records an error decoding yEnc encoded data.
type DecodeError string

func (err DecodeError) Error() string {
	return string(err)
}

// Header represents a yEnc header.
type Header struct {
	Multipart bool
	Part      int // All int because they're derived from strconv.Atoi()
	Total     int
	Line      int
	Size      int
	Name      string
}

// PartHeader represents a yEnc part header for multipart binaries.
type PartHeader struct {
	Begin int
	End   int
}

// Trailer represents a yEnc part trailer.
type Trailer struct {
	Size      int
	CRC32     string
	PartCRC32 string
}

// Decoder wraps an io.Writer, decoding yEnc as it writes.
// A decoder handles exactly one yEnc file (or one part of a multipart file).
type Decoder struct {
	w                  io.Writer
	b                  *bytes.Buffer
	nextByteIsCritical bool
	crc                hash.Hash32
	Header             *Header
	PartHeader         *PartHeader
	Trailer            *Trailer
	bytesBeforeHeader  int
	decodeMap          [256]byte
	criticalDecodeMap  [256]byte
}

// NewDecoder returns a Decoder wrapping the supplied io.Writer.
func NewDecoder(w io.Writer) *Decoder {
	d := &Decoder{w: w, b: new(bytes.Buffer), crc: crc32.NewIEEE()}
	for i := 0; i < len(d.decodeMap); i++ {
		d.decodeMap[i] = byte((i - 42) & 255)
	}
	for i := 0; i < len(d.criticalDecodeMap); i++ {
		d.criticalDecodeMap[i] = byte((d.decodeMap[i] - 64) & 255)
	}
	return d
}

// headerStringToMap converts a yEnc header (or trailer) to a string map of key value pairs
func headerBytesToMap(h []byte) (map[string]string, error) {
	m := make(map[string]string)

	if bytes.IndexByte(h, ' ') == -1 {
		return m, DecodeError(fmt.Sprintf("Malformed yEnc header: %v", h))
	}

	for _, element := range bytes.Split(h, []byte(" ")) {
		if bytes.IndexByte(element, '=') == -1 {
			return m, DecodeError(fmt.Sprintf("Malformed yEnc header: %v", h))
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

// parseHeader parses a yEnc header if one exists in the buffer.
func parseHeader(b []byte) (*Header, error) {
	headerMap, err := headerBytesToMap(b)
	header := &Header{Name: headerMap["name"]}

	if header.Line, err = strconv.Atoi(headerMap["line"]); err != nil {
		return nil, DecodeError(fmt.Sprintf("Malformed yEnc header: %v", string(b)))
	}
	if header.Size, err = strconv.Atoi(headerMap["size"]); err != nil {
		return nil, DecodeError(fmt.Sprintf("Malformed yEnc header: %v", string(b)))
	}

	// There's nothing more to do if we're not a multipart header
	if headerMap["part"] == "" {
		return header, nil
	}
	if header.Part, err = strconv.Atoi(headerMap["part"]); err != nil {
		return nil, DecodeError(fmt.Sprintf("Malformed yEnc header: %v", string(b)))
	}

	header.Multipart = true

	// total is not required by yEnc 1.1
	if headerMap["total"] == "" {
		return header, nil
	}
	if header.Total, err = strconv.Atoi(headerMap["total"]); err != nil {
		return nil, DecodeError(fmt.Sprintf("Malformed yEnc header: %v", string(b)))
	}

	return header, nil
}

// parsePartHeader parses a yEnc part header if one exists in the buffer.
func parsePartHeader(b []byte) (*PartHeader, error) {
	headerMap, err := headerBytesToMap(b)
	header := &PartHeader{}
	if header.Begin, err = strconv.Atoi(headerMap["begin"]); err != nil {
		return nil, DecodeError(fmt.Sprintf("Malformed yEnc part header: %v", string(b)))
	}
	if header.End, err = strconv.Atoi(headerMap["end"]); err != nil {
		return nil, DecodeError(fmt.Sprintf("Malformed yEnc part header: %v", string(b)))
	}

	return header, nil
}

// parseTrailer parses a yEnc part trailer if one exists in the buffer.
func parseTrailer(b []byte) (*Trailer, error) {
	trailerMap, err := headerBytesToMap(b)
	trailer := &Trailer{}
	if trailer.Size, err = strconv.Atoi(trailerMap["size"]); err != nil {
		return nil, DecodeError(fmt.Sprintf("Malformed yEnc part trailer: %v", string(b)))
	}

	// CRCs are optional
	trailer.CRC32 = trailerMap["crc32"]
	trailer.PartCRC32 = trailerMap["pcrc32"]

	return trailer, nil
}

// verifyCRC32s verifies any CRC32s found in the trailer.
// Note that if a pcrc32 (i.e. the CRC32 of this part) is present we ignore crc32
// as we don't care about the file's larger context.
func (d *Decoder) verifyCRC32s() error {
	switch {
	case d.Trailer.PartCRC32 != "":
		if !util.ValidCRCString(d.crc.Sum32())[d.Trailer.PartCRC32] {
			return DecodeError(fmt.Sprintf("Invalid part checksum. Got %v, wanted %x.", d.Trailer.PartCRC32, d.crc.Sum32()))
		}
	case d.Trailer.CRC32 != "":
		if !util.ValidCRCString(d.crc.Sum32())[d.Trailer.CRC32] {
			return DecodeError(fmt.Sprintf("Invalid checksum. Got %v, wanted %x.", d.Trailer.CRC32, d.crc.Sum32()))
		}
	}
	return nil
}

// decodeLine decodes a single line of yEnc data, writing it to the output writer.
// Output is also written to the CRC32 hash for future verification.
func (d *Decoder) decodeLine(line []byte) error {
	// TODO(negz): It would be nice to reuse the input line, but we don't write
	// the '=' control character out and I'm too lazy to handle that.
	outLine := make([]byte, 0, len(line))
	for _, b := range line {
		switch {
		case d.nextByteIsCritical:
			outLine = append(outLine, d.criticalDecodeMap[b])
			d.nextByteIsCritical = false
		case b == '=':
			d.nextByteIsCritical = true
		default:
			outLine = append(outLine, d.decodeMap[b])
		}
	}
	if _, err := d.w.Write(outLine); err != nil {
		return err
	}
	if _, err := d.crc.Write(outLine); err != nil {
		return err
	}
	return nil
}

// Decode decodes yEnc data as it is written to the Decoder's buffer.
func (d *Decoder) decode() error {
	for d.Header == nil {
		if d.bytesBeforeHeader >= maxHeaderBuffer {
			return DecodeError(fmt.Sprintf("No yEnc header found in first %v bytes", maxHeaderBuffer))
		}
		if bytes.IndexByte(d.b.Bytes(), '\n') == -1 {
			return nil
		}
		line, err := d.b.ReadBytes('\n')
		if err != nil {
			return err
		}
		d.bytesBeforeHeader += len(line)
		if string(line[:8]) != "=ybegin " {
			continue
		}
		line = bytes.TrimRight(line, "\r\n")
		if d.Header, err = parseHeader(line); err != nil {
			return err
		}
	}

	if d.Header.Multipart && (d.PartHeader == nil) {
		if bytes.IndexByte(d.b.Bytes(), '\n') == -1 {
			return nil
		}
		line, err := d.b.ReadBytes('\n')
		if err != nil {
			return err
		}
		if string(line[:7]) != "=ypart " {
			return DecodeError("No yEnc part header immediately followed multipart header")
		}
		line = bytes.TrimRight(line, "\r\n")
		if d.PartHeader, err = parsePartHeader(line); err != nil {
			return err
		}
	}

	for bytes.IndexByte(d.b.Bytes(), '\n') != -1 {
		line, err := d.b.ReadBytes('\n')
		if err != nil {
			return err
		}
		line = bytes.TrimRight(line, "\r\n")

		if string(line[:6]) == "=yend " {
			if d.Trailer, err = parseTrailer(line); err != nil {
				return err
			}
			if err := d.verifyCRC32s(); err != nil {
				return err
			}
			return nil
		}

		if err := d.decodeLine(line); err != nil {
			return err
		}
	}
	return nil
}

func (d *Decoder) Write(p []byte) (int, error) {
	d.b.Write(p)
	if err := d.decode(); err != nil {
		return 0, err
	}
	return len(p), nil
}
