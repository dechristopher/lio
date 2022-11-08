import React, { useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import Octadground, {
	OctadgroundProps,
	Key,
	Piece,
	Pieces,
} from "react-octadground/octadground";
import "react-octadground/dist/styles/octadground.css";
import {
	CaptureSound,
	ConfirmationSound,
	GetBrowserId,
	IsMobile,
	MoveSound,
	NotificationSound,
	useAnimationFrame,
} from "@/components/shared";
import { Color, SocketResponse } from "@/proto/proto";
import {
	RoomState,
	MovePayload,
	MovePayloadDeserialized,
	MovePayloadSerialized,
} from "@/proto/move";
import {
	GameOverPayload,
	GameOverPayloadDeserialized,
	GameOverPayloadSerialized,
} from "@/proto/game_over";
import { CrowdPayload, CrowdPayloadSerialized } from "@/proto/crowd";
import { Howl } from "howler";
import { Footer } from "@/components/Footer/Footer";
import { Header } from "@/components/Header/Header";
import Clock from "@/components/Clock/Clock";
import { useRouter } from "next/router";
import useWebSocket, { ReadyState } from "react-use-websocket";
import Lobby from "@/components/Lobby/Lobby";

type Command = {
	t: string;
	d: any;
};

export default function GameBoard() {
	const router = useRouter();
	const [playPremoveFn, setPlayPremoveFn] = useState<(() => boolean) | null>(
		null,
	);
	const [octadGroundState, setOctadgroundState] = useState<OctadgroundProps>({
		ofen: "",
		setPlayPremoveFn: (playPremove) => setPlayPremoveFn(playPremove),
		highlight: {
			lastMove: true,
			check: true,
		},
		movable: {
			free: false,
		},
		onMove: (orig, dest, pieces, _capturedPiece) =>
			doMove(orig, dest, pieces),
	});
	const [lastPingTime, setLastPingTime] = useState<number>(Date.now());
	// const [frameId, setFrameId] = useState<number | null>(null);
	const [moveCount, setMoveCount] = useState(1);
	const [latency, setLatency] = useState(0);
	const [pongCount, setPongCount] = useState(0);
	const [playerTime, setPlayerTime] = useState<string>("");
	const [playerRemainingTime, setPlayerRemainingTime] = useState(0);
	const [opponentRemainingTime, setOpponentRemainingTime] = useState(0);
	const [opponentTime, setOpponentTime] = useState<string>("");
	const [playerScore, setPlayerScore] = useState<number>(0);
	const [opponentScore, setOpponentScore] = useState<number>(0);
	const [isPlayerWhite, setIsPlayerWhite] = useState<boolean>(false);
	const [timeControl, setTimeControl] = useState<number>(0);
	const [isPlayersTurn, setIsPlayersTurn] = useState(false);
	const [playerClockActive, setPlayerClockActive] = useState(false);
	const [opponentClockActive, setOpponentClockActive] = useState(false);
	const [playerClockBarWidth, setPlayerClockBarWidth] = useState(0);
	const [opponentClockBarWidth, setOpponentClockBarWidth] = useState(0);
	const [gameState, setGameState] = useState<RoomState>(RoomState.StateInit);

	const playerClockAnimationFrameHandler = (clockStartFrameTime: number) => {
		// converting milliseconds to centiseconds
		const elapsedTime = (performance.now() - clockStartFrameTime) / 10;
		const remainingTime = playerRemainingTime - elapsedTime;

		if (remainingTime < 0) {
			setPlayerClockActive(false);
		} else {
			setPlayerTime(timeFormatter(Math.max(remainingTime, 0)));
			setPlayerClockBarWidth(calcBarWidth(timeControl, remainingTime));
		}
	};

	const opponentClockAnimationFrameHandler = (
		clockStartFrameTime: number,
	) => {
		// converting milliseconds to centiseconds
		const elapsedTime = (performance.now() - clockStartFrameTime) / 10;
		const remainingTime = opponentRemainingTime - elapsedTime;

		if (remainingTime < 0) {
			setOpponentClockActive(false);
		} else {
			setOpponentTime(timeFormatter(Math.max(remainingTime, 0)));
			setOpponentClockBarWidth(calcBarWidth(timeControl, remainingTime));
		}
	};

	useAnimationFrame(playerClockAnimationFrameHandler, playerClockActive);
	useAnimationFrame(opponentClockAnimationFrameHandler, opponentClockActive);

	const { sendMessage } = useWebSocket(
		`ws://localhost:3000/api/socket${router.asPath}`,
		{
			onOpen: () => {
				console.log("[Websocket] Connected to lioctad.org");
				// sends an empty move message to prompt a response with board info
				sendMessage(buildCommand("m", { a: 0 }));
			},
			onClose: (event) => {
				console.warn(
					"[Websocket] Lost connection to lioctad.org",
					event,
				);

				// disable the board
				setOctadgroundState((oldState) => ({
					...oldState,
					movable: {
						free: false,
						dests: new Map(),
					},
				}));
			},
			onMessage: (event) => {
				if (!!event.data) {
					parseResponse(event.data);
				}
			},
		},
	);

	// kick things off, on component mount
	useEffect(() => {
		// TODO add polyfills
		// window.requestAnimationFrame = (function () {
		// 	return function (callback: FrameRequestCallback): number {
		// 		return window.setTimeout(callback, 1000 / 60);
		// 	};
		// })();

		// sends a keep-alive message, requesting the socket stay open
		const keepAlive = setInterval(() => {
			console.log("[Websocket] Sending keep alive...");
			sendMessage((null as unknown) as string);
		}, 3000);
		// pings the backend server every, used for calculating client latency
		const pingRunner = setInterval(() => {
			console.log("[Websocket] Sending ping...");
			sendMessage(JSON.stringify({ pi: 1 }));
			setLastPingTime(Date.now());
		}, 5000);

		// clears intervals when the component un-mounts
		return () => {
			console.log("Clearing keep alive...");
			clearInterval(keepAlive);
			console.log("Clearing ping runner...");
			clearInterval(pingRunner);
		};
	}, [sendMessage]);

	// build socket message
	const buildCommand = (tag: string, data: any) => {
		let m: Command = {
			t: tag,
			d: data,
		};
		return JSON.stringify(m);
	};

	/**
	 * Determine what to do with received responses
	 * @param raw - the raw message JSON string
	 */
	const parseResponse = (raw: string) => {
		let message: SocketResponse = JSON.parse(raw);

		console.log("[Websocket] Message", message);

		// handle pongs
		if (message.po === 1) {
			console.log("[Websocket] Received pong");
			const currentLag = Math.min(Date.now() - lastPingTime, 10000);
			const newPongCount = pongCount + 1;

			// average first few pings and then move to weighted moving average
			const weight = newPongCount > 4 ? 0.1 : 1 / newPongCount;
			setPongCount(newPongCount);
			setLatency((oldLatency) =>
				Math.round((oldLatency += weight * (currentLag - oldLatency))),
			);
			return;
		}

		switch (message.t) {
			case "m": // move
				const movePayload = new MovePayload(
					message.d as MovePayloadSerialized,
				).get();

				handleMove(movePayload);
				break;
			case "g": // game over
				const gameOverPayload = new GameOverPayload(
					message.d as GameOverPayloadSerialized,
				).get();

				setPlayerClockActive(false);
				setOpponentClockActive(false);
				setPlayerRemainingTime(timeControl);
				// if (requestRef.current) {
				// 	cancelAnimationFrame(requestRef.current);
				// }
				// const info = document.getElementById("info");
				// if (info) {
				// 	info.innerHTML = gameOverPayload.Status;
				// }
				NotificationSound.play();

				// disallow further moves
				setOctadgroundState((oldState) => ({
					...oldState,
					movable: {
						dests: new Map(),
					},
				}));

				// update match score TODO update to provide correct payload data
				// updateScore(gameOverPayload);

				// if room over, redirect home after a second
				if (gameOverPayload.RoomOver === true) {
					setTimeout(() => {
						window.location.href = "/";
					}, 3000);
				}
				break;
			case "c": // crowd
				const crowdPayload = new CrowdPayload(
					message.d as CrowdPayloadSerialized,
				).get();

				// const crowd = document.getElementById("crowd");
				// if (crowd) {
				// 	crowd.innerHTML = crowdPayload.Spec.toString();
				// }
				break;
			default:
				return;
		}
	};

	/**
	 * Handle incoming move messages, update board state, update UI and clocks
	 * @param message
	 */
	const handleMove = (message: MovePayloadDeserialized) => {
		console.log("[Game] Move Payload", message);
		const ofenParts: string[] = message.OFEN?.split(" ") ?? [];

		if (message.RoomState) {
			setGameState(message.RoomState);
		}

		const isPlayerWhite = message.White === GetBrowserId() ? true : false;
		const isWhitesTurn = ofenParts[1] === "w";
		const isPlayersTurn =
			(isPlayerWhite && isWhitesTurn) ||
			(!isPlayerWhite && !isWhitesTurn);
		setIsPlayersTurn(isPlayersTurn);
		setIsPlayerWhite(isPlayerWhite);

		// these are in centiseconds
		const whiteTime = message.Clock?.White;
		const blackTime = message.Clock?.Black;
		const playerTime = (isPlayerWhite ? whiteTime : blackTime) ?? 0;
		const opponentTime = (isPlayerWhite ? blackTime : whiteTime) ?? 0;
		setPlayerRemainingTime(playerTime);
		setOpponentRemainingTime(opponentTime);

		// ensure both clocks are stopped
		setPlayerClockActive(false);
		setOpponentClockActive(false);

		// game sounds
		if (message.GameStart) {
			// play confirmation sound on game start
			ConfirmationSound.play();
		} else {
			// play move sounds if game is not starting
			// and only if board ofen is different from current
			if (message.SAN && ofenParts[0] !== octadGroundState.ofen) {
				if (message.SAN.includes("x")) {
					CaptureSound.play();
				} else {
					MoveSound.play();
				}
			}
		}

		const getLastMove = (moves: string[]): [string, string] | [] => {
			if (moves && moves.length > 0) {
				const move = moves[moves.length - 1];
				return [move.substring(0, 2), move.substring(2, 4)];
			}
			return [];
		};

		const getLegalMoves = (moves: Map<string, string[]>) => {
			let allMoves = new Map();
			if (!!moves) {
				Object.entries(moves).forEach(([s1, dests]) => {
					allMoves.set(s1, dests);
				});
			}
			return allMoves;
		};

		setTimeControl(message.Clock?.TimeControl ?? 0);

		const playerColor = isPlayerWhite ? Color.WHITE : Color.BLACK;
		setOctadgroundState((oldState) => ({
			...oldState,
			ofen: ofenParts[0],
			check: message.Check,
			orientation: playerColor,
			lastMove: getLastMove(message.Moves ?? []),
			turnColor: isWhitesTurn ? Color.WHITE : Color.BLACK,
			selectable: {
				enabled: IsMobile(),
			},
			movable: {
				free: false,
				color: playerColor,
				dests: getLegalMoves(
					message.ValidMoves ?? new Map<string, string[]>(),
				),
			},
		}));

		const whiteScore = message.Score?.["w"];
		const blackScore = message.Score?.["b"];
		if (whiteScore && blackScore) {
			if (isPlayerWhite) {
				setPlayerScore(whiteScore);
				setOpponentScore(blackScore);
			} else {
				setPlayerScore(blackScore);
				setOpponentScore(whiteScore);
			}
		}

		// run at the start of the game
		if (!message.Moves?.length) {
			setMoveCount(1);
			setPlayerTime(timeFormatter(Math.max(playerTime, 0)));
			setOpponentTime(timeFormatter(Math.max(opponentTime, 0)));
			setPlayerClockBarWidth(100);
			setOpponentClockBarWidth(100);
		} else {
			// all subsequent moves
			if (isPlayersTurn) {
				setPlayerClockActive(true);
			} else {
				setOpponentClockActive(true);
			}
		}

		// perform pre-move if set
		if (playPremoveFn) {
			playPremoveFn();
		}
	};

	/**
	 * Perform move from origin to destination square and prompt for promotion
	 * @param orig - origin square
	 * @param dest - destination square
	 */
	const doMove = (orig: Key, dest: Key, pieces: Pieces) => {
		console.log("OnMove", orig, dest, pieces);
		if (pieces.get(dest)?.role === "pawn") {
			let destPiece = pieces.get(dest);

			// TODO handle piece promotion
			if (
				destPiece &&
				((destPiece.color === Color.WHITE && dest[1] === "4") ||
					(destPiece.color === Color.BLACK && dest[1] === "1"))
			) {
				// show the promo popup

				// set file for promo bar TODO figure out what this does
				// promoBar.classList.add(`f${dest[0]}`);

				// set piece selector colors and event handlers
				// let promoButtons = promoBar.getElementsByTagName("piece");
				// for (let i = 0; i < promoButtons.length; i++) {
				// 	promoButtons[i].classList.add(destPiece.color);

				// 	if (promoButtons[i].classList.contains("queen")) {
				// 		promoButtons[i].addEventListener("click", () =>
				// 			doMovePromo(orig, dest, "q"),
				// 		);
				// 	} else if (promoButtons[i].classList.contains("rook")) {
				// 		promoButtons[i].addEventListener("click", () =>
				// 			doMovePromo(orig, dest, "r"),
				// 		);
				// 	} else if (promoButtons[i].classList.contains("bishop")) {
				// 		promoButtons[i].addEventListener("click", () =>
				// 			doMovePromo(orig, dest, "b"),
				// 		);
				// 	} else if (promoButtons[i].classList.contains("knight")) {
				// 		promoButtons[i].addEventListener("click", () =>
				// 			doMovePromo(orig, dest, "n"),
				// 		);
				// 	}
				// }

				// return early and wait for doMovePromo to run
				return;
			}
		}

		console.log("[Websocket] Sending game move...");
		// send move message
		sendMessage(
			buildCommand("m", {
				u: orig + dest,
				a: moveCount,
			}),
		);
		setMoveCount((oldMove) => oldMove++);
	};

	/**
	 * Perform move from origin to destination square with selected promotion
	 * @param orig - origin square
	 * @param dest - destination square
	 * @param promo - code of piece to promote to
	 */
	// const doMovePromo = (orig: Key, dest: Key, promo: string) => {
	// 	sendGameMove(orig + dest + promo, move);
	// 	setMove((oldMove) => oldMove++);

	// 	// hide promo bar and shade after promotion
	// 	const promoShade = document.getElementById("promo-shade");
	// 	if (promoShade) {
	// 		promoShade.classList.add("hidden");
	// 	}

	// 	let promoBar = document.getElementById("promo-select");

	// 	if (promoBar) {
	// 		// hide promo bar
	// 		promoBar.classList.add("hidden");

	// 		// unset file for promo bar
	// 		promoBar.classList.remove(`f${dest[0]}`);

	// 		// unset promo piece color
	// 		let promoButtons = promoBar.getElementsByTagName("piece");
	// 		for (let i = 0; i < promoButtons.length; i++) {
	// 			promoButtons[i].classList.remove(Color.WHITE);
	// 			promoButtons[i].classList.remove(Color.BLACK);
	// 		}
	// 	}
	// };
	/**
	 *
	 * TODO
	 * 3. Pass game state status to move message payload
	 * 1. Create "Game" component to house the board and active game logic
	 * 2. Create logic to handle player connection and determine whether to show the lobby or game
	 *
	 */

	if (gameState === RoomState.StateWaitingForPlayers) {
		return (
			<Lobby playerColor={isPlayerWhite ? Color.WHITE : Color.BLACK} />
		);
	}

	return (
		<div
			className="flex flex-col items-center pt-8"
			style={{
				width: "100vw",
				height: "100vh",
			}}
		>
			<Header />
			<div className="font-bold text-3xl italic mb-2 leading-none">
				½ + 1 Blitz
			</div>

			<div className="flex flex-col items-center">
				<Clock
					flipOrientation={true}
					time={opponentTime}
					barWidth={opponentClockBarWidth}
					isWhite={!isPlayerWhite}
					isActive={!isPlayersTurn}
				/>
				<Octadground {...octadGroundState} width="38vw" height="38vw" />
				<Clock
					time={playerTime}
					isActive={isPlayersTurn}
					barWidth={playerClockBarWidth}
					isWhite={isPlayerWhite}
				/>
				<Chin latency={latency} />
			</div>

			<Footer />
		</div>
	);
}

const padZero = (time: number, slice: number): string =>
	`0${time}`.slice(slice);

/**
 * Format time in MM:SS.CC
 * @param centiSeconds - number of centiseconds remaining
 * @returns {string} formatted time
 */
const timeFormatter = (centiSeconds: number): string => {
	const minutes = (centiSeconds / 6000) | 0;
	let minutesFmt: string | null;
	if (minutes > 9) {
		minutesFmt = padZero((centiSeconds / 6000) | 0, 1);
	} else {
		minutesFmt = padZero((centiSeconds / 6000) | 0, 0);
	}

	let seconds = ((centiSeconds / 100) | 0) % 60;
	if (seconds < 0) {
		seconds = 0;
	}
	const secondsFmt = padZero(seconds, -2);

	const centis = centiSeconds % 100;
	let centiFmt: string | null;
	if (centis < 10) {
		centiFmt = padZero(centis, 0).slice(0, 1);
	} else {
		centiFmt = `${centis}`.slice(0, 1);
	}

	return `${minutesFmt}:${secondsFmt}.${centiFmt}`;
};

/**
 * Returns a CSS width percentage based on the percentage of
 * the clock time remaining for the given time control
 * @param timeControl - time control total centiseconds
 * @param time - centiseconds remaining
 * @returns {`${number}%`}
 */
const calcBarWidth = (timeControl: number, time: number): number => {
	return Math.min((time / timeControl) * 100, 100);
};

interface ChinProps {
	latency: number;
}

const Chin = (props: ChinProps) => {
	const numConnected = 1;

	return (
		<div className="flex justify-center pt-2 pb-1 octad-tan text-2xs w-11/12 rounded-b">
			<div className="mr-1">{`${numConnected} CONNECTED`}</div>
			<div>{`(${props.latency}ms)`}</div>
		</div>
	);
};
