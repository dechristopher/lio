import React, {FC, useEffect, useRef, useState} from "react";
import {Game as GameController} from "@components/Game";
import useWebSocket, {ReadyState} from "react-use-websocket";
import {BaseWsURL, WebSocketConnectionStatuses} from "@utils/constants";
import {BuildSocketMessage, MessageTag, Payload, SocketResponse} from "@utils/proto/proto";
import {MovePayload} from "@utils/proto/game/move";

const logMe = () => console.log(`© 2021 lioctad.org`);

const boardUpdateReqPayload = new MovePayload({
	c: {
		Black: 0,
		White: 0,
		Lag: 0
	},
	k: false,
	l: 0,
	m: [],
	n: 0,
	o: "",
	s: "",
	u: "",
	v: new Map<string, string[]>(),
	a: 0
})

interface PingState {
	pingRunner?: NodeJS.Timeout;   // interval to calculate ping
	pingRunnerId?: number;
	lastPingTime?: number; // time in seconds of last ping
	latency: number;      // avg latency between pings
	pongCount: number;    // qty of pong responses
	pingDelay: number;    // amount of time between sending ping requests
}

const initialPingState: PingState = {
	latency: 0,
	pongCount: 0,
	pingDelay: 3000
}

export const Game: FC = () => {
	const socketURL = `${BaseWsURL}/test`;
	const [ka, setKa] = useState<NodeJS.Timeout | undefined>(undefined)	// keep-alive interval id
	const [backoff, setBackoff] = useState<number>(0);           			// incremental backoff
	const [pingState, setPingState] = useState<PingState>(initialPingState);  		// internal ping state
	const [move, setMove] = useState<number>(0)
	const didUnmount = useRef(false);

	const connected = () => {
		setBackoff(0);
		sendBoardUpdateRequest();
		schedulePing(500);
		setKa(setInterval(() => {
			sendKeepAlive();
		}, 3000))
	}

	const {
		sendMessage,
		lastMessage,
		readyState
	} = useWebSocket(socketURL, {
		onOpen: connected,
		onClose: () => {
			console.warn("Lost connection to lioctad.org");
			if (ka) {
				clearInterval(ka);
			}
			if (pingState.pingRunner) {
				clearInterval(pingState.pingRunner);
			}
			disableBoard();
		},
		onError: () => {
		},
		onMessage: (event) => {
			parseResponse(event.data);
		},
		shouldReconnect: () => {
			incrBackoff();
			/*
              useWebSocket will handle unmounting for you, but this is an example of a
              case in which you would not want it to automatically reconnect
            */
			return !didUnmount.current;
		},
		reconnectAttempts: 10,
		reconnectInterval: backoff,
	})

	useEffect(() => {
		logMe();

		// updates the ref used by the websocket re-connection handler
		return () => {
			didUnmount.current = true;
		};
	}, [])


	useEffect(() => {
		console.log(`Web socket connection status: ${WebSocketConnectionStatuses[readyState]}`)

		if (readyState === ReadyState.OPEN) {
			sendMessage("Hello!")
		}
	}, [readyState])

	useEffect(() => {
		console.log(`Last message: ${lastMessage}`)
	}, [lastMessage])

	/**
	 * Determine what to do with received responses
	 * @param raw - the raw message JSON string
	 */
	const parseResponse = (raw: string) => {
		if (!raw) {
			return
		}

		const message = JSON.parse(raw) as SocketResponse;

		// handle pongs
		// if (message.po && message.po === 1) {
		// 	pong();
		// 	return;
		// }

		switch (message.t) {
			case MessageTag.MoveTag: // move happened
				// TODO: is this the best way to do this?
				const data = message.d as MovePayload;

				if (!data.get().Moves) {
					setMove(1)
					// document.getElementById("info").innerHTML = ""
					// 	+ "FREE, ONLINE OCTAD COMING SOON!";
				}
				const ofenParts = data.get().OFEN.split(' ');
				// og.set({
				// 	ofen: ofenParts[0],
				// 	lastMove: getLastMove(message.d.m),
				// 	turnColor: ofenParts[1] === "w" ? "white" : "black",
				// 	check: message.d.k,
				// 	movable: {
				// 		free: false,
				// 		dests: allMoves(message.d.v),
				// 	}
				// });
				if (data.get().SAN) {
					playSound(data.get().SAN);
				}
				// perform pre-move if set
				// og.playPremove();
				break;
			case MessageTag.GameOverTag: // game over
				// document.getElementById("info").innerHTML = message.d.s;
				endSound.play();
				break;
			case MessageTag.CrowdTag:
				// document.getElementById("crowd").innerHTML = message.d.s;
				break;
			default:
				return;
		}
	};


	/**
	 * Increment the backoff time so we don't flood the backend
	 */
	const incrBackoff = () => {
		if (backoff === 0) {
			setBackoff(1000)
		} else if (backoff <= 4000) {
			setBackoff(s => s * 2)
		}
		console.log("Waiting " + backoff + " seconds to retry...");
	};

	/**
	 * Disable board if disconnected
	 */
	const disableBoard = () => {
		// TODO: implement
		// og.set({
		// 	movable: {
		// 		free: false,
		// 		dests: new Map(),
		// 	}
		// });
	};

	/**
	 * Sends a keep-alive message, requesting the socket stay open
	 */
	const sendKeepAlive = () => {
		send("null");
	};

	/**
	 * Send a ping immediately
	 */
	const ping = () => {
		try {
			send(JSON.stringify({"pi": 1}));
			setPingState(s => ({
				...s,
				lastPingTime: Date.now()
			}))
		} catch (e) {
			console.debug(e, true);
		}
	};

	/**
	 * Schedule a ping message after the specified delay
	 * @param delay - delay in ms to wait before pinging
	 */
	const schedulePing = (delay: number) => {
		if (pingState.pingRunner) {
			clearInterval(pingState.pingRunner);
		}

		setPingState(s => ({
			...s,
			pingRunner: setInterval(ping, delay)
		}))
	};

	/**
	 * Sends an empty move message to prompt a response with board info.
	 */
	const sendBoardUpdateRequest = () => {
		send(BuildSocketMessage(MessageTag.MoveTag, boardUpdateReqPayload));
	};

	/**
	 * Send a JSON stringified command over the websocket.
	 *
	 * @param {string} command - websocket command
	 */
	const send = (command: string) => {
		if (readyState === WebSocket.OPEN) {
			sendMessage(command);
		}
	};


	return (
		<div className="mt-16 w-screen flex justify-center items-center overflow-hidden"
		     style={{height: "calc(100vh - 4rem)"}}>
			<GameController/>
		</div>
	);
}