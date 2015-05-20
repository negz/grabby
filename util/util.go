package util

import (
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"strconv"
)

func PasswordFromFile(f string) (string, error) {
	password, err := ioutil.ReadFile(f)
	if err != nil {
		return "", err
	}
	return string(password), nil
}

func FormatArticleID(id string) string {
	return fmt.Sprintf("<%s>", id)
}

func HashBytes(b []byte) string {
	h := fnv.New64a()
	h.Write(b)
	return strconv.FormatUint(h.Sum64(), 16)
}

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
