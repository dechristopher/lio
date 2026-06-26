package channel

import "sync"

// Directory is a map[channel] -> SockMap (map[string]Socket)
type Directory struct {
	*sync.Map
}

var (
	Map = Directory{Map: &sync.Map{}}
)

// GetSockMap returns the SockMap for a given channel, or a
// new one if it does not exist for the channel already
func (d *Directory) GetSockMap(channel string) *SockMap {
	sockMapRaw, ok := Map.Load(channel)
	if !ok {
		Map.Store(channel, NewSockMap(channel))
		sockMapRaw, _ = Map.Load(channel)
	}
	sockMap, ok := sockMapRaw.(*SockMap)
	if !ok {
		return nil
	}

	return sockMap
}

// Peek returns the SockMap for a channel if one already exists, or nil. Unlike
// GetSockMap it never creates (and never starts a broadcaster goroutine for) a
// channel that has no connections yet, so read-only callers — like the home-page
// online count walking every room — don't spawn empty SockMaps as a side effect.
func (d *Directory) Peek(channel string) *SockMap {
	raw, ok := d.Load(channel)
	if !ok {
		return nil
	}
	sockMap, _ := raw.(*SockMap)
	return sockMap
}
