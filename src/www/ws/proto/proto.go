package proto

// PayloadTag defines the message payload data type
type PayloadTag string

const (
	// OFENTag is the message type tag for the OFENPayload
	OFENTag PayloadTag = "o"
	// MoveTag is the message type tag for the MovePayload
	MoveTag PayloadTag = "m"
	// MoveAckTag is the message type tag for the MoveAckPayload
	MoveAckTag PayloadTag = "a"
	// CrowdTag is the message type tag for the CrowdPayload
	CrowdTag PayloadTag = "c"
	// GameOverTag is the message type tag for the GameOverPayload
	GameOverTag PayloadTag = "g"
)

// Message represents our websocket protocol messages container
type Message struct {
	Tag          string      `json:"t"`            // message type tag
	Data         interface{} `json:"d"`            // data payload
	Version      int         `json:"v,omitempty"`  // data payload version for series
	ProtoVersion int         `json:"pv,omitempty"` // protocol version for data type
}

// OFENPayloadVersion represents the current proto version of the OFENPayload
const OFENPayloadVersion = 1

// OFENPayload contains a full board state and is sent to
// spectators after each move to update game boards
type OFENPayload struct {
	OFEN       string `json:"o"` // OFEN (position, toMove)
	LastMove   string `json:"l"` // last move played in UOI notation
	BlackClock string `json:"b"` // black clock in seconds
	WhiteClock string `json:"w"` // white clock in seconds
	GameID     string `json:"i"` // game id for routing message to board
}

// MovePayloadVersion represents the current proto version of the MovePayload
const MovePayloadVersion = 1

// MovePayload contains all data necessary to represent a single
// move during a live game and update game ui accordingly
type MovePayload struct {
	Clock      ClockPayload        `json:"c,omitempty"` // clock state
	OFEN       string              `json:"o,omitempty"` // (position, toMove)
	SAN        string              `json:"s,omitempty"`
	UOI        string              `json:"u,omitempty"`
	MoveNum    int                 `json:"n,omitempty"`
	Check      bool                `json:"k,omitempty"`
	Moves      []string            `json:"m,omitempty"`
	ValidMoves map[string][]string `json:"v,omitempty"`
	Latency    int                 `json:"l,omitempty"` // player latency indicator
	Ack        int                 `json:"a,omitempty"` // move ack from player
}

// MessageMove contains a MovePayload message
type MessageMove struct {
	Tag          string      `json:"t"`            // message type tag
	Data         MovePayload `json:"d"`            // move data payload
	Version      int         `json:"v,omitempty"`  // data payload version for series
	ProtoVersion int         `json:"pv,omitempty"` // protocol version for data type
}

// MoveAckPayload is the move number acknowledgement
type MoveAckPayload int

// ClockPayload is a wire representation of the current state of a game's clock
type ClockPayload struct {
	Black int64 `json:"b"` // black clock in centi-seconds
	White int64 `json:"w"` // white clock in centi-seconds
	Lag   int   `json:"l"` // internal server lag in ms
}

// CrowdPayload contains data about connected players and spectator count
type CrowdPayload struct {
	Black bool `json:"b"`
	White bool `json:"w"`
	Spec  int  `json:"s,omitempty"`
}

// GameOverPayload contains data regarding the outcome of the game
type GameOverPayload struct {
	Winner   string       `json:"w"`
	StatusID int          `json:"i"`
	Status   string       `json:"s"`
	Clock    ClockPayload `json:"c,omitempty"`
}
