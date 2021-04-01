import {ClockPayload, WsPayloadBaseClass} from "@utils/proto/proto";

// GameOverPayload contains data regarding the outcome of the game
interface GameOverPayloadDeserialized {
	Winner: string;       // `json:"w"`
	StatusID: number;     // `json:"i"`
	Status: string;       // `json:"s"`
	Clock: ClockPayload;  // `json:"c,omitempty"`
}


// GameOverPayload contains data regarding the outcome of the game
export interface GameOverPayloadSerialized {
	w: string;          // `json:"w"`
	i: number;          // `json:"i"`
	s: string;          // `json:"s"`
	c: ClockPayload;    // `json:"c,omitempty"`
}

export class GameOverPayload extends WsPayloadBaseClass<GameOverPayloadSerialized, GameOverPayloadDeserialized> {
	private data: GameOverPayloadDeserialized;

	constructor(data: GameOverPayloadSerialized) {
		super(data);
		this.data = this.deserialize(data);
	}

	static deserialize(data: GameOverPayloadSerialized): GameOverPayloadDeserialized {
		return {
			Winner: data.w,
			StatusID: data.i,
			Status: data.s,
			Clock: data.c
		}
	}

	public serialize(): GameOverPayloadSerialized {
		return {
			w: this.data.Winner,
			i: this.data.StatusID,
			s: this.data.Status,
			c: this.data.Clock
		}
	}

	get(): GameOverPayloadDeserialized {
		return this.data;
	}

	set(data: GameOverPayloadSerialized): void {
		this.data = this.deserialize(data);
	}
}