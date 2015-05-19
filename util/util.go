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

func StringCRCSum(c uint32) string {
    return strconv.FormatUint(uint64(c), 16)
}
