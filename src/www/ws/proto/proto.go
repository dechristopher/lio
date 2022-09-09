package proto

import (
	"encoding/json"

	"github.com/dechristopher/lio/clock"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

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
	// RoomTag is the message type tag for the RoomMessage
	RoomTag PayloadTag = "r"
	// RedirectTag is the message type tag for the RedirectMessage
	RedirectTag PayloadTag = "e"
)

// Message represents our websocket protocol messages container
type Message struct {
	Tag          string      `json:"t"`            // message type tag
	Data         interface{} `json:"d"`            // data payload
	Version      int         `json:"v,omitempty"`  // data payload version for series
	ProtoVersion int         `json:"pv,omitempty"` // protocol version for data type
}

// PingMessage is used to determine socket latency to server
type PingMessage struct {
	Ping string `json:"pi"`
}

// PongMessage is the response to the PingMessage
type PongMessage struct {
	Pong string `json:"po"`
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

type ScorePayload map[string]float64

// MovePayloadVersion represents the current proto version of the MovePayload
const MovePayloadVersion = 2

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
	Latency    clock.CTime         `json:"l,omitempty"`  // player latency indicator
	Ack        int                 `json:"a,omitempty"`  // move ack from player
	White      string              `json:"w,omitempty"`  // white player id
	Black      string              `json:"b,omitempty"`  // black player id
	Score      ScorePayload        `json:"sc,omitempty"` // match score
	GameStart  bool                `json:"gs,omitempty"`
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
	Control int64 `json:"tc"` // time control total time
	Black   int64 `json:"b"`  // black clock in centi-seconds
	White   int64 `json:"w"`  // white clock in centi-seconds
	Lag     int64 `json:"l"`  // internal server lag in ms
}

// CrowdPayload contains data about connected players and spectator count
type CrowdPayload struct {
	Black bool `json:"b"`
	White bool `json:"w"`
	Spec  int  `json:"s,omitempty"`
}

// GameOverPayload contains data regarding the outcome of the game
type GameOverPayload struct {
	Winner   string       `json:"w,omitempty"`
	StatusID int          `json:"i,omitempty"`
	Status   string       `json:"s"`
	Clock    ClockPayload `json:"c,omitempty"`
	Score    ScorePayload `json:"sc,omitempty"`
	RoomOver bool         `json:"o,omitempty"`
}

// RoomMessage contains room state data
type RoomMessage struct {
	RoomID  string `json:"id,omitempty"`
	Query   bool   `json:"q,omitempty"`
	Ready   bool   `json:"r,omitempty"`
	P1Score int    `json:"p1,omitempty"`
	P2Score int    `json:"p2,omitempty"`
}

// RedirectMessage instructs the client to redirect to a different page
// optionally displaying an intermediate message in a modal
type RedirectMessage struct {
	Message  string `json:"m,omitempty"`
	Location string `json:"l"`
}

// Marshal encodes the given message and payload into JSON
func (m *Message) Marshal() ([]byte, error) {
	return json.Marshal(&m)
}

// Please will return the marshaled text as a byte[], hoping it doesn't fail
func (m *Message) Please() []byte {
	b, err := m.Marshal()
	if err != nil {
		util.Error(str.CProt, str.EProtoMarshal, err)
		// we've got problems if these messages fail to marshal
		panic(err)
	}

	return b
}
