import Button from "@client/components/Button/Button";
import { Modal, ModalProps } from "@client/components/Modal/Modal";
import { Piece, PieceType } from "@client/components/Piece/Piece";
import {
	GameOutcome,
	GameOverPayload,
	NewCustomRoomPayload,
	PlayerColor,
	RematchPayload,
	WebsocketMessage,
} from "@client/proto/ws_pb";
import classNames from "classnames";
import { usePathname, useRouter } from "next/navigation";
import { useState } from "react";
import useWebSocket from "react-use-websocket";
import { ValidColors } from "./Board";
import styles from "./RematchModal.module.scss";

enum PlayerOutcome {
	WIN,
	LOSE,
	DRAW,
}

enum RematchButtonState {
	INITIAL,
	WAITING,
	REQUESTED,
	DENIED,
}

const RematchButtonTextMap: Record<RematchButtonState, string> = {
	[RematchButtonState.INITIAL]: "Rematch",
	[RematchButtonState.WAITING]: "Rematch Requested",
	[RematchButtonState.REQUESTED]: "Accept Rematch?",
	[RematchButtonState.DENIED]: "Rematch Denied",
};

type MatchDetails = {
	playerOutcome: PlayerOutcome;
	opponentOutcome: PlayerOutcome;
	gameOutcomeString: string;
	outcomeDetails: string;
	playerScoresStr: string;
};

interface RematchModalProps extends Omit<ModalProps, "children" | "close"> {
	playerColor: ValidColors;
	variantHtmlName: string;
	websocketOpen: boolean;
	close: (closeWebsocket: boolean) => void;
}

export function RematchModal(props: RematchModalProps) {
	const router = useRouter();
	const pathName = usePathname();
	const [canRematch, setCanRematch] = useState<boolean>(false);
	const [rematchBtnState, setRematchBtnState] = useState(
		RematchButtonState.INITIAL,
	);
	const [matchDetails, setMatchDetails] = useState<MatchDetails | null>(null);

	useWebSocket(
		`ws://localhost:3000/api/ws/socket${pathName}`,
		{
			share: true,
			onOpen: () => {
				console.log("[Websocket] Connected to lioctad.org");
			},
			onClose: (event) => {
				console.warn(
					"[Websocket] Lost connection to lioctad.org",
					event,
				);
			},
			onMessage: (event) => {
				if (event.data) {
					parseSocketResponse(event.data);
				} else {
					// TODO handle error
				}
			},
			shouldReconnect: () => true,
			onError: (event) =>
				console.log("[Websocket] Encountered error ", event),
			// TODO add logging
		},
		props.websocketOpen,
	);

	async function parseSocketResponse(res: Blob) {
		const buffer = await res.arrayBuffer();
		const message = WebsocketMessage.fromBinary(new Uint8Array(buffer));

		switch (message.data.case) {
			case "rematchPayload":
				handleRematch(message.data.value);
				break;
			case "gameOverPayload":
				handleGameOver(message.data.value);
				break;
			default:
			// TODO do we want to handle the default case?
		}
	}

	function handleRematch(payload: RematchPayload) {
		if (payload.rematchReady) {
			handleOnClose(false);
			return;
		}

		if (!payload.bothPlayersPresent) {
			setCanRematch(false);
			// show a "denied" status if a player requests a rematch but then one leaves
			if (payload.blackRequested || payload.whiteRequested) {
				setRematchBtnState(RematchButtonState.DENIED);
			}
			return;
		}

		setCanRematch(true);
		if (payload.blackRequested) {
			if (props.playerColor === PlayerColor.BLACK) {
				setRematchBtnState(RematchButtonState.WAITING);
			} else {
				setRematchBtnState(RematchButtonState.REQUESTED);
			}
		}

		if (payload.whiteRequested) {
			if (props.playerColor === PlayerColor.WHITE) {
				setRematchBtnState(RematchButtonState.WAITING);
			} else {
				setRematchBtnState(RematchButtonState.REQUESTED);
			}
		}
	}

	function handleGameOver(payload: GameOverPayload): void {
		const playerScores = payload.score;
		if (!playerScores) {
			// TODO handle error
			return;
		}

		let playerOutcome: PlayerOutcome | null = null;
		let opponentOutcome: PlayerOutcome | null = null;
		let gameOutcomeString: string | null = null;
		let playerScoresString: string | null = null;

		if (props.playerColor === PlayerColor.WHITE) {
			playerScoresString = `${playerScores.white} - ${playerScores.black}`;
		} else {
			playerScoresString = `${playerScores.black} - ${playerScores.white}`;
		}

		switch (payload.gameOutcome) {
			case GameOutcome.UNSPECIFIED:
				// TODO handle error
				return;
			case GameOutcome.DRAW:
				gameOutcomeString = "Draw!";
				playerOutcome = PlayerOutcome.DRAW;
				opponentOutcome = PlayerOutcome.DRAW;
				break;
			case GameOutcome.BLACK_WINS:
				if (props.playerColor === PlayerColor.BLACK) {
					gameOutcomeString = "You won!";
					playerOutcome = PlayerOutcome.WIN;
					opponentOutcome = PlayerOutcome.LOSE;
				} else {
					gameOutcomeString = "Opponent won!";
					playerOutcome = PlayerOutcome.LOSE;
					opponentOutcome = PlayerOutcome.WIN;
				}
				break;
			case GameOutcome.WHITE_WINS:
				if (props.playerColor === PlayerColor.WHITE) {
					gameOutcomeString = "You won!";
					playerOutcome = PlayerOutcome.WIN;
					opponentOutcome = PlayerOutcome.LOSE;
				} else {
					gameOutcomeString = "Opponent won!";
					playerOutcome = PlayerOutcome.LOSE;
					opponentOutcome = PlayerOutcome.WIN;
				}
				break;
		}

		setMatchDetails({
			playerOutcome,
			opponentOutcome,
			gameOutcomeString,
			playerScoresStr: playerScoresString,
			outcomeDetails: payload.outcomeDetails,
		});
	}

	function newGame() {
		fetch("/api/room/new/human", {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
			},
			body: new NewCustomRoomPayload({
				playerColor: PlayerColor.UNSPECIFIED,
				variantHtmlName: props.variantHtmlName,
			}).toBinary(),
		}).then((response) => {
			if (response.status === 200) {
				router.push(response.url);
			} else {
				console.error(`Error creating new game`, response);
			}
		});
	}

	function requestRematch() {
		fetch(`/api/room${pathName}/rematch`, {
			method: "POST",
		});
	}

	// reset state and close the modal
	function handleOnClose(closeWebsocket: boolean): void {
		setCanRematch(false);
		setRematchBtnState(RematchButtonState.INITIAL);
		props.close(closeWebsocket);
	}

	// TODO handle nulls
	if (!matchDetails) {
		return null;
	}

	return (
		<Modal open={props.open} close={() => handleOnClose(true)}>
			<div>
				<div
					className={classNames([styles.header], {
						[styles.win]:
							matchDetails.playerOutcome === PlayerOutcome.WIN,
						[styles.lose]:
							matchDetails.playerOutcome === PlayerOutcome.LOSE,
						[styles.draw]:
							matchDetails.playerOutcome === PlayerOutcome.DRAW,
					})}
				>
					<div className={styles.outcome}>
						{matchDetails.gameOutcomeString}
					</div>
					<div className={styles.outcomeDetails}>
						{matchDetails.outcomeDetails}
					</div>
				</div>
				<div className={styles.body}>
					<div className="flex justify-center items-center">
						<div className={styles.player}>
							<div
								className={classNames([styles.piece], {
									[styles.win]:
										matchDetails.playerOutcome ===
										PlayerOutcome.WIN,
									[styles.lose]:
										matchDetails.playerOutcome ===
										PlayerOutcome.LOSE,
								})}
							>
								<Piece
									pieceType={PieceType.PAWN}
									pieceColor={props.playerColor}
								/>
							</div>
							<div>You</div>
						</div>
						<div className={styles.scores}>
							{matchDetails.playerScoresStr}
						</div>
						<div className={styles.player}>
							<div
								className={classNames([styles.piece], {
									[styles.win]:
										matchDetails?.opponentOutcome ===
										PlayerOutcome.WIN,
									[styles.lose]:
										matchDetails.opponentOutcome ===
										PlayerOutcome.LOSE,
								})}
							>
								<Piece
									pieceType={PieceType.PAWN}
									pieceColor={
										props.playerColor === PlayerColor.WHITE
											? PlayerColor.BLACK
											: PlayerColor.WHITE
									}
								/>
							</div>
							<div>Opponent</div>
						</div>
					</div>

					{/* TODO add back when Elo is tracked */}
					{/* <div className={styles.elo}>2144 +7</div> */}
				</div>
				<div className={styles.footer}>
					<Button onClick={() => newGame()}>New Game</Button>
					<Button
						disabled={
							!canRematch ||
							rematchBtnState === RematchButtonState.WAITING
						}
						onClick={requestRematch}
						className={classNames({
							[styles.denied]:
								rematchBtnState === RematchButtonState.DENIED,
							[styles.waiting]:
								rematchBtnState === RematchButtonState.WAITING,
							[styles.requested]:
								rematchBtnState ===
								RematchButtonState.REQUESTED,
						})}
					>
						{RematchButtonTextMap[rematchBtnState]}
					</Button>
				</div>
			</div>
		</Modal>
	);
}
