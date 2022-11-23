"use client";

import { usePathname } from "next/navigation";
import useWebSocket from "react-use-websocket";

export default function Layout({ children }: { children: React.ReactNode }) {
	const pathName = usePathname();
	const ws = useWebSocket(`ws://localhost:3000/api/ws/socket${pathName}`, {
		share: true,
		onOpen: () => {
			console.log("[Websocket - BOARD] Connected to lioctad.org");
			// sends an empty move message to prompt a response with board info
			// sendMessage(BuildCommand("m", { a: 0 }));
		},
		onClose: (event) => {
			console.warn("[Websocket] Lost connection to lioctad.org", event);

			// disable the board
			// setOctadgroundState((oldState) => ({
			// 	...oldState,
			// 	movable: {
			// 		free: false,
			// 		dests: new Map(),
			// 	},
			// }));
		},
		onMessage: (event) => {
			console.log("WS Event", event);
			// if (event.data) {
			// 	const socketResponse: SocketResponse = JSON.parse(
			// 		event.data,
			// 	);
			// 	parseResponse(socketResponse);
			// }
		},
	});

	return children;
}
