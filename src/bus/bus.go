package bus

import (
	"time"

	ev "github.com/asaskevich/EventBus"
)

// Internal event bus implementation
var bus ev.Bus
var ready = false

// Up brings the event bus online
func Up() {
	bus = ev.New()
	ready = true
}

// Event that's published to the bus by a Publisher
type Event struct {
	Publisher string
	Data      []interface{}
}

// Publisher is a struct representing a
// named publisher to a channel
type Publisher struct {
	Name    string
	Channel Channel
}

// NewPublisher returns a new Publisher instance
// configured for the given Channel
func NewPublisher(name string, channel Channel) *Publisher {
	return &Publisher{
		Name:    name,
		Channel: channel,
	}
}

// Publish an event to the Publisher's configured channel
func (p *Publisher) Publish(data ...interface{}) {
	for ready == false {
		time.Sleep(time.Millisecond * 5)
	}
	bus.Publish(string(p.Channel), Event{
		Publisher: p.Name,
		Data:      data,
	})
}

// Channel is a constant for publishing or
// subscribing to events to the bus
type Channel string

// SystemChannel Predefined bus channels
const (
	SystemChannel Channel = "lio:sys"
)

// Handler function for event listeners
type Handler = func(e Event)

// SubscribeOnce subscribes to the given channel and is removed
// once the given handler function has been executed.
func (c Channel) SubscribeOnce(fn Handler) error {
	for ready == false {
		time.Sleep(time.Millisecond * 5)
	}
	return bus.SubscribeOnce(string(c), fn)
}

// Subscribe subscribes the given handler function to the
// given channel and is removed on process termination
func (c Channel) Subscribe(fn Handler) error {
	for ready == false {
		time.Sleep(time.Millisecond * 5)
	}
	return bus.Subscribe(string(c), fn)
}

// Unsubscribe a given handler from the channel
func (c Channel) Unsubscribe(fn Handler) error {
	return bus.Unsubscribe(string(c), fn)
}
