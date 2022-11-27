"use client";

import { RoomInfo } from "@client/proto/room";
import { usePathname } from "next/navigation";
import { useRouter } from "next/navigation";
import useWebSocket from "react-use-websocket";
import CreatorLobby from "./CreatorLobby";
import JoinerLobby from "./JoinerLobby";

export default function Lobby(props: Omit<RoomInfo, "RoomState">) {
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
				const socketResponse = JSON.parse(event.data);
				console.log(socketResponse);
				if (socketResponse?.d?.l) {
					router.push(socketResponse?.d?.l);
				}
			}
		},
	});

	if (props.IsCreator) {
		return (
			<CreatorLobby
				playerColor={props.PlayerColor}
				variantName={props.Variant.name}
				variantGroup={props.Variant.group}
			/>
		);
	} else {
		return (
			<JoinerLobby
				playerColor={props.PlayerColor}
				variantName={props.Variant.name}
				variantGroup={props.Variant.group}
			/>
		);
	}
}
