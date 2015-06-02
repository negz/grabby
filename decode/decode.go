package decode

import "github.com/negz/grabby/decode/yenc"

// IsDecodeError returns true if error e was recorded while decoding a segment.
func IsDecodeError(e error) bool {
	switch e.(type) {
	case yenc.DecodeError:
		return true
	}
	return false
}
