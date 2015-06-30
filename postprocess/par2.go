package postprocess

import (
	"os"
	"path/filepath"

	"github.com/negz/grabby/grabber"
)

// TODO(negz): Theoretically we could do this in the Grabber to set IsPar2 early
func SmellsLikePar2(wd string, f grabber.Filer) bool {
	if len(f.Segments()) == 0 {
		return false
	}

	sp := filepath.Join(wd, f.Segments()[0].WorkingFilename())
	sf, err := os.Open(sp)
	if err != nil {
		return false
	}
	defer sf.Close()

	b := make([]byte, 8)
	br, err := sf.Read(b)
	if err != nil {
		return false
	}

	return GetMagic(b[:br]) == Par2
}
