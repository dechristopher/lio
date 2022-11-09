import {
	DeserializedClockPayload,
	DeserializedScorePayload,
	SerializedClockPayload,
	SerializedScorePayload,
	WsPayloadBaseClass,
} from "./proto";

// GameOverPayload contains data regarding the outcome of the game
export interface GameOverPayloadDeserialized {
	Winner: string; // `json:"w"`
	StatusID: number; // `json:"i"`
	Status: string; // `json:"s"`
	Clock: DeserializedClockPayload; // `json:"c,omitempty"`
	Score: DeserializedScorePayload; // sc
	RoomOver: boolean; // o
}

// GameOverPayload contains data regarding the outcome of the game
export interface GameOverPayloadSerialized {
	w: string; // `json:"w"`
	i: number; // `json:"i"`
	s: string; // `json:"s"`
	c: SerializedClockPayload; // `json:"c,omitempty"`
	sc: SerializedScorePayload; // Score
	o: boolean; // RoomOver
}

export class GameOverPayload extends WsPayloadBaseClass<
	GameOverPayloadSerialized,
	GameOverPayloadDeserialized
> {
	private data: GameOverPayloadDeserialized;

	constructor(data: GameOverPayloadSerialized) {
		super(data);
		this.data = this.deserialize(data);
	}

	public deserialize(
		data: GameOverPayloadSerialized,
	): GameOverPayloadDeserialized {
		return {
			Winner: data.w,
			StatusID: data.i,
			Status: data.s,
			Clock: {
				White: data.c?.w,
				Black: data.c?.b,
				TimeControl: data.c?.tc,
				Lag: data.c?.l,
				VariantName: data.c?.n,
				VariantGroup: data.c?.g,
			},
			Score: {
				Black: data.sc?.b,
				White: data.sc?.w,
			},
			RoomOver: data.o,
		};
	}

	public serialize(): GameOverPayloadSerialized {
		return {
			w: this.data.Winner,
			i: this.data.StatusID,
			s: this.data.Status,
			c: {
				w: this.data.Clock?.White,
				b: this.data.Clock?.Black,
				tc: this.data.Clock?.TimeControl,
				l: this.data.Clock?.Lag,
				n: this.data.Clock?.VariantName,
				g: this.data.Clock?.VariantGroup,
			},
			sc: {
				b: this.data.Score?.Black,
				w: this.data.Score?.White,
			},
			o: this.data.RoomOver,
		};
	}

	get(): GameOverPayloadDeserialized {
		return this.data;
	}

	set(data: GameOverPayloadSerialized): void {
		this.data = this.deserialize(data);
	}
}
