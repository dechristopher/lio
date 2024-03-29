package game

import (
	"github.com/dechristopher/lio/variant"
)

// OctadGameConfig is used to configure a new game
type OctadGameConfig struct {
	White   string          `json:"w"` // white userid
	Black   string          `json:"b"` // black userid
	Variant variant.Variant // octad variant
	OFEN    string          `json:"o"` // initial ofen
}
