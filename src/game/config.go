package game

import "github.com/dechristopher/lioctad/clock"

// OctadGameConfig is used to configure a new game
type OctadGameConfig struct {
	White   string            `json:"w"` // white userid
	Black   string            `json:"b"` // black userid
	Control clock.TimeControl // time control
	OFEN    string            `json:"o"` // initial ofen
	Channel string            `json:"c"` // ws game channel
}
