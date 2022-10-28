import {WsPayloadBaseClass} from "./proto";

export interface CrowdPayloadDeserialized {
	Black: boolean;   // `json:"b"`
	White: boolean;   // `json:"w"`
	Spec: number;     //  `json:"s,omitempty"`
}


export interface CrowdPayloadSerialized {
	b: boolean;   // `json:"b"`
	w: boolean;   // `json:"w"`
	s: number;    //  `json:"s,omitempty"`
}

export class CrowdPayload extends WsPayloadBaseClass<CrowdPayloadSerialized, CrowdPayloadDeserialized> {
	private data: CrowdPayloadDeserialized;

	constructor(data: CrowdPayloadSerialized) {
		super(data);
		this.data = this.deserialize(data);
	}

	public deserialize(data: CrowdPayloadSerialized): CrowdPayloadDeserialized {
		return {
			Black: data.b,
			White: data.w,
			Spec: data.s
		}
	}

	public serialize(): CrowdPayloadSerialized {
		return {
			b: this.data.Black,
			w: this.data.White,
			s: this.data.Spec
		}
	}

	get(): CrowdPayloadDeserialized {
		return this.data;
	}

	set(data: CrowdPayloadSerialized): void {
		this.data = this.deserialize(data);
	}
}