"use client";

import "react-octadground/dist/styles/octadground.css";
import { usePathname } from "next/navigation";
import React, { useEffect, useState } from "react";
import Octadground, {
	OctadgroundProps,
	Pieces,
	Key,
} from "react-octadground/octadground";
import useWebSocket from "react-use-websocket";
import { Howl } from "howler";
import { useAnimationFrame } from "@hooks/useAnimationFrame";
import { GetBrowserId, IsMobile } from "@utils/index";
import Clock from "./Clock";
import {
	WebsocketMessage,
	MovePayload,
	Moves,
	GameOverPayload,
	PlayerColor,
} from "@client/proto/ws_pb";
import PromotionModal, {
	BoardColumns,
	PromoPiece,
} from "@client/components/PromotionModal/PromotionModal";

export const MoveSound = new Howl({
	src: ["/sfx/move.ogg"],
	preload: true,
	autoplay: true,
	html5: true,
	volume: 0.9,
});

export const CaptureSound = new Howl({
	src: ["/sfx/capture.ogg"],
	preload: true,
	volume: 0.9,
});

export const ConfirmationSound = new Howl({
	src: ["/sfx/confirmation.ogg"],
	preload: true,
	volume: 0.99,
});

export const NotificationSound = new Howl({
	src: ["/sfx/end.ogg"],
	preload: true,
	volume: 0.6,
});

const WhitePlayerString = "white";
const BlackPlayerString = "black";

const Board = () => {
	const pathName = usePathname();
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
		onMove: (orig, dest, pieces) => doMove(orig, dest, pieces),
	});

	const [moveCount, setMoveCount] = useState(1);
	const [playerTime, setPlayerTime] = useState<string>("");
	const [playerRemainingTime, setPlayerRemainingTime] = useState(0);
	const [opponentRemainingTime, setOpponentRemainingTime] = useState(0);
	const [lastPingTime, setLastPingTime] = useState<number>(Date.now());
	const [pongCount, setPongCount] = useState(0);
	const [latency, setLatency] = useState(0);
	const [timeControl, setTimeControl] = useState<number>(0);
	const [opponentTime, setOpponentTime] = useState<string>("");
	const [playerScore, setPlayerScore] = useState<number>(0);
	const [opponentScore, setOpponentScore] = useState<number>(0);

	const [playerColor, setPlayerColor] = useState<PlayerColor | null>(null);
	const [isPlayerTurn, setIsPlayerTurn] = useState(false);
	const [playerClockActive, setPlayerClockActive] = useState(false);
	const [opponentClockActive, setOpponentClockActive] = useState(false);
	const [playerClockBarWidth, setPlayerClockBarWidth] = useState(0);
	const [opponentClockBarWidth, setOpponentClockBarWidth] = useState(0);
	const [showPromoModal, setShowPromoModal] = useState(false);
	const [
		promoPieceColumn,
		setPromoPieceColumn,
	] = useState<BoardColumns | null>(null);
	const [orig, setOrig] = useState<Key | null>(null);
	const [dest, setDest] = useState<Key | null>(null);
	const [numClients, setNumClients] = useState(0);

	const { sendMessage } = useWebSocket(
		`ws://localhost:3000/api/ws/socket${pathName}`,
		{
			onOpen: () => {
				console.log("[Websocket] Connected to lioctad.org");
				// sends an empty move message to prompt a response with board info
				requestBoardInfo();
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
				console.log("[Websocket] Received message", event);
				if (event.data) {
					parseSocketResponse(event.data);
				}
			},
			shouldReconnect: () => true,
			onError: (event) =>
				console.log("[Websocket] Encountered error ", event),
		},
	);

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

	// kick things off, on component mount
	useEffect(() => {
		// requestBoardInfo();
		// TODO add polyfills
		// window.requestAnimationFrame = (function () {
		// 	return function (callback: FrameRequestCallback): number {
		// 		return window.setTimeout(callback, 1000 / 60);¡¡
		// 	};
		// })();

		// sends a keep-alive message, requesting the socket stay open
		const keepAlive = setInterval(() => {
			console.log("[Websocket] Sending keep alive...");
			sendMessage(
				new WebsocketMessage({
					data: {
						case: "keepAlivePayload",
						value: {},
					},
				}).toBinary(),
			);
		}, 3000);
		// pings the backend server every, used for calculating client latency
		const pingRunner = setInterval(() => {
			console.log("[Websocket] Sending ping...");
			sendMessage(
				new WebsocketMessage({
					data: {
						case: "pingPayload",
						value: {},
					},
				}).toBinary(),
			);
			setLastPingTime(Date.now());
		}, 5000);

		// clears intervals when the component un-mounts
		return () => {
			console.log("Clearing keep alive...");
			clearInterval(keepAlive);
			console.log("Clearing ping runner...");
			clearInterval(pingRunner);
		};
	}, []);

	function requestBoardInfo() {
		sendMessage(
			new WebsocketMessage({
				data: {
					case: "movePayload",
					value: {
						ack: 0,
					},
				},
			}).toBinary(),
		);
	}

	async function parseSocketResponse(res: Blob) {
		const buffer = await res.arrayBuffer();
		const message = WebsocketMessage.fromBinary(new Uint8Array(buffer));

		switch (message.data.case) {
			case "pongPayload":
				handlePing();
				break;
			case "movePayload":
				handleMove(message.data.value);
				break;
			case "gameOverPayload":
				handleGameOver(message.data.value);
				break;
			case "crowdPayload":
				setNumClients(message.data.value.spectators);
				break;
			default:
				console.warn(
					`[Websocket] Unimplemented message handler! (${message.data.case})`,
				);
		}
	}

	function handlePing() {
		console.log("[Websocket] Received pong");
		const currentLag = Math.min(Date.now() - lastPingTime, 10000);
		const newPongCount = pongCount + 1;

		// average first few pings and then move to weighted moving average
		const weight = newPongCount > 4 ? 0.1 : 1 / newPongCount;
		setPongCount(newPongCount);
		setLatency((oldLatency) =>
			Math.round((oldLatency += weight * (currentLag - oldLatency))),
		);
	}

	function handleGameOver(payload: GameOverPayload) {
		setPlayerClockActive(false);
		setOpponentClockActive(false);
		setPlayerRemainingTime(timeControl);
		setOpponentRemainingTime(timeControl);

		NotificationSound.play();

		// disallow further moves
		setOctadgroundState((oldState) => ({
			...oldState,
			movable: {
				dests: new Map(),
			},
		}));

		// TODO show rematch modal

		// if room over, redirect home after a second
		if (payload.roomOver === true) {
			setTimeout(() => {
				window.location.href = "/";
			}, 3000);
		}
	}

	/**
	 * Handle incoming move messages, update board state, update UI and clocks
	 * @param message
	 */
	const handleMove = (message: MovePayload) => {
		console.log("[Game] Move Payload", message);
		const ofenParts: string[] = message.ofen.split(" ") ?? [];

		const isPlayerWhite = message.white === GetBrowserId() ? true : false;
		const isWhitesTurn = ofenParts[1] === "w";
		const isPlayersTurn =
			(isPlayerWhite && isWhitesTurn) ||
			(!isPlayerWhite && !isWhitesTurn);
		setIsPlayerTurn(isPlayersTurn);
		setPlayerColor(isPlayerWhite ? PlayerColor.WHITE : PlayerColor.BLACK);

		// these are in centiseconds
		const whiteTime = message.clock?.white;
		const blackTime = message.clock?.black;
		const playerTime = (isPlayerWhite ? whiteTime : blackTime) ?? 0;
		const opponentTime = (isPlayerWhite ? blackTime : whiteTime) ?? 0;
		setPlayerRemainingTime(Number(playerTime));
		setOpponentRemainingTime(Number(opponentTime));

		// ensure both clocks are stopped
		setPlayerClockActive(false);
		setOpponentClockActive(false);

		// game sounds
		if (message.gameStart) {
			// play confirmation sound on game start
			ConfirmationSound.play();
		} else {
			// play move sounds if game is not starting
			// and only if board ofen is different from current
			if (message.san && ofenParts[0] !== octadGroundState.ofen) {
				if (message.san.includes("x")) {
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

		const getLegalMoves = (moves: { [key: string]: Moves }) => {
			const allMoves = new Map<Key, Key[]>();
			if (moves) {
				Object.entries(moves).forEach(([s1, dests]) => {
					allMoves.set(s1 as Key, dests.moves as Key[]);
				});
			}
			return allMoves;
		};

		setTimeControl(Number(message.clock?.control) ?? 0);

		const playerColor = isPlayerWhite
			? WhitePlayerString
			: BlackPlayerString;
		setOctadgroundState((oldState) => ({
			...oldState,
			ofen: ofenParts[0],
			check: message.check ?? false,
			orientation: playerColor,
			lastMove: getLastMove(message.moves?.moves ?? []),
			turnColor: isWhitesTurn ? WhitePlayerString : BlackPlayerString,
			selectable: {
				enabled: IsMobile(),
			},
			movable: {
				free: false,
				color: playerColor,
				dests: getLegalMoves(message.validMoves),
			},
		}));

		const whiteScore = message.score?.white;
		const blackScore = message.score?.black;
		if (whiteScore !== undefined && blackScore !== undefined) {
			if (isPlayerWhite) {
				setPlayerScore(whiteScore);
				setOpponentScore(blackScore);
			} else {
				setPlayerScore(blackScore);
				setOpponentScore(whiteScore);
			}
		}

		// run at the start of the game
		if (!message.moves?.moves.length) {
			setMoveCount(1);
			setPlayerTime(timeFormatter(Math.max(Number(playerTime), 0)));
			setOpponentTime(timeFormatter(Math.max(Number(opponentTime), 0)));
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
		const destPiece = pieces.get(dest);

		if (destPiece && destPiece.role === "pawn") {
			if (
				(destPiece.color === WhitePlayerString && dest[1] === "4") ||
				(destPiece.color === BlackPlayerString && dest[1] === "1")
			) {
				setOrig(orig);
				setDest(dest);
				setShowPromoModal(true);
				setPromoPieceColumn(dest[0] as BoardColumns);

				// return early and wait for doMovePromo to run
				return;
			}
		}

		console.log("[Websocket] Sending game move...");
		setMoveCount((oldMove) => oldMove++);
		sendMessage(
			new WebsocketMessage({
				data: {
					case: "movePayload",
					value: {
						uoi: orig + dest,
						ack: moveCount,
					},
				},
			}).toBinary(),
		);
	};

	/**
	 * Perform move from origin to destination square with selected promotion
	 * @param promo - code of piece to promote to
	 */
	const doMovePromo = (promo: PromoPiece) => {
		console.log("doMovePromo", orig, dest, promo);
		if (orig && dest) {
			setMoveCount((oldMove) => oldMove++);
			setShowPromoModal(false);
			setPromoPieceColumn(null);
			sendMessage(
				new WebsocketMessage({
					data: {
						case: "movePayload",
						value: {
							uoi: orig + dest + promo,
							ack: moveCount,
						},
					},
				}).toBinary(),
			);
		}
	};

	return (
		<div className="flex flex-col items-center pt-8">
			<div className="font-bold text-3xl italic mb-2 leading-none">
				½ + 1 Blitz
			</div>

			<div className="flex flex-col items-center">
				<Clock
					flipOrientation
					time={opponentTime}
					score={opponentScore}
					isActive={!isPlayerTurn}
					barWidth={opponentClockBarWidth}
					isWhite={!(playerColor === PlayerColor.WHITE)}
				/>
				<div className="relative">
					<Octadground
						{...octadGroundState}
						width="38vw"
						height="38vw"
					/>

					{!!promoPieceColumn && !!playerColor && (
						<PromotionModal
							open={showPromoModal}
							boardColumn={promoPieceColumn}
							playerColor={playerColor}
							onPieceSelection={doMovePromo}
						/>
					)}
				</div>
				<Clock
					time={playerTime}
					score={playerScore}
					isActive={isPlayerTurn}
					barWidth={playerClockBarWidth}
					isWhite={playerColor === PlayerColor.WHITE}
				/>

				<div className="flex justify-center pt-2 pb-1 octad-tan text-2xs w-11/12 rounded-b">
					<div className="mr-1">{`${numClients} CONNECTED`}</div>
					<div>{`(${latency}ms)`}</div>
				</div>
			</div>
		</div>
	);
};

/**
 * Format time in MM:SS.CC
 * @param centiSeconds - number of centiseconds remaining
 * @returns {string} formatted time
 */
const timeFormatter = (centiSeconds: number): string => {
	const padZero = (time: number, slice: number): string =>
		`0${time}`.slice(slice);

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

export default Board;
