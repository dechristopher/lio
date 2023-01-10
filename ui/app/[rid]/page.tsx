import { PlayerColor, RoomPayload, RoomState } from "@client/proto/ws_pb";
import { headers } from "next/headers";
import { notFound } from "next/navigation";
import Board from "./Board";
import Lobby from "./Lobby";

async function getRoomData(roomId: string): Promise<RoomPayload> {
	const headersList = headers();
	const cookie = headersList.get("cookie");
	const requestHeaders: HeadersInit = new Headers();

	if (cookie) {
		requestHeaders.set("cookie", cookie);
	}

	const res = await fetch(`http://127.0.0.1:4444/api/room/${roomId}`, {
		headers: requestHeaders,
	});

	if (!res.ok) {
		notFound();
	}

	const data = await res.json();
	return RoomPayload.fromJson(data);
}

export default async function Page({ params }: { params: { rid: string } }) {
	// this value comes through when the client starts or refreshes. need to look into this further
	if (params.rid === "custom-sw.js") {
		return null;
	}

	const roomData = await getRoomData(params.rid);
	const variant = roomData.variant;
	const playerColor = roomData.playerColor;
	console.log("Room Data", roomData);

	if (!variant || playerColor === PlayerColor.UNSPECIFIED) {
		// TODO handle errors for missing data
		return null;
	}

	if (roomData.roomState === RoomState.WAITING_FOR_PLAYERS) {
		return (
			<Lobby
				isCreator={roomData.isCreator}
				playerColor={roomData.playerColor}
				variantName={variant.name}
				variantGroup={variant.group}
			/>
		);
	} else {
		return <Board />;
	}
}
