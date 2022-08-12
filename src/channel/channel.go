package channel

// Directory is a map[channel] -> SockMap (map[string]Socket)
type Directory = map[string]*SockMap

var (
	Map = make(Directory)
)
