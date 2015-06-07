package grabber

import "github.com/negz/grabby/nzb"

type Metadataer interface {
	Type() string
	Value() string
}

type Metadata struct {
	nm *nzb.Metadata
	g  *Grabber
}

func NewMetadata(nm *nzb.Metadata, g *Grabber) Metadataer {
	return &Metadata{nm, g}
}

func (m *Metadata) Type() string {
	return m.nm.Type
}

func (m *Metadata) Value() string {
	return m.nm.Value
}
