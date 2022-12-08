import { PlayerColor } from "@client/proto/ws_pb";
import classNames from "classnames";
import styles from "./Piece.module.scss";

export enum PieceTheme {
	ALPHA = "alpha",
	CBURNETT = "cburnett",
	MERIDA = "merida",
}

export enum PieceType {
	PAWN = "pawn",
	ROOK = "rook",
	KNIGHT = "knight",
	BISHOP = "bishop",
	QUEEN = "queen",
	KING = "king",
}

interface PieceProps {
	pieceType: PieceType;
	pieceColor: PlayerColor.WHITE | PlayerColor.BLACK;
}

export function Piece(props: PieceProps) {
	const pieceColorStr = PlayerColor[props.pieceColor].toLowerCase();

	return (
		<div
			className={classNames([
				props.pieceType,
				pieceColorStr,
				styles.piece,
			])}
		/>
	);
}

// TODO add split piece variants for the `alpha` and `merida` themes
export function SplitPiece() {
	return <div className={classNames(["split", styles.piece])} />;
}
