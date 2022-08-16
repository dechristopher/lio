package room

import "fmt"

type ErrNoRoom struct {
	ID string
}

func (e ErrNoRoom) Error() string {
	return fmt.Sprintf("room:get: no room found with id %s", e.ID)
}

type ErrBadParamsBots struct{}

func (e ErrBadParamsBots) Error() string {
	return "room:config: P1 must be set as a human for P2 to be set as a human"
}

type ErrBadParamsTwoBots struct{}

func (e ErrBadParamsTwoBots) Error() string {
	return "room:config: P1 must be a human if P2 is set as a bot"
}

type ErrBadParamsPlayers struct{}

func (e ErrBadParamsPlayers) Error() string {
	return "room:config: both players must be configured when creating a room"
}
