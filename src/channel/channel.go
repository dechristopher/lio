package channel

import "sync"

// Directory is a map[channel] -> SockMap (map[string]Socket)
type Directory struct {
	*sync.Map
}

var (
	Map = Directory{Map: &sync.Map{}}
)

// GetSocket returns the socket from the given channel's SockMap
func (d *Directory) GetSocket(channel, uid string) *Socket {
	sockMap := d.GetSockMap(channel)
	if sockMap == nil {
		return nil
	}

	return sockMap.Get(uid)
}

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
