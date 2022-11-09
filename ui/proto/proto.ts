import { OFENPayload, OFENPayloadSerialized } from "./ofen";
import { MovePayload, MovePayloadSerialized } from "./move";
import { CrowdPayload, CrowdPayloadSerialized } from "./crowd";
import { MoveAckPayload, MoveAckPayloadSerialized } from "./move_ack";
import { GameOverPayload, GameOverPayloadSerialized } from "./game_over";
import { VariantPool } from "@/types";

export interface SerializedClockPayload {
	tc?: number; // Control
	b?: number; // Black (black clock in centi-seconds)
	w?: number; // White (white clock in centi-seconds)
	l?: number; // Lag (internal server lag in ms)
	n?: string; // variant name
	g?: VariantPool; // variant group name
}

export interface DeserializedClockPayload {
	TimeControl?: number; // tc
	Black?: number; // `json:"b"` (black clock in centi-seconds)
	White?: number; // `json:"w"` (white clock in centi-seconds)
	Lag?: number; // `json:"l"` (internal server lag in ms)
	VariantName?: string; // `json:"n"` (variant name)
	VariantGroup?: VariantPool; // `json:"g"` (variant group name)
}

export type SerializedScorePayload = {
	b?: number;
	w?: number;
};

export type DeserializedScorePayload = {
	Black?: number;
	White?: number;
};

export enum MessageTag {
	OFENTag = "o", // OFENTag is the message type tag for the OFENPayload
	MoveTag = "m", // MoveTag is the message type tag for the MovePayload
	MoveAckTag = "a", // MoveAckTag is the message type tag for the MoveAckPayload
	CrowdTag = "c", // CrowdTag is the message type tag for the CrowdPayload
	GameOverTag = "g", // GameOverTag is the message type tag for the GameOverPayload
}

type Payload =
	| OFENPayload
	| MovePayload
	| MoveAckPayload
	| CrowdPayload
	| GameOverPayload;

type SerializedPayloads =
	| OFENPayloadSerialized
	| MovePayloadSerialized
	| MoveAckPayloadSerialized
	| CrowdPayloadSerialized
	| GameOverPayloadSerialized;

interface PayloadMap extends Record<MessageTag, Payload> {
	[MessageTag.OFENTag]: OFENPayload;
	[MessageTag.MoveTag]: MovePayload;
	[MessageTag.MoveAckTag]: MoveAckPayload;
	[MessageTag.CrowdTag]: CrowdPayload;
	[MessageTag.GameOverTag]: GameOverPayload;
}

type MessageData = PayloadMap[MessageTag];

export class WsPayloadBaseClass<Serialized, Deserialized> {
	constructor(_: Serialized) {
		if (new.target === WsPayloadBaseClass) {
			throw new TypeError("Cannot construct Abstract instances directly");
		}
	}

	public deserialize(_: Serialized): Deserialized {
		throw new Error("Method deserialize must be implemented");
	}

	public serialize(): Serialized {
		throw new Error("Method serialize must be implemented");
	}

	public set(_: Serialized): void {
		throw new Error("Method set must be implemented");
	}

	public get(): Deserialized {
		throw new Error("Method get must be implemented");
	}
}

export type SocketResponse = {
	t: MessageTag;
	d: SerializedPayloads;
	po?: number | null;
};

interface SocketMessage {
	t: MessageTag; // message tag
	d: SerializedPayloads; // message data
}

/**
 * Build socket message.
 *
 * @param {MessageTag} tag - message tag
 * @param {MessageData} data - message payload data
 *
 * @returns {string} - socket message
 */
export const BuildSocketMessage = (
	tag: MessageTag,
	data: MessageData,
): string => {
	const m: SocketMessage = {
		t: tag,
		d: data.serialize(),
	};

	return JSON.stringify(m);
};
