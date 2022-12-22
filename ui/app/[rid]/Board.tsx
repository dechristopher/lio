"use client";

import "react-octadground/dist/styles/octadground.css";
import { usePathname, useRouter } from "next/navigation";
import React, { useEffect, useState } from "react";
import Octadground, {
	OctadgroundProps,
	Pieces,
	Key,
} from "react-octadground/octadground";
import useWebSocket from "react-use-websocket";
import { Howl } from "howler";
import { GetBrowserId, IsMobile } from "@utils/index";
import Clock, { ClockState } from "./Clock";
import {
	WebsocketMessage,
	MovePayload,
	GameOverPayload,
	PlayerColor,
	Variant,
} from "@client/proto/ws_pb";
import PromotionModal, {
	BoardColumns,
	PromoPiece,
} from "@client/components/PromotionModal/PromotionModal";
import { RematchModal } from "./RematchModal";
import dayjs from "dayjs";
import duration from "dayjs/plugin/duration";
dayjs.extend(duration);

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

export const GameStartSound = new Howl({
	src: ["/sfx/confirmation.ogg"],
	preload: true,
	volume: 0.99,
});

export const GameOverSound = new Howl({
	src: ["/sfx/end.ogg"],
	preload: true,
	volume: 0.6,
});

export type ValidColors = PlayerColor.BLACK | PlayerColor.WHITE;

const WhitePlayerString = "white";
const BlackPlayerString = "black";

const Board = () => {
	const pathName = usePathname();
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
		onMove: (orig, dest, pieces) => doMove(orig, dest, pieces),
	});
	const [latency, setLatency] = useState(0);
	const [pongCount, setPongCount] = useState(0);
	const [numConnections, setNumConnections] = useState(0);
	const [lastPingTime, setLastPingTime] = useState<number | null>(null);
	const [showPromoModal, setShowPromoModal] = useState(false);
	const [
		promoPieceColumn,
		setPromoPieceColumn,
	] = useState<BoardColumns | null>(null);
	const [orig, setOrig] = useState<Key | null>(null);
	const [dest, setDest] = useState<Key | null>(null);
	const [showRematchModal, setShowRematchModal] = useState(true);
	const [playerClock, setPlayerClock] = useState<ClockState | null>(null);
	const [opponentClock, setOpponentClock] = useState<ClockState | null>(null);
	const [variant, setVariant] = useState<Variant | null>(null);
	const [webSocketOpen, setWebsocketOpen] = useState(true);

	const { sendMessage } = useWebSocket(
		`ws://localhost:3000/api/ws/socket${pathName}`,
		{
			share: true,
			onOpen: () => {
				console.log("[Websocket] Connected to lioctad.org");
				// immediately request game state
				sendMessage(
					new WebsocketMessage({
						data: {
							case: "gameStatePayload",
							value: {},
						},
					}).toBinary(),
				);
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
				if (event.data) {
					parseSocketMessage(event.data);
				} else {
					// TODO handle errors
				}
			},
			shouldReconnect: () => true,
			onError: (event) =>
				// TODO add logging
				console.log("[Websocket] Encountered error ", event),
		},
		webSocketOpen,
	);

	// setup interval runners on component mount
	useEffect(() => {
		// sends a keep-alive message, requesting the socket stay open
		const keepAlive = setInterval(() => {
			// console.log("[Websocket] Sending keep alive...");
			sendMessage(
				new WebsocketMessage({
					data: {
						case: "keepAlivePayload",
						value: {},
					},
				}).toBinary(),
			);
		}, 3000);
		// repeatedly pings the backend server, allowing us to calculate client latency
		const pingRunner = setInterval(() => {
			// console.log("[Websocket] Sending ping...");
			setLastPingTime(Date.now());
			sendMessage(
				new WebsocketMessage({
					data: {
						case: "pingPayload",
						value: {},
					},
				}).toBinary(),
			);
		}, 5000);

		// clears intervals when the component un-mounts
		return () => {
			console.log("Clearing keep alive...");
			clearInterval(keepAlive);
			console.log("Clearing ping runner...");
			clearInterval(pingRunner);
		};
	}, []);

	async function parseSocketMessage(rawMessage: Blob) {
		const buffer = await rawMessage.arrayBuffer();
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
				setNumConnections(message.data.value.connections);
				break;
			case "redirectPayload":
				router.push(message.data.value.location);
				break;
			default:
			// TODO do we want to do anything for the default case?
		}
	}

	function handlePing() {
		// console.log("[Websocket] Received pong");
		const currentLag = Math.min(Date.now() - (lastPingTime ?? 0), 10000);
		const newPongCount = pongCount + 1;

		// average first few pings and then move to weighted moving average
		const weight = newPongCount > 4 ? 0.1 : 1 / newPongCount;
		setPongCount(newPongCount);
		setLatency((oldLatency) =>
			Math.round((oldLatency += weight * (currentLag - oldLatency))),
		);
	}

	function handleGameOver(payload: GameOverPayload) {
		GameOverSound.play();
		setPlayerClock(
			(oldState) =>
				({
					...oldState,
					isPlayerTurn: false,
				} as ClockState),
		);
		setOpponentClock(
			(oldState) => ({ ...oldState, isPlayerTurn: false } as ClockState),
		);

		// disallow further moves
		setOctadgroundState((oldState) => ({
			...oldState,
			movable: {
				dests: new Map(),
			},
		}));

		if (payload.roomOver) {
			// TODO add a delay with notification that the user will be redirected
			router.push("/");
		} else {
			setShowRematchModal(true);
		}
	}

	const handleMove = (message: MovePayload) => {
		console.log("Move Payload", message);
		const moves = message.moves?.moves;
		const variant = message.clock?.variant;

		// TODO handle undefined values
		if (!variant?.control || !moves || !message.score || !message.clock) {
			return;
		}

		const ofenParts: string[] = message.ofen.split(" ");
		const isPlayerWhite =
			message.whitePlayerId === GetBrowserId() ? true : false;
		const isWhitePlayerTurn = ofenParts[1] === "w";
		const whitePlayerTime = dayjs.duration(
			Number(message.clock.white),
			"milliseconds",
		);
		const blackPlayerTime = dayjs.duration(
			Number(message.clock.black),
			"milliseconds",
		);

		const whitePlayerScore = message.score.white;
		const blackPlayerScore = message.score.black;
		const playerColorString = isPlayerWhite
			? WhitePlayerString
			: BlackPlayerString;
		const initialTime = dayjs.duration(
			Number(variant.control.initialTime),
			"milliseconds",
		);
		const isPlayerTurn =
			(isPlayerWhite && isWhitePlayerTurn) ||
			(!isPlayerWhite && !isWhitePlayerTurn);
		const gameStarted = moves.length > 0;

		if (isPlayerWhite) {
			setPlayerClock({
				initialTime,
				gameStarted,
				isPlayerTurn,
				score: whitePlayerScore,
				timeRemaining: whitePlayerTime,
				playerColor: PlayerColor.WHITE,
			});
			setOpponentClock({
				initialTime,
				gameStarted,
				score: blackPlayerScore,
				timeRemaining: blackPlayerTime,
				playerColor: PlayerColor.BLACK,
				isPlayerTurn: !isPlayerTurn,
			});
		} else {
			setPlayerClock({
				initialTime,
				gameStarted,
				isPlayerTurn,
				score: blackPlayerScore,
				timeRemaining: blackPlayerTime,
				playerColor: PlayerColor.BLACK,
			});
			setOpponentClock({
				initialTime,
				gameStarted,
				isPlayerTurn: !isPlayerTurn,
				score: whitePlayerScore,
				timeRemaining: whitePlayerTime,
				playerColor: PlayerColor.WHITE,
			});
		}

		// play game sounds
		if (!gameStarted) {
			GameStartSound.play();
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

		const getLastMove = (): [string, string] | [] => {
			if (gameStarted) {
				const move = moves[moves.length - 1];
				return [move.substring(0, 2), move.substring(2, 4)];
			}

			return [];
		};

		const getLegalMoves = () => {
			const allMoves = new Map<Key, Key[]>();
			Object.entries(message.validMoves).forEach(([s1, dests]) => {
				allMoves.set(s1 as Key, dests.moves as Key[]);
			});

			return allMoves;
		};

		setVariant(variant);
		setOctadgroundState((oldState) => ({
			...oldState,
			ofen: ofenParts[0],
			check: message.check,
			orientation: playerColorString,
			lastMove: getLastMove(),
			turnColor: isWhitePlayerTurn
				? WhitePlayerString
				: BlackPlayerString,
			selectable: {
				enabled: IsMobile(),
			},
			movable: {
				free: false,
				color: playerColorString,
				dests: getLegalMoves(),
			},
		}));

		// play pre-move if set
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
		sendMessage(
			new WebsocketMessage({
				data: {
					case: "movePayload",
					value: {
						uoi: orig + dest,
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
			setShowPromoModal(false);
			setPromoPieceColumn(null);
			sendMessage(
				new WebsocketMessage({
					data: {
						case: "movePayload",
						value: {
							uoi: orig + dest + promo,
						},
					},
				}).toBinary(),
			);
		}
	};

	if (!variant || !variant.control || !playerClock || !opponentClock) {
		console.log(variant, playerClock, opponentClock);
		return <div>Loading...</div>;
	}

	return (
		<div className="flex flex-col items-center pt-8">
			{GetBrowserId()}
			<div className="font-bold text-3xl italic mb-2 leading-none">
				{variant.name}
			</div>

			<div className="flex flex-col items-center">
				<Clock
					state={opponentClock}
					flipOrientation={true}
					setIsActive={() =>
						setOpponentClock(
							(s) =>
								({ ...s, isPlayerTurn: false } as ClockState),
						)
					}
				/>
				<div className="relative">
					<Octadground
						{...octadGroundState}
						width="38vw"
						height="38vw"
					/>

					{!!promoPieceColumn && (
						<PromotionModal
							open={showPromoModal}
							boardColumn={promoPieceColumn}
							playerColor={playerClock.playerColor}
							onPieceSelection={doMovePromo}
						/>
					)}
				</div>
				<Clock
					state={playerClock}
					flipOrientation={true}
					setIsActive={() =>
						setPlayerClock(
							(s) =>
								({ ...s, isPlayerTurn: false } as ClockState),
						)
					}
				/>

				<div className="flex justify-center pt-2 pb-1 octad-tan text-2xs w-11/12 rounded-b">
					<div className="mr-1">{`${numConnections} CONNECTED`}</div>
					<div>{`(${latency}ms)`}</div>
				</div>
			</div>

			<RematchModal
				open={showRematchModal}
				websocketOpen={webSocketOpen}
				variantHtmlName={variant.htmlName}
				playerColor={playerClock.playerColor}
				close={(closeWebsocket) => {
					setShowRematchModal(false);
					if (closeWebsocket) {
						setWebsocketOpen(false);
					}
				}}
			/>
		</div>
	);
};

export default Board;
