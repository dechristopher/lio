import {OFENPayload} from "@utils/proto/watch/ofen";
import {MovePayload} from "@utils/proto/game/move";
import {CrowdPayload} from "@utils/proto/crowd/crowd"
import {MoveAckPayload} from "@utils/proto/game/move_ack";
import {GameOverPayload} from "@utils/proto/crowd/game_over";

export enum MessageTag {
	OFENTag = "o",    // OFENTag is the message type tag for the OFENPayload
	MoveTag = "m",    // MoveTag is the message type tag for the MovePayload
	MoveAckTag = "a", // MoveAckTag is the message type tag for the MoveAckPayload
	CrowdTag = "c",   // CrowdTag is the message type tag for the CrowdPayload
	GameOverTag = "g" // GameOverTag is the message type tag for the GameOverPayload
}

// ClockPayload is a wire representation of the current state of a game's clock
export interface ClockPayload {
	Black: number;  //`json:"b"` (black clock in centi-seconds)
	White: number;  //`json:"w"` (white clock in centi-seconds)
	Lag: number;    //`json:"l"` (internal server lag in ms)
}

export type Payload =
	| OFENPayload
	| MovePayload
	| MoveAckPayload
	| CrowdPayload
	| GameOverPayload

interface PayloadMap extends Record<MessageTag, Payload> {
	[MessageTag.OFENTag]: OFENPayload;
	[MessageTag.MoveTag]: MovePayload;
	[MessageTag.MoveAckTag]: MoveAckPayload;
	[MessageTag.CrowdTag]: CrowdPayload;
	[MessageTag.GameOverTag]: GameOverPayload;
}

export class WsPayloadBaseClass<Serialized, Deserialized> {
	constructor(_: Serialized) {
		if (new.target === WsPayloadBaseClass) {
			throw new TypeError("Cannot construct Abstract instances directly");
		}
	}

	public deserialize(_: Serialized): Deserialized {
		throw new Error("Method deserialize must be implemented")
	}

	public serialize(): Serialized {
		throw new Error("Method serialize must be implemented")
	}

	public set(_: Serialized): void {
		throw new Error("Method set must be implemented")
	}

	public get(): Deserialized {
		throw new Error("Method get must be implemented")
	}
}

export type SocketResponse = {
	t: MessageTag;
	d: unknown;
}

interface SocketMessage {
	t: MessageTag;   // message tag
	d: unknown;     // message data
}

/**
 * Build socket message.
 *
 * @param {MessageTag} tag - message tag
 * @param {PayloadMap[MessageTag]} data - message payload data
 *
 * @returns {string} - socket message
 */
export const BuildSocketMessage = (tag: MessageTag, data: PayloadMap[MessageTag]): string => {
	const m: SocketMessage = {
		t: tag,
		d: data.get()
	}

	return JSON.stringify(m);
};