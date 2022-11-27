import { RoomInfo, RoomState } from "@client/proto/room";
import { headers } from "next/headers";
import Board from "./Board";
import Lobby from "./Lobby";

async function getRoomData(roomId: string): Promise<RoomInfo> {
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
		// this will activate the closest error boundary
		throw new Error("Failed to fetch room data");
	}

	return res.json();
}

export default async function Page({ params }: { params: { rid: string } }) {
	// this value comes through when the client starts or refreshes. need to look into this further
	if (params.rid === "custom-sw.js") {
		return null;
	}

	const roomData = await getRoomData(params.rid);
	console.log("Room Data", roomData);

	if (roomData.RoomState === RoomState.WaitingForPlayers) {
		return <Lobby {...roomData} />;
	} else {
		return <Board />;
	}
}
