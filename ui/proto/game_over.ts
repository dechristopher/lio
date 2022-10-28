import { ScorePayload } from "./move";
import {ClockPayload, WsPayloadBaseClass} from "./proto";

// GameOverPayload contains data regarding the outcome of the game
export interface GameOverPayloadDeserialized {
	Winner: string;       // `json:"w"`
	StatusID: number;     // `json:"i"`
	Status: string;       // `json:"s"`
	Clock: ClockPayload;  // `json:"c,omitempty"`
  Score: ScorePayload; // sc
  RoomOver: boolean; // o
}


// GameOverPayload contains data regarding the outcome of the game
export interface GameOverPayloadSerialized {
	w: string;          // `json:"w"`
	i: number;          // `json:"i"`
	s: string;          // `json:"s"`
	c: ClockPayload;    // `json:"c,omitempty"`
  sc: ScorePayload; // Score
  o: boolean; // RoomOver
}

export class GameOverPayload extends WsPayloadBaseClass<GameOverPayloadSerialized, GameOverPayloadDeserialized> {
	private data: GameOverPayloadDeserialized;

	constructor(data: GameOverPayloadSerialized) {
		super(data);
		this.data = this.deserialize(data);
	}

	public deserialize(data: GameOverPayloadSerialized): GameOverPayloadDeserialized {
		return {
			Winner: data.w,
			StatusID: data.i,
			Status: data.s,
			Clock: data.c,
      Score: data.sc,
      RoomOver: data.o,
		}
	}

	public serialize(): GameOverPayloadSerialized {
		return {
			w: this.data.Winner,
			i: this.data.StatusID,
			s: this.data.Status,
			c: this.data.Clock,
      sc: this.data.Score,
      o: this.data.RoomOver,
		}
	}

	get(): GameOverPayloadDeserialized {
		return this.data;
	}

	set(data: GameOverPayloadSerialized): void {
		this.data = this.deserialize(data);
	}
}