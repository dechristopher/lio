import { RoomPayload, RoomState, VariantGroup } from "@client/proto/ws_pb";
import { headers } from "next/headers";
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
		// this will activate the closest error boundary
		throw new Error("Failed to fetch room data");
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
	console.log("Room Data", roomData);

	if (roomData.roomState === RoomState.WAITING_FOR_PLAYERS) {
		return (
			/**
			 * we must pass singular props here because we are going from a server component to a client component. the following
			 * warning is thrown if we pass the entire room payload: Warning: Only plain objects can be passed to Client Components
			 * from Server Components. Objects with toJSON methods are not supported. Convert it manually to a simple
			 * value before passing it to props.
			 */
			<Lobby
				isCreator={roomData.isCreator}
				playerColor={roomData.playerColor}
				variantName={roomData.variant?.name ?? ""}
				variantGroup={
					roomData.variant?.group ?? VariantGroup.UNSPECIFIED
				}
			/>
		);
	} else {
		return <Board />;
	}
}
