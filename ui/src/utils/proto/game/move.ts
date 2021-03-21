import {ClockPayload, WsPayloadBaseClass} from "@utils/proto/proto";

// MovePayload contains all data necessary to represent a single
// move during a live game and update game ui accordingly
interface MovePayloadDeserialized {
	Clock: ClockPayload;                // c
	OFEN: string;                       // o
	SAN: string;                        // s
	UOI: string;                        // u
	MoveNum: number;                    // n
	Check: boolean;                     // k
	Moves: string[];                    // m
	ValidMoves: Map<string, string[]>;  // v
	Latency: number;                    // l
	Ack: number;                        // a
}

interface MovePayloadSerialized {
	c: ClockPayload;            // Clock
	o: string;                  // OFEN
	s: string;                  // SAN
	u: string;                  // UOI
	n: number;                  // MoveNum
	k: boolean;                 // Check
	m: string[];                // Moves
	v: Map<string, string[]>;   // Valid Moves
	l: number;                  // Latency
	a: number;                  // Ack
}

export class MovePayload extends WsPayloadBaseClass<MovePayloadSerialized, MovePayloadDeserialized> {
	private data: MovePayloadDeserialized;

	constructor(data: MovePayloadSerialized) {
		super(data);
		this.data = this.deserialize(data);
	}

	static deserialize(data: MovePayloadSerialized): MovePayloadDeserialized {
		return {
			Clock: data.c,
			OFEN: data.o,
			SAN: data.s,
			UOI: data.u,
			MoveNum: data.n,
			Check: data.k,
			Moves: data.m,
			ValidMoves: data.v,
			Latency: data.l,
			Ack: data.a
		}
	}

	public serialize(): MovePayloadSerialized {
		return {
			c: this.data.Clock,
			o: this.data.OFEN,
			s: this.data.SAN,
			u: this.data.UOI,
			n: this.data.MoveNum,
			k: this.data.Check,
			m: this.data.Moves,
			v: this.data.ValidMoves,
			l: this.data.Latency,
			a: this.data.Ack
		}
	}

	get(): MovePayloadDeserialized {
		return this.data;
	}

	set(data: MovePayloadSerialized): void {
		this.data = this.deserialize(data);
	}
}