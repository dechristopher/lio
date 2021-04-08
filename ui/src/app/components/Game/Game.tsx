import React, {FC, useEffect, useRef, useState} from "react";
import {Game as GameController} from "@components/Game";
import useWebSocket, {ReadyState} from "react-use-websocket";
import {BaseWsURL} from "@utils/constants";
import {BuildSocketMessage, ClockPayload, MessageTag, SocketResponse} from "@utils/proto/proto";
import {MovePayload, MovePayloadSerialized} from "@utils/proto/game/move";
import {OctadgroundProps} from "react-octadground/octadground";
import {Howl} from 'howler'
import {CrowdPayload, CrowdPayloadSerialized} from "@utils/proto/crowd/crowd";
import {GameOverPayload, GameOverPayloadSerialized} from "@utils/proto/crowd/game_over";

const logMe = () => console.log(`© 2021 lioctad.org`);

const soundPath = "../../../assets/sfx/"

const moveSound = new Howl({
	src: [`${soundPath}move.ogg`],
	preload: true,
	autoplay: true,
	html5: true,
	volume: 1.0
});

const capSound = new Howl({
	src: [`${soundPath}capture.ogg`],
	preload: true,
	volume: 1.0
});

const endSound = new Howl({
	src: [`${soundPath}end.ogg`],
	preload: true,
	volume: 0.6
});

const initialClockState: ClockPayload = {
	Black: 0,
	White: 0,
	Lag: 0
}

// TODO better way to do this than initialize an entire class?
// we only need to send {a: 0}
const boardUpdateReqPayload = new MovePayload({
	c: initialClockState,
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

const initialGameState: OctadgroundProps = {
	ofen: "",
	highlight: {
		lastMove: true,
		check: true,
	},
	movable: {
		free: false,
		color: 'white',
	},
	selectable: {
		enabled: false
	},
}

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
	const [move, setMove] = useState<number>(0) // TODO add move
	const [gameState, setGameState] = useState<OctadgroundProps>({
		...initialGameState
	})
	const [,setClock] = useState<ClockPayload>(initialClockState)
	const [numConnected, setNumConnected] = useState<number>(0)
	const [infoContent, setInfoContent] = useState<string>("FREE, ONLINE OCTAD COMING SOON!")
	const didUnmount = useRef(false);

	useEffect(() => {
		console.log("useEffect - gameState", gameState)
	}, [JSON.stringify(gameState)] )

	const connected = () => {
		console.log("connected")
		setBackoff(0);
		sendBoardUpdateRequest();
		schedulePing(500);
		scheduleKeepAlive(3000)
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
		onError: (event) => {
			console.error("Error", event)
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

	/**
	 * Refresh our intervals whenever readyState changes so that they
	 * get access to the freshest state.
	 */
	useEffect(() => {
		console.debug("useEffect - refresh intervals")
		schedulePing(pingState.pingDelay)
		scheduleKeepAlive(3000)
	}, [readyState])

	/**
	 * Log when we first mount, update the reconnection.
	 */
	useEffect(() => {
		logMe();

		// updates the ref used by the websocket reconnection handler
		return () => {
			didUnmount.current = true;
		};
	}, [])

	// DEBUG
	useEffect(() => {
		console.debug(`Last message: ${lastMessage?.data}`)
	}, [lastMessage])

	/**
	 * Return the most recent game move for last move highlighting.
	 *
	 * @param {string[]} moves - ordered list of all moves
	 * @returns {[string, string]|*[]} - most recent move
	 */
	const getLastMove = (moves: string[]) => {
		if (moves && moves.length > 0) {
			const move = moves[moves.length - 1];
			return [
				move.substring(0, 2),
				move.substring(2, 4)
			];
		}

		return [];
	};

	/**
	 * Return a map of all legal moves.
	 *
	 * @param {Map<string, string[]>} moves - raw moves object
	 * @returns {Map<string, string[]>} - all legal moves
	 */
	const allMoves = (moves: Map<string, string[]>) => {
		const allMoves = new Map();

		if (!!moves) {
			Object.entries(moves).forEach(([s1, dests]) => {
				allMoves.set(s1, dests);
			});
		}

		return allMoves;
	};

	/**
	 * Play sounds for incoming moves based on the SAN for the move.
	 *
	 * @param {string} san - SAN for the last move
	 */
	const playSound = (san: string) => {
		if (san.includes("x")) {
			capSound.play();
		} else {
			moveSound.play();
		}
	};

	/**
	 * Determine what to do with received responses.
	 *
	 * @param {string} raw - the raw message JSON string
	 */
	const parseResponse = (raw: string) => {
		if (!raw) {
			return
		}

		const message = JSON.parse(raw) as SocketResponse;

		// handle pongs
		// @ts-ignore TODO need to implement ping/pong payload
		if (message.po && message.po === 1) {
			pong();
			return;
		}

		switch (message.t) {
			case MessageTag.MoveTag: // move happened
				console.log("MOVE MESSAGE", message)
				const movePayload = new MovePayload(message.d as MovePayloadSerialized)

				if (!movePayload.get().Moves) {
					setMove(1)
					setInfoContent("FREE, ONLINE OCTAD COMING SOON!")
				}

				const { OFEN, Moves, ValidMoves, Clock, SAN, Check } = movePayload.get();

				if (OFEN) {
					const ofenParts = OFEN.split(' ');

					setGameState(s => ({
						...s,
						ofen: ofenParts[0],
						lastMove: Moves ? getLastMove(Moves) : [],
						turnColor: ofenParts[1] === "w" ? "white" : "black",
						check: Check,
						movable: {
							free: false,
							dests: ValidMoves ? allMoves(ValidMoves) : new Map()
						},
					}))
				}

				if (Clock) {
					setClock(Clock)
				}

				if (SAN) {
					playSound(SAN);
				}
				// perform pre-move if set
				// gameState.playPremove();
				break;
			case MessageTag.GameOverTag: // game over
				console.log("GAME OVER MESSAGE", message)
				const gameOverPayload = new GameOverPayload(message.d as GameOverPayloadSerialized)

				setInfoContent(gameOverPayload.get().Status)
				endSound.play();
				break;
			case MessageTag.CrowdTag:
				console.log("CROWD MESSAGE", message)

				const cp = new CrowdPayload(message.d as CrowdPayloadSerialized)

				setNumConnected(cp.get().Spec)
				break;
			default:
				return;
		}
	};

	/**
	 * Perform move from origin to destination square and prompt for promotion.
	 *
	 * @param {string} orig - origin square
	 * @param {string} dest - destination square
	 */
	const doMove = (orig: string, dest: string) => {
		const promo = "";

		console.log("doMove - gameState", gameState)

		// if (gameState.state.pieces.get(dest) && gameState.state.pieces.get(dest).role === "pawn") {
		// 	let destPiece = og.state.pieces.get(dest);
		// 	// TODO prompt for promo piece type
		// 	if (destPiece.color === "white" && dest[1] === "4") {
		// 		promo = 'q';
		// 		// document.getElementById("promo-shade-xx").classList.remove('hidden');
		// 		// document.getElementById("promo-xx").classList.remove('hidden');
		// 	} else if (destPiece.color === "black" && dest[1] === "1") {
		// 		promo = 'q';
		// 	}
		// }

		sendGameMove(orig + dest + promo, move);
		setMove(s => s++)
	};

	/**
	 * Sends a game move in Universal Octad Interface format.
	 *
	 * @param {string} move - UOI move string
	 * @param {string} num - move number
	 */
	const sendGameMove = (move: string, num: number) => {
		const gameMove = new MovePayload({
			a: num,
			u: move,
			v: new Map<string, string[]>([])
		})

		console.log("sendGameMove", gameMove)

		send(BuildSocketMessage(
			MessageTag.MoveTag,
			gameMove
		));
	};

	/**
	 * Increment the backoff time so we don't flood the backend.
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
	 * Disable board if disconnected.
	 */
	const disableBoard = () => {
		setGameState(s => ({
			...s,
			movable: {
				free: false,
				dests: new Map()
			}
		}))
	};

	/**
	 * Sends a keep-alive message, requesting the socket stay open.
	 */
	const sendKeepAlive = () => {
		console.debug("sendKeepAlive")
		send("null");
	};

	/**
	 * Send a ping immediately.
	 */
	const ping = () => {
		try {
			console.debug("ping")
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
	 * Handle pong response, calculate latency.
	 */
	const pong = () => {
		console.debug("pong")
		const { pingDelay, lastPingTime, pongCount, latency } = pingState
		const newPongCount = pongCount + 1;

		// schedule the next ping
		schedulePing(pingDelay);
		const currentLag = Math.min(Date.now() - (lastPingTime || 0), 10000);

		// average first few pings and then move to weighted moving average
		const weight = newPongCount > 4 ? 0.1 : 1 / newPongCount;

		// console.log(`Weight * (currentLag - latency)`)
		// console.log(`${weight} * (${currentLag} - ${latency})`)

		setPingState(s => ({
			...s,
			pongCount: newPongCount,
			latency: s.latency + weight * (currentLag - latency)
		}))
	};

	/**
	 * Schedule a ping message after the specified delay.
	 *
	 * @param {number} delay - delay in ms to wait before pinging
	 */
	const schedulePing = (delay: number) => {
		console.debug("schedulePing")
		// clear the old interval if it exists
		if (pingState.pingRunner) {
			clearInterval(pingState.pingRunner);
		}

		setPingState(s => ({
			...s,
			pingRunner: setInterval(ping, delay)
		}))
	};

	/**
	 * Schedule a keep alive message after the specified delay.
	 *
	 * @param {number} delay - delay in ms to wait before sending a keep alive
	 */
	const scheduleKeepAlive = (delay: number) => {
		console.debug("scheduleKeepAlive")
		// clear the old interval if it exists
		if (ka) {
			clearInterval(ka);
		}

		setKa(setInterval(sendKeepAlive, delay))
	};

	/**
	 * Sends an empty move message to prompt a response with board info.
	 */
	const sendBoardUpdateRequest = () => {
		console.log("sendBoardUpdateRequest", readyState === ReadyState.OPEN)
		sendMessage(BuildSocketMessage(MessageTag.MoveTag, boardUpdateReqPayload));
	};

	/**
	 * Send a JSON stringified command over the websocket.
	 *
	 * @param {string} command - websocket command
	 */
	const send = (command: string) => {
		if (readyState === ReadyState.OPEN) {
			sendMessage(command);
		}
	};


	return (
		<div
			className="mt-16 w-screen flex justify-center items-center overflow-hidden"
			style={{height: "calc(100vh - 4rem)"}}>
			<div>
				<GameController
					gameState={gameState}
					onMove={doMove}
				/>

				<div className="text-center flex flex-col">
					<span className="sm" id="info">{infoContent}</span>
					<span className="sm">
					<span id="crowd">{numConnected}</span> CONNECTED
					[LAG <span id="lat">{pingState.latency.toFixed(1)}</span><span className="unit">MS</span>]
					</span>
				</div>
			</div>
		</div>
	);
}