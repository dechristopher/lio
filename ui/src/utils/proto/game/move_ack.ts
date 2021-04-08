import {WsPayloadBaseClass} from "@utils/proto/proto";

export type MoveAckPayloadSerialized = number;
export type MoveAckPayloadDeserialized = number;

export class MoveAckPayload extends WsPayloadBaseClass<MoveAckPayloadSerialized, MoveAckPayloadDeserialized> {
	private data: MoveAckPayloadDeserialized;

	constructor(data: number) {
		super(data);
		this.data = this.deserialize(data);
	}

	static deserialize(data: MoveAckPayloadSerialized): MoveAckPayloadDeserialized {
		return data
	}

	public serialize(): MoveAckPayloadSerialized {
		return this.data;
	}

	get(): MoveAckPayloadDeserialized {
		return this.data;
	}

	set(data: MoveAckPayloadSerialized): void {
		this.data = this.deserialize(data);
	}
}