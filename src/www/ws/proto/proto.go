package proto

// Command enums for websocket messages
const (
	CommandError   = -1
	CommandGoodbye = 0
	CommandHello   = 1
	CommandGame    = 2
)

// Message is a struct representing a websocket control message
type Message struct {
	Time      int64    `json:"t"`
	Channel   string   `json:"ch"`
	Command   int      `json:"c"`
	Body      []string `json:"b"`
	UserID    string   `json:"u,omitempty"`
	BrowserID int      `json:"i,omitempty"`
	GameID    string   `json:"g,omitempty"`
	Error     string   `json:"e,omitempty"`
}
