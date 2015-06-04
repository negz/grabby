package grabber

import "github.com/negz/grabby/nzb"

type Metadata struct {
	*nzb.Metadata
	g *Grabber
}
