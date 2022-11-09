import {
	SerializedClockPayload,
	DeserializedClockPayload,
	WsPayloadBaseClass,
} from "./proto";

export type SerializedScorePayload = {
	b?: number;
	w?: number;
};

export type DeserializedScorePayload = {
	Black?: number;
	White?: number;
};

export enum RoomState {
	Init = "init",
	WaitingForPlayers = "waiting_for_players",
	GameReady = "game_ready",
	GameOngoing = "game_ongoing",
	GameOver = "game_over",
	RoomOver = "room_over",
}

// MovePayload contains all data necessary to represent a single
// move during a live game and update game ui accordingly
export interface MovePayloadDeserialized {
	Clock?: DeserializedClockPayload; // c
	OFEN?: string; // o
	SAN?: string; // s
	UOI: string; // u
	MoveNum?: number; // n
	Check?: boolean; // k
	Moves?: string[]; // m
	ValidMoves?: Map<string, string[]>; // v
	Latency?: number; // l
	Ack: number; // a
	White?: string; // w
	Black?: string; // b
	Score?: DeserializedScorePayload; // sc
	GameStart?: boolean; // gs
	RoomState?: RoomState; // rs
}

export interface MovePayloadSerialized {
	c?: SerializedClockPayload; // Clock
	o?: string; // OFEN
	s?: string; // SAN
	u: string; // UOI
	n?: number; // MoveNum
	k?: boolean; // Check
	m?: string[]; // Moves
	v?: Map<string, string[]>; // Valid Moves
	l?: number; // Latency
	a: number; // Acknowledgement
	w?: string; // White
	b?: string; // Black
	sc?: SerializedScorePayload; // Score
	gs?: boolean; // GameStart
	rs?: RoomState; // RoomState
}

export class MovePayload extends WsPayloadBaseClass<
	MovePayloadSerialized,
	MovePayloadDeserialized
> {
	private data: MovePayloadDeserialized;

	constructor(data: MovePayloadSerialized) {
		super(data);
		this.data = this.deserialize(data);
	}

	public deserialize(data: MovePayloadSerialized): MovePayloadDeserialized {
		return {
			Clock: {
				White: data.c?.w,
				Black: data.c?.b,
				TimeControl: data.c?.tc,
				Lag: data.c?.l,
				VariantName: data.c?.n,
				VariantGroup: data.c?.g,
			},
			OFEN: data.o,
			SAN: data.s,
			UOI: data.u,
			MoveNum: data.n,
			Check: data.k,
			Moves: data.m,
			ValidMoves: data.v,
			Latency: data.l,
			Ack: data.a,
			White: data.w,
			Black: data.b,
			Score: {
				Black: data.sc?.b,
				White: data.sc?.w,
			},
			GameStart: data.gs,
			RoomState: data.rs,
		};
	}

	public serialize(): MovePayloadSerialized {
		return {
			c: {
				w: this.data.Clock?.White,
				b: this.data.Clock?.Black,
				tc: this.data.Clock?.TimeControl,
				l: this.data.Clock?.Lag,
				n: this.data.Clock?.VariantName,
				g: this.data.Clock?.VariantGroup,
			},
			o: this.data.OFEN,
			s: this.data.SAN,
			u: this.data.UOI,
			n: this.data.MoveNum,
			k: this.data.Check,
			m: this.data.Moves,
			v: this.data.ValidMoves,
			l: this.data.Latency,
			a: this.data.Ack,
			w: this.data.White,
			b: this.data.Black,
			sc: {
				b: this.data.Score?.Black,
				w: this.data.Score?.White,
			},
			gs: this.data.GameStart,
			rs: this.data.RoomState,
		};
	}

	get(): MovePayloadDeserialized {
		return this.data;
	}

	set(data: MovePayloadSerialized): void {
		this.data = this.deserialize(data);
	}
}
