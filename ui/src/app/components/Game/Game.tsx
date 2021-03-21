import React, {FC, useEffect, useState} from "react";
import {Game as GameController} from "@components/Game";
import useWebSocket, {ReadyState} from "react-use-websocket";
import {BaseWsURL, WebSocketConnectionStatuses} from "@utils/constants";

const logMe = () => console.log(`© 2021 lioctad.org`);


/**
 * Sends an empty move message to prompt a response with board info
 */
const sendBoardUpdateRequest = () => {
	send(buildCommand("m", {a: 0}));
};

interface PingState {
	pingRunner?: number;   // interval to calculate ping
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
	// TODO: change to proper WS channel for game queues
	const socketURL = `${BaseWsURL}/test`;
	const [ka, setKa] = useState<number>()                                    // keep-alive interval id
	const [backoff, setBackoff] = useState<number>(0);           // incremental backoff
	const [pingState, setPingState] = useState<PingState>(initialPingState);  // internal ping state

	const connected = () => {
		setBackoff(0);

	}

	const {
		sendMessage,
		lastMessage,
		readyState
	} = useWebSocket(socketURL, {
		onOpen: connected,
		onClose: () => {
		},
		onError: () => {
		},
		onMessage: () => {
		},
		shouldReconnect: () => true
	})

	useEffect(() => {
		logMe();
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


	return (
		<div className="mt-16 w-screen flex justify-center items-center overflow-hidden"
		     style={{height: "calc(100vh - 4rem)"}}>
			<GameController/>
		</div>
	);
}