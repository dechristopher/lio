"use client";

import { WebsocketMessage } from "@client/proto/ws_pb";
import { usePathname } from "next/navigation";
import { useRouter } from "next/navigation";
import useWebSocket from "react-use-websocket";
import CreatorLobby from "./CreatorLobby";
import { GameSettingsProps } from "./GameSettings";
import JoinerLobby from "./JoinerLobby";

interface LobbyProps extends GameSettingsProps {
	isCreator: boolean;
}

export default function Lobby(props: LobbyProps) {
	const router = useRouter();
	const pathName = usePathname();

	// anyone who joins a lobby will establish a websocket connection. once an opponent clicks "join" it will start the
	// game and the server will tell every websocket connection to refresh their UI, switching from the lobby to the board
	useWebSocket(`ws://localhost:3000/api/ws/socket/wait${pathName}`, {
		onOpen: () =>
			console.log("[Websocket - Lobby] Connected to lioctad.org"),
		onClose: (event) =>
			console.warn(
				"[Websocket - Lobby] Lost connection to lioctad.org",
				event,
			),
		onMessage: (event) => {
			console.log("[Websocket - Lobby] Received message", event);
			if (event.data) {
				parseSocketMessage(event.data);
			}
		},
	});

	async function parseSocketMessage(res: Blob) {
		const buffer = await res.arrayBuffer();
		const message = WebsocketMessage.fromBinary(new Uint8Array(buffer));

		switch (message.data.case) {
			case "redirectPayload":
				router.push(message.data.value.location);
				break;
			default:
		}
	}

	if (props.isCreator) {
		return <CreatorLobby {...props} />;
	} else {
		return <JoinerLobby {...props} />;
	}
}
