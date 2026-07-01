package room

import "time"

const roomExpiryTime = time.Minute * 5
const abandonTimeout = time.Second * 20

// idleTimeout is how long a bot game will wait on a socket-connected human who
// has not made a single move this game before treating them as having abandoned
// it. It is more generous than abandonTimeout (which keys off an actual socket
// disconnect) because a real player may legitimately take time over their first
// reply; it only needs to be well under a typical clock so a no-show game is
// cleaned up promptly instead of being played out to a flag for nobody.
const idleTimeout = time.Second * 30
