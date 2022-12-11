import Button from "@client/components/Button/Button";
import { Modal, ModalProps } from "@client/components/Modal/Modal";
import { Piece, PieceType } from "@client/components/Piece/Piece";
import {
	GameOutcome,
	GameOverPayload,
	NewCustomRoomPayload,
	PlayerColor,
} from "@client/proto/ws_pb";
import classNames from "classnames";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import styles from "./RematchModal.module.scss";

enum PlayerOutcome {
	WIN,
	LOSE,
	DRAW,
}

interface RematchModalProps extends Omit<ModalProps, "children"> {
	playerColor: PlayerColor.WHITE | PlayerColor.BLACK;
	gameOverPayload: GameOverPayload;
	variantHtmlName: string;
}

export function RematchModal(props: RematchModalProps) {
	const router = useRouter();
	const [playerOutcome, setPlayerOutcome] = useState<PlayerOutcome | null>(
		null,
	);
	const [
		opponentOutcome,
		setOpponentOutcome,
	] = useState<PlayerOutcome | null>(null);

	const gameOutcome = useMemo((): string => {
		switch (props.gameOverPayload.gameOutcome) {
			case GameOutcome.UNSPECIFIED:
				// TODO handle errors?
				return "Err";
			case GameOutcome.DRAW:
				setPlayerOutcome(PlayerOutcome.DRAW);
				setOpponentOutcome(PlayerOutcome.DRAW);
				return "Draw!";
			case GameOutcome.BLACK_WINS:
				if (props.playerColor === PlayerColor.BLACK) {
					setPlayerOutcome(PlayerOutcome.WIN);
					setOpponentOutcome(PlayerOutcome.LOSE);
					return "You won!";
				} else {
					setPlayerOutcome(PlayerOutcome.LOSE);
					setOpponentOutcome(PlayerOutcome.WIN);
					return "Opponent won!";
				}
			case GameOutcome.WHITE_WINS:
				if (props.playerColor === PlayerColor.WHITE) {
					setPlayerOutcome(PlayerOutcome.WIN);
					setOpponentOutcome(PlayerOutcome.LOSE);
					return "You won!";
				} else {
					setPlayerOutcome(PlayerOutcome.LOSE);
					setOpponentOutcome(PlayerOutcome.WIN);
					return "Opponent won!";
				}
		}
	}, [props.gameOverPayload.gameOutcome, props.playerColor]);

	const scores = useMemo((): string => {
		const playerScores = props.gameOverPayload.score;

		if (props.playerColor === PlayerColor.WHITE) {
			return `${playerScores?.white} - ${playerScores?.black}`;
		} else {
			return `${playerScores?.black} - ${playerScores?.white}`;
		}
	}, [props.gameOverPayload.score, props.playerColor]);

	function handleNewGame() {
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

	// function handleRematch() {

	// }

	return (
		<Modal open={props.open} close={props.close}>
			<div>
				<div
					className={classNames([styles.header], {
						[styles.win]: playerOutcome === PlayerOutcome.WIN,
						[styles.lose]: playerOutcome === PlayerOutcome.LOSE,
						[styles.draw]: playerOutcome === PlayerOutcome.DRAW,
					})}
				>
					<div className={styles.outcome}>{gameOutcome}</div>
					<div className={styles.outcomeDetails}>
						{props.gameOverPayload.outcomeDetails}
					</div>
				</div>

				<div className={styles.body}>
					<div className="flex justify-center items-center">
						<div className={styles.player}>
							<div
								className={classNames([styles.piece], {
									[styles.win]:
										playerOutcome === PlayerOutcome.WIN,
									[styles.lose]:
										playerOutcome === PlayerOutcome.LOSE,
								})}
							>
								<Piece
									pieceType={PieceType.PAWN}
									pieceColor={props.playerColor}
								/>
							</div>
							<div>You</div>
						</div>
						<div className={styles.scores}>{scores}</div>
						<div className={styles.player}>
							<div
								className={classNames([styles.piece], {
									[styles.win]:
										opponentOutcome === PlayerOutcome.WIN,
									[styles.lose]:
										opponentOutcome === PlayerOutcome.LOSE,
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
					<Button onClick={() => handleNewGame()}>New Game</Button>
					<Button>Rematch</Button>
				</div>
			</div>
		</Modal>
	);
}
