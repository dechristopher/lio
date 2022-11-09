import React, { useState } from "react";

import { GetBrowserId } from "@/components/shared";
import { Color, SocketResponse } from "@/proto/proto";
import { RoomState, MovePayload, MovePayloadSerialized } from "@/proto/move";

import { useRouter } from "next/router";
import useWebSocket from "react-use-websocket";
import Lobby from "@/components/Lobby/Lobby";
import Board, { BuildCommand } from "@/components/Board/Board";
import { VariantPool } from "@/proto/pools";

export default function Room() {
	const router = useRouter();
	const [gameState, setGameState] = useState<RoomState>(RoomState.Init);
	const [playerColor, setPlayerColor] = useState<Color | null>(null);
	const [variantName, setVariantName] = useState<string | null>(null);
	const [variantGroup, setVariantGroup] = useState<VariantPool | null>(null);

	const { sendMessage } = useWebSocket(
		`ws://localhost:3000/api/socket${router.asPath}`,
		{
			share: true,
			onOpen: () => {
				console.log("[Websocket - ROOT] Connected to lioctad.org");
				// sends an empty move message to prompt a response with board info
				sendMessage(BuildCommand("m", { a: 0 }));
			},
			onClose: (event) => {
				console.warn(
					"[Websocket] Lost connection to lioctad.org",
					event,
				);
			},
			onMessage: (event) => {
				console.log("BROWSER ID", GetBrowserId());
				if (event.data) {
					const socketResponse = JSON.parse(event.data);
					parseResponse(socketResponse);
				}
			},
		},
	);

	const parseResponse = (socketResponse: SocketResponse) => {
		console.log("[Websocket - ROOT] Message", socketResponse);

		switch (socketResponse.t) {
			case "m": {
				const payload = new MovePayload(
					socketResponse.d as MovePayloadSerialized,
				).get();

				setPlayerColor(
					payload.White === GetBrowserId()
						? Color.WHITE
						: Color.BLACK,
				);
				if (payload.RoomState) {
					setGameState(payload.RoomState);
				}
				if (payload.Clock?.VariantName && payload.Clock?.VariantGroup) {
					setVariantName(payload.Clock.VariantName);
					setVariantGroup(payload.Clock.VariantGroup);
				}
				break;
			}
			default:
				return;
		}
	};

	/**
	 * TODO: need to implement pre-join screen
	 * - a challenging player joins the invite link and kicks the game off
	 */

	if (
		gameState === RoomState.WaitingForPlayers &&
		playerColor &&
		variantName &&
		variantGroup
	) {
		return (
			<Lobby
				playerColor={playerColor}
				variantName={variantName}
				variantGroup={variantGroup}
			/>
		);
	} else if (
		gameState === RoomState.GameReady ||
		gameState === RoomState.GameOngoing
	) {
		return <Board />;
	}

	return <div>Loading...</div>;
}
