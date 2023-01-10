package game

import (
	wsv1 "github.com/dechristopher/lio/proto"
)

// OctadGameConfig is used to configure a new game
type OctadGameConfig struct {
	White   string        `json:"w"` // white userid
	Black   string        `json:"b"` // black userid
	Variant *wsv1.Variant // octad variant
	OFEN    string        `json:"o"` // initial ofen
}
