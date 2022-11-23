import { SerializedColor, Variant } from "@client/types";

export enum RoomState {
	Init = "init",
	WaitingForPlayers = "waiting_for_players",
	GameReady = "game_ready",
	GameOngoing = "game_ongoing",
	GameOver = "game_over",
	RoomOver = "room_over",
}

export type RoomInfo = {
	RoomID: string;
	RoomState: RoomState;
	PlayerColor: SerializedColor;
	Variant: Variant;
	IsCreator: boolean;
};
