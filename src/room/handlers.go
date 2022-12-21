package room

import "time"

const lobbyExpiryTime = time.Minute * 5
const gameReadyExpiryTime = time.Minute
const gameOverExpiryTime = time.Second * 30
const abandonTimeout = time.Second * 20
