// OFENPayload contains a full board state and is sent to
// spectators after each move to update game boards
import {WsPayloadBaseClass} from "@utils/proto/proto";

export interface OFENPayloadDeserialized {
	OFEN: string;       // (OFEN (position, toMove))
	LastMove: string;   // (last move played in UOI notation)
	BlackClock: string; // (black clock in seconds)
	WhiteClock: string; // (white clock in seconds)
	GameID: string;     // (game id for routing message to board)
}

export interface OFENPayloadSerialized {
	o: string;  // (OFEN (position, toMove))
	l: string;  // (last move played in UOI notation)
	b: string;  // (black clock in seconds)
	w: string;  // (white clock in seconds)
	i: string;  // (game id for routing message to board)
}

export class OFENPayload extends WsPayloadBaseClass<OFENPayloadSerialized, OFENPayloadDeserialized> {
	private data: OFENPayloadDeserialized;

	constructor(data: OFENPayloadSerialized) {
		super(data);
		this.data = this.deserialize(data);
	}

	static deserialize(data: OFENPayloadSerialized): OFENPayloadDeserialized {
		return {
			OFEN: data.o,
			LastMove: data.l,
			BlackClock: data.b,
			WhiteClock: data.w,
			GameID: data.i
		}
	}

	public serialize(): OFENPayloadSerialized {
		return {
			o: this.data.OFEN,
			l: this.data.LastMove,
			b: this.data.BlackClock,
			w: this.data.WhiteClock,
			i: this.data.GameID
		}
	}

	get(): OFENPayloadDeserialized {
		return this.data;
	}

	set(data: OFENPayloadSerialized): void {
		this.data = this.deserialize(data);
	}
}