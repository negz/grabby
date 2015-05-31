package util

import (
	"fmt"
	"hash/fnv"
	"io/ioutil"
)

// PasswordFromFile reads a password from a file.
func PasswordFromFile(f string) (string, error) {
	password, err := ioutil.ReadFile(f)
	if err != nil {
		return "", err
	}
	return string(password), nil
}

// FormatArticleID returns supplied ID id wrapped with <>
func FormatArticleID(id string) string {
	return fmt.Sprintf("<%s>", id)
}

// HashBytes returns the FNV-1a 64 bit hex hash for the supplied byte array.
func HashBytes(b []byte) string {
	h := fnv.New64a()
	h.Write(b)
	return fmt.Sprintf("%016x", h.Sum64())
}

// HashString returns the FNV-1a 64 bit hex hash for the supplied string.
func HashString(s string) string {
	return HashBytes([]byte(s))
}

// ValidCRCString handles the fact that yEnc is terribad and doesn't actually
// specify how to present the CRC32 sum in the trailer.
func ValidCRCString(c uint32) map[string]bool {
	return map[string]bool{
		fmt.Sprintf("%x", c):   true,
		fmt.Sprintf("%X", c):   true,
		fmt.Sprintf("%08x", c): true,
		fmt.Sprintf("%08X", c): true,
	}
}

// UpdateDownloadRate updates a supplied bytes-per-second download rate based on
// the provided bytes downloaded in seconds time. It uses an exponentially
// weighted moving average to decay the influence of older rates on the average.
func UpdateDownloadRate(decay, rate, seconds float64, bytes int64) float64 {
	if rate == 0 {
		return float64(bytes) / seconds
	}
	return ((float64(bytes) / seconds) * decay) + (rate * (1 - decay))
}
